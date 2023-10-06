// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package offsetguess

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/native"
)

const InterfaceLocalMulticastIPv6 = "ff01::1"
const listenIPv4 = "127.0.0.2"

const (
	tcpGetSockOptKProbeNotCalled uint64 = 0
	tcpGetSockOptKProbeCalled    uint64 = 1
)

var tcpKprobeCalledString = map[uint64]string{
	tcpGetSockOptKProbeNotCalled: "tcp_getsockopt kprobe not executed",
	tcpGetSockOptKProbeCalled:    "tcp_getsockopt kprobe executed",
}

type tracerOffsetGuesser struct {
	m          *manager.Manager
	status     *TracerStatus
	guessTCPv6 bool
	guessUDPv6 bool
}

func NewTracerOffsetGuesser() (OffsetGuesser, error) {
	return &tracerOffsetGuesser{
		m: &manager.Manager{
			Maps: []*manager.Map{
				{Name: "connectsock_ipv6"},
				{Name: probes.TracerStatusMap},
			},
			PerfMaps: []*manager.PerfMap{},
			Probes: []*manager.Probe{
				{ProbeIdentificationPair: idPair(probes.TCPGetSockOpt)},
				{ProbeIdentificationPair: idPair(probes.SockGetSockOpt)},
				{ProbeIdentificationPair: idPair(probes.TCPv6Connect)},
				{ProbeIdentificationPair: idPair(probes.IPMakeSkb)},
				{ProbeIdentificationPair: idPair(probes.IP6MakeSkb)},
				{ProbeIdentificationPair: idPair(probes.IP6MakeSkbPre470)},
				{ProbeIdentificationPair: idPair(probes.TCPv6ConnectReturn), KProbeMaxActive: 128},
				{ProbeIdentificationPair: idPair(probes.NetDevQueue)},
			},
		},
	}, nil
}

func (t *tracerOffsetGuesser) Manager() *manager.Manager {
	return t.m
}

func (t *tracerOffsetGuesser) Close() {
	ebpfcheck.RemoveNameMappings(t.m)
	if err := t.m.Stop(manager.CleanAll); err != nil {
		log.Warnf("error stopping tracer offset guesser: %s", err)
	}
}

func extractIPsAndPorts(conn net.Conn) (
	saddr, daddr uint32,
	sport, dport uint16,
	err error,
) {
	saddrStr, sportStr, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return
	}
	saddr = native.Endian.Uint32(net.ParseIP(saddrStr).To4())
	sportn, err := strconv.Atoi(sportStr)
	if err != nil {
		return
	}
	sport = uint16(sportn)

	daddrStr, dportStr, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return
	}
	daddr = native.Endian.Uint32(net.ParseIP(daddrStr).To4())
	dportn, err := strconv.Atoi(dportStr)
	if err != nil {
		return
	}

	dport = uint16(dportn)
	return
}

func extractIPv6AddressAndPort(addr net.Addr) (ip [4]uint32, port uint16, err error) {
	udpAddr, err := net.ResolveUDPAddr(addr.Network(), addr.String())
	if err != nil {
		return
	}

	ip, err = uint32ArrayFromIPv6(udpAddr.IP)
	if err != nil {
		return
	}
	port = uint16(udpAddr.Port)

	return
}

func expectedValues(conn net.Conn) (*fieldValues, error) {
	netns, err := kernel.GetCurrentIno()
	if err != nil {
		return nil, err
	}

	tcpInfo, err := TcpGetInfo(conn)
	if err != nil {
		return nil, err
	}

	saddr, daddr, sport, dport, err := extractIPsAndPorts(conn)
	if err != nil {
		return nil, err
	}

	return &fieldValues{
		saddr:  saddr,
		daddr:  daddr,
		sport:  sport,
		dport:  dport,
		netns:  netns,
		family: syscall.AF_INET,
		rtt:    tcpInfo.Rtt,
		rttVar: tcpInfo.Rttvar,
	}, nil
}

func waitUntilStable(conn net.Conn, window time.Duration, attempts int) (*fieldValues, error) {
	var (
		current *fieldValues
		prev    *fieldValues
		err     error
	)
	for i := 0; i <= attempts; i++ {
		current, err = expectedValues(conn)
		if err != nil {
			return nil, err
		}

		if prev != nil && *prev == *current {
			return current, nil
		}

		prev = current
		time.Sleep(window)
	}

	return nil, errors.New("unstable TCP socket params")
}

func (*tracerOffsetGuesser) Probes(c *config.Config) (map[probes.ProbeFuncName]struct{}, error) {
	p := map[probes.ProbeFuncName]struct{}{}
	enableProbe(p, probes.TCPGetSockOpt)
	enableProbe(p, probes.SockGetSockOpt)
	enableProbe(p, probes.IPMakeSkb)
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("could not kernel version: %w", err)
	}
	if kv >= kernel.VersionCode(4, 7, 0) {
		enableProbe(p, probes.NetDevQueue)
	}

	if c.CollectTCPv6Conns || c.CollectUDPv6Conns {
		enableProbe(p, probes.TCPv6Connect)
		enableProbe(p, probes.TCPv6ConnectReturn)
	}

	if c.CollectUDPv6Conns {
		if kv < kernel.VersionCode(5, 18, 0) {
			if kv < kernel.VersionCode(4, 7, 0) {
				enableProbe(p, probes.IP6MakeSkbPre470)
			} else {
				enableProbe(p, probes.IP6MakeSkb)
			}
		}
	}
	return p, nil
}

func compareIPv6(a [4]uint32, b [4]uint32) bool {
	for i := 0; i < 4; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func htons(a uint16) uint16 {
	var arr [2]byte
	binary.BigEndian.PutUint16(arr[:], a)
	return native.Endian.Uint16(arr[:])
}

func generateRandomIPv6Address() net.IP {
	// multicast (ff00::/8) or link-local (fe80::/10) addresses don't work for
	// our purposes so let's choose a "random number" for the first 32 bits.
	//
	// chosen by fair dice roll.
	// guaranteed to be random.
	// https://xkcd.com/221/
	base := []byte{0x87, 0x58, 0x60, 0x31}
	addr := make([]byte, 16)
	copy(addr, base)
	_, err := rand.Read(addr[4:])
	if err != nil {
		panic(err)
	}

	return addr
}

func uint32ArrayFromIPv6(ip net.IP) (addr [4]uint32, err error) {
	buf := []byte(ip)
	if len(buf) < 15 {
		err = fmt.Errorf("invalid IPv6 address byte length %d", len(buf))
		return
	}

	addr[0] = native.Endian.Uint32(buf[0:4])
	addr[1] = native.Endian.Uint32(buf[4:8])
	addr[2] = native.Endian.Uint32(buf[8:12])
	addr[3] = native.Endian.Uint32(buf[12:16])
	return
}

// IPv6LinkLocalPrefix is only exposed for testing purposes
var IPv6LinkLocalPrefix = "fe80::"

func GetIPv6LinkLocalAddress() ([]*net.UDPAddr, error) {
	ints, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var udpAddrs []*net.UDPAddr
	for _, i := range ints {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if strings.HasPrefix(a.String(), IPv6LinkLocalPrefix) && !strings.HasPrefix(i.Name, "dummy") {
				// this address *may* have CIDR notation
				if ar, _, err := net.ParseCIDR(a.String()); err == nil {
					udpAddrs = append(udpAddrs, &net.UDPAddr{IP: ar, Zone: i.Name})
					continue
				}
				udpAddrs = append(udpAddrs, &net.UDPAddr{IP: net.ParseIP(a.String()), Zone: i.Name})
			}
		}
	}
	if len(udpAddrs) > 0 {
		return udpAddrs, nil
	}
	return nil, fmt.Errorf("no IPv6 link local address found")
}

// checkAndUpdateCurrentOffset checks the value for the current offset stored
// in the eBPF map against the expected value, incrementing the offset if it
// doesn't match, or going to the next field to guess if it does
func (t *tracerOffsetGuesser) checkAndUpdateCurrentOffset(mp *ebpf.Map, expected *fieldValues, maxRetries *int, threshold uint64) error {
	// get the updated map value so we can check if the current offset is
	// the right one
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(t.status)); err != nil {
		return fmt.Errorf("error reading tracer_status: %v", err)
	}

	if State(t.status.State) != StateChecked {
		if *maxRetries == 0 {
			return fmt.Errorf("invalid guessing state while guessing %v, got %v expected %v. %v",
				whatString[GuessWhat(t.status.What)], stateString[State(t.status.State)], stateString[StateChecked], tcpKprobeCalledString[t.status.Tcp_info_kprobe_status])
		}
		*maxRetries--
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	var overlapped bool
	switch GuessWhat(t.status.What) {
	case GuessSAddr:
		t.status.Offset_saddr, overlapped = skipOverlaps(t.status.Offset_saddr, t.sockRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Saddr == expected.saddr {
			t.logAndAdvance(t.status.Offset_saddr, GuessDAddr)
			break
		}
		t.status.Offset_saddr++
		t.status.Offset_saddr, _ = skipOverlaps(t.status.Offset_saddr, t.sockRanges())
	case GuessDAddr:
		t.status.Offset_daddr, overlapped = skipOverlaps(t.status.Offset_daddr, t.sockRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Daddr == expected.daddr {
			t.logAndAdvance(t.status.Offset_daddr, GuessDPort)
			break
		}
		t.status.Offset_daddr++
		t.status.Offset_daddr, _ = skipOverlaps(t.status.Offset_daddr, t.sockRanges())
	case GuessDPort:
		t.status.Offset_dport, overlapped = skipOverlaps(t.status.Offset_dport, t.sockRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Dport == htons(expected.dport) {
			t.logAndAdvance(t.status.Offset_dport, GuessFamily)
			// we know the family ((struct __sk_common)->skc_family) is
			// after the skc_dport field, so we start from there
			t.status.Offset_family = t.status.Offset_dport
			break
		}
		t.status.Offset_dport++
		t.status.Offset_dport, _ = skipOverlaps(t.status.Offset_dport, t.sockRanges())
	case GuessFamily:
		t.status.Offset_family, overlapped = skipOverlaps(t.status.Offset_family, t.sockRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Family == expected.family {
			t.logAndAdvance(t.status.Offset_family, GuessSPort)
			// we know the sport ((struct inet_sock)->inet_sport) is
			// after the family field, so we start from there
			t.status.Offset_sport = t.status.Offset_family
			break
		}
		t.status.Offset_family++
		t.status.Offset_family, _ = skipOverlaps(t.status.Offset_family, t.sockRanges())
	case GuessSPort:
		t.status.Offset_sport, overlapped = skipOverlaps(t.status.Offset_sport, t.sockRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Sport == htons(expected.sport) {
			t.logAndAdvance(t.status.Offset_sport, GuessSAddrFl4)
			break
		}
		t.status.Offset_sport++
		t.status.Offset_sport, _ = skipOverlaps(t.status.Offset_sport, t.sockRanges())
	case GuessSAddrFl4:
		t.status.Offset_saddr_fl4, overlapped = skipOverlaps(t.status.Offset_saddr_fl4, t.flowI4Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Saddr_fl4 == expected.saddrFl4 {
			t.logAndAdvance(t.status.Offset_saddr_fl4, GuessDAddrFl4)
			break
		}
		t.status.Offset_saddr_fl4++
		t.status.Offset_saddr_fl4, _ = skipOverlaps(t.status.Offset_saddr_fl4, t.flowI4Ranges())
		if t.status.Offset_saddr_fl4 >= threshold {
			// Let's skip all other flowi4 fields
			t.logAndAdvance(notApplicable, t.flowi6EntryState())
			t.status.Fl4_offsets = disabled
			break
		}
	case GuessDAddrFl4:
		t.status.Offset_daddr_fl4, overlapped = skipOverlaps(t.status.Offset_daddr_fl4, t.flowI4Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Daddr_fl4 == expected.daddrFl4 {
			t.logAndAdvance(t.status.Offset_daddr_fl4, GuessSPortFl4)
			break
		}
		t.status.Offset_daddr_fl4++
		t.status.Offset_daddr_fl4, _ = skipOverlaps(t.status.Offset_daddr_fl4, t.flowI4Ranges())
		if t.status.Offset_daddr_fl4 >= threshold {
			t.logAndAdvance(notApplicable, t.flowi6EntryState())
			t.status.Fl4_offsets = disabled
			break
		}
	case GuessSPortFl4:
		t.status.Offset_sport_fl4, overlapped = skipOverlaps(t.status.Offset_sport_fl4, t.flowI4Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Sport_fl4 == htons(expected.sportFl4) {
			t.logAndAdvance(t.status.Offset_sport_fl4, GuessDPortFl4)
			break
		}
		t.status.Offset_sport_fl4++
		t.status.Offset_sport_fl4, _ = skipOverlaps(t.status.Offset_sport_fl4, t.flowI4Ranges())
		if t.status.Offset_sport_fl4 >= threshold {
			t.logAndAdvance(notApplicable, t.flowi6EntryState())
			t.status.Fl4_offsets = disabled
			break
		}
	case GuessDPortFl4:
		t.status.Offset_dport_fl4, overlapped = skipOverlaps(t.status.Offset_dport_fl4, t.flowI4Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Dport_fl4 == htons(expected.dportFl4) {
			t.logAndAdvance(t.status.Offset_dport_fl4, t.flowi6EntryState())
			t.status.Fl4_offsets = enabled
			break
		}
		t.status.Offset_dport_fl4++
		t.status.Offset_dport_fl4, _ = skipOverlaps(t.status.Offset_dport_fl4, t.flowI4Ranges())
		if t.status.Offset_dport_fl4 >= threshold {
			t.logAndAdvance(notApplicable, t.flowi6EntryState())
			t.status.Fl4_offsets = disabled
			break
		}
	case GuessSAddrFl6:
		t.status.Offset_saddr_fl6, overlapped = skipOverlaps(t.status.Offset_saddr_fl6, t.flowI6Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if compareIPv6(t.status.Saddr_fl6, expected.saddrFl6) {
			t.logAndAdvance(t.status.Offset_saddr_fl6, GuessDAddrFl6)
			break
		}
		t.status.Offset_saddr_fl6++
		t.status.Offset_saddr_fl6, _ = skipOverlaps(t.status.Offset_saddr_fl6, t.flowI6Ranges())
		if t.status.Offset_saddr_fl6 >= threshold {
			// Let's skip all other flowi6 fields
			t.logAndAdvance(notApplicable, GuessNetNS)
			t.status.Fl6_offsets = disabled
			break
		}
	case GuessDAddrFl6:
		t.status.Offset_daddr_fl6, overlapped = skipOverlaps(t.status.Offset_daddr_fl6, t.flowI6Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if compareIPv6(t.status.Daddr_fl6, expected.daddrFl6) {
			t.logAndAdvance(t.status.Offset_daddr_fl6, GuessSPortFl6)
			break
		}
		t.status.Offset_daddr_fl6++
		t.status.Offset_daddr_fl6, _ = skipOverlaps(t.status.Offset_daddr_fl6, t.flowI6Ranges())
		if t.status.Offset_daddr_fl6 >= threshold {
			t.logAndAdvance(notApplicable, GuessNetNS)
			t.status.Fl6_offsets = disabled
			break
		}
	case GuessSPortFl6:
		t.status.Offset_sport_fl6, overlapped = skipOverlaps(t.status.Offset_sport_fl6, t.flowI6Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Sport_fl6 == htons(expected.sportFl6) {
			t.logAndAdvance(t.status.Offset_sport_fl6, GuessDPortFl6)
			break
		}
		t.status.Offset_sport_fl6++
		t.status.Offset_sport_fl6, _ = skipOverlaps(t.status.Offset_sport_fl6, t.flowI6Ranges())
		if t.status.Offset_sport_fl6 >= threshold {
			t.logAndAdvance(notApplicable, GuessNetNS)
			t.status.Fl6_offsets = disabled
			break
		}
	case GuessDPortFl6:
		t.status.Offset_dport_fl6, overlapped = skipOverlaps(t.status.Offset_dport_fl6, t.flowI6Ranges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Dport_fl6 == htons(expected.dportFl6) {
			t.logAndAdvance(t.status.Offset_dport_fl6, GuessNetNS)
			t.status.Fl6_offsets = enabled
			break
		}
		t.status.Offset_dport_fl6++
		t.status.Offset_dport_fl6, _ = skipOverlaps(t.status.Offset_dport_fl6, t.flowI6Ranges())
		if t.status.Offset_dport_fl6 >= threshold {
			t.logAndAdvance(notApplicable, GuessNetNS)
			t.status.Fl6_offsets = disabled
			break
		}
	case GuessNetNS:
		t.status.Offset_netns, overlapped = skipOverlaps(t.status.Offset_netns, t.sockRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Netns == expected.netns {
			t.logAndAdvance(t.status.Offset_netns, GuessRTT)
			log.Debugf("Successfully guessed %v with offset of %d bytes", "ino", t.status.Offset_ino)
			break
		}
		t.status.Offset_ino++
		// go to the next offset_netns if we get an error
		if t.status.Err != 0 || t.status.Offset_ino >= threshold {
			t.status.Offset_ino = 0
			t.status.Offset_netns++
			t.status.Offset_netns, _ = skipOverlaps(t.status.Offset_netns, t.sockRanges())
		}
	case GuessRTT:
		t.status.Offset_rtt, overlapped = skipOverlaps(t.status.Offset_rtt, t.sockRanges())
		if overlapped {
			t.status.Offset_rtt_var = t.status.Offset_rtt + 4
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		// For more information on the bit shift operations see:
		// https://elixir.bootlin.com/linux/v4.6/source/net/ipv4/tcp.c#L2686
		if t.status.Rtt>>3 == expected.rtt && t.status.Rtt_var>>2 == expected.rttVar {
			t.logAndAdvance(t.status.Offset_rtt, GuessSocketSK)
			break
		}
		// We know that these two fields are always next to each other, 4 bytes apart:
		// https://elixir.bootlin.com/linux/v4.6/source/include/linux/tcp.h#L232
		// rtt -> srtt_us
		// rtt_var -> mdev_us
		t.status.Offset_rtt++
		t.status.Offset_rtt, _ = skipOverlaps(t.status.Offset_rtt, t.sockRanges())
		t.status.Offset_rtt_var = t.status.Offset_rtt + 4
	case GuessSocketSK:
		if t.status.Sport_via_sk == htons(expected.sport) && t.status.Dport_via_sk == htons(expected.dport) {
			// if we are on kernel version < 4.7, net_dev_queue tracepoint will not be activated, and thus we should skip
			// the guessing for `struct sk_buff`
			next := GuessSKBuffSock
			kv, err := kernel.HostVersion()
			if err != nil {
				return fmt.Errorf("error getting kernel version: %w", err)
			}
			kv470 := kernel.VersionCode(4, 7, 0)

			// if IPv6 enabled & kv lower than 4.7.0, skip guessing for some fields
			if (t.guessTCPv6 || t.guessUDPv6) && kv < kv470 {
				next = GuessDAddrIPv6
			}

			// if both IPv6 disabled and kv lower than 4.7.0, skip to the end
			if !t.guessTCPv6 && !t.guessUDPv6 && kv < kv470 {
				t.logAndAdvance(t.status.Offset_socket_sk, GuessNotApplicable)
				return t.setReadyState(mp)
			}

			t.logAndAdvance(t.status.Offset_socket_sk, next)
			break
		}
		t.status.Offset_socket_sk++
		// no overlaps because only field offset from `struct socket`
	case GuessSKBuffSock:
		t.status.Offset_sk_buff_sock, overlapped = skipOverlaps(t.status.Offset_sk_buff_sock, t.skBuffRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Sport_via_sk_via_sk_buf == htons(expected.sportFl4) && t.status.Dport_via_sk_via_sk_buf == htons(expected.dportFl4) {
			t.logAndAdvance(t.status.Offset_sk_buff_sock, GuessSKBuffTransportHeader)
			break
		}
		t.status.Offset_sk_buff_sock++
		t.status.Offset_sk_buff_sock, _ = skipOverlaps(t.status.Offset_sk_buff_sock, t.skBuffRanges())
	case GuessSKBuffTransportHeader:
		t.status.Offset_sk_buff_transport_header, overlapped = skipOverlaps(t.status.Offset_sk_buff_transport_header, t.skBuffRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		networkDiffFromMac := t.status.Network_header - t.status.Mac_header
		transportDiffFromNetwork := t.status.Transport_header - t.status.Network_header
		if networkDiffFromMac == 14 && transportDiffFromNetwork == 20 {
			t.logAndAdvance(t.status.Offset_sk_buff_transport_header, GuessSKBuffHead)
			break
		}
		t.status.Offset_sk_buff_transport_header++
		t.status.Offset_sk_buff_transport_header, _ = skipOverlaps(t.status.Offset_sk_buff_transport_header, t.skBuffRanges())
	case GuessSKBuffHead:
		t.status.Offset_sk_buff_head, overlapped = skipOverlaps(t.status.Offset_sk_buff_head, t.skBuffRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if t.status.Sport_via_sk_via_sk_buf == htons(expected.sportFl4) && t.status.Dport_via_sk_via_sk_buf == htons(expected.dportFl4) {
			if !t.guessTCPv6 && !t.guessUDPv6 {
				t.logAndAdvance(t.status.Offset_sk_buff_head, GuessNotApplicable)
				return t.setReadyState(mp)
			} else {
				t.logAndAdvance(t.status.Offset_sk_buff_head, GuessDAddrIPv6)
				break
			}
		}
		t.status.Offset_sk_buff_head++
		t.status.Offset_sk_buff_head, _ = skipOverlaps(t.status.Offset_sk_buff_head, t.skBuffRanges())
	case GuessDAddrIPv6:
		t.status.Offset_daddr_ipv6, overlapped = skipOverlaps(t.status.Offset_daddr_ipv6, t.sockRanges())
		if overlapped {
			// adjusted offset from eBPF overlapped with another field, we need to check new offset
			break
		}

		if compareIPv6(t.status.Daddr_ipv6, expected.daddrIPv6) {
			t.logAndAdvance(t.status.Offset_daddr_ipv6, GuessNotApplicable)
			// at this point, we've guessed all the offsets we need,
			// set the t.status to "stateReady"
			return t.setReadyState(mp)
		}
		t.status.Offset_daddr_ipv6++
		t.status.Offset_daddr_ipv6, _ = skipOverlaps(t.status.Offset_daddr_ipv6, t.sockRanges())
	default:
		return fmt.Errorf("unexpected field to guess: %v", whatString[GuessWhat(t.status.What)])
	}

	t.status.State = uint64(StateChecking)
	// update the map with the new offset/field to check
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(t.status)); err != nil {
		return fmt.Errorf("error updating tracer_t.status: %v", err)
	}

	return nil
}

func (t *tracerOffsetGuesser) setReadyState(mp *ebpf.Map) error {
	t.status.State = uint64(StateReady)
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(t.status)); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}
	return nil
}

func (t *tracerOffsetGuesser) flowi6EntryState() GuessWhat {
	if !t.guessUDPv6 {
		return GuessNetNS
	}
	return GuessSAddrFl6
}

// Guess expects manager.Manager to contain a map named tracer_status and helps initialize the
// tracer by guessing the right struct sock kernel struct offsets. Results are
// returned as constants which are runtime-edited into the tracer eBPF code.
//
// To guess the offsets, we create connections from localhost (127.0.0.1) to
// 127.0.0.2:$PORT, where we have a server listening. We store the current
// possible offset and expected value of each field in a eBPF map. In kernel-space
// we rely on two different kprobes: `tcp_getsockopt` and `tcp_connect_v6`. When they're
// are triggered, we store the value of
//
//	(struct sock *)skp + possible_offset
//
// in the eBPF map. Then, back in userspace (checkAndUpdateCurrentOffset()), we
// check that value against the expected value of the field, advancing the
// offset and repeating the process until we find the value we expect. Then, we
// guess the next field.
func (t *tracerOffsetGuesser) Guess(cfg *config.Config) ([]manager.ConstantEditor, error) {
	mp, _, err := t.m.GetMap(probes.TracerStatusMap)
	if err != nil {
		return nil, fmt.Errorf("unable to find map %s: %s", probes.TracerStatusMap, err)
	}

	// When reading kernel structs at different offsets, don't go over the set threshold
	// Defaults to 400, with a max of 3000. This is an arbitrary choice to avoid infinite loops.
	threshold := cfg.OffsetGuessThreshold

	// pid & tid must not change during the guessing work: the communication
	// between ebpf and userspace relies on it
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	processName := filepath.Base(os.Args[0])
	if len(processName) > ProcCommMaxLen { // Truncate process name if needed
		processName = processName[:ProcCommMaxLen]
	}

	cProcName := [ProcCommMaxLen + 1]int8{} // Last char has to be null character, so add one
	for i, ch := range processName {
		cProcName[i] = int8(ch)
	}

	t.guessUDPv6 = cfg.CollectUDPv6Conns
	t.guessTCPv6 = cfg.CollectTCPv6Conns
	t.status = &TracerStatus{
		State: uint64(StateChecking),
		Proc:  Proc{Comm: cProcName},
		What:  uint64(GuessSAddr),
	}

	// if we already have the offsets, just return
	err = mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(t.status))
	if err == nil && State(t.status.State) == StateReady {
		return t.getConstantEditors(), nil
	}

	eventGenerator, err := newTracerEventGenerator(t.guessUDPv6)
	if err != nil {
		return nil, err
	}
	defer eventGenerator.Close()

	// initialize map
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(t.status)); err != nil {
		return nil, fmt.Errorf("error initializing tracer_status map: %v", err)
	}

	// If the kretprobe for tcp_v4_connect() is configured with a too-low maxactive, some kretprobe might be missing.
	// In this case, we detect it and try again. See: https://github.com/weaveworks/tcptracer-bpf/issues/24
	maxRetries := 100

	// Retrieve expected values from local connection
	expected, err := waitUntilStable(eventGenerator.conn, 200*time.Millisecond, 5)
	if err != nil {
		return nil, fmt.Errorf("error retrieving expected value: %w", err)
	}

	err = eventGenerator.populateUDPExpectedValues(expected)
	if err != nil {
		return nil, fmt.Errorf("error retrieving expected value: %w", err)
	}
	log.Tracef("expected values: %+v", expected)

	log.Debugf("Checking for offsets with threshold of %d", threshold)
	for State(t.status.State) != StateReady {
		if err := eventGenerator.Generate(GuessWhat(t.status.What), expected); err != nil {
			return nil, err
		}

		if err := t.checkAndUpdateCurrentOffset(mp, expected, &maxRetries, threshold); err != nil {
			return nil, err
		}

		// Stop at a reasonable offset so we don't run forever.
		// Reading too far away in kernel memory is not a big deal:
		// probe_kernel_read() handles faults gracefully.
		if t.status.Offset_saddr >= threshold || t.status.Offset_daddr >= threshold ||
			t.status.Offset_sport >= thresholdInetSock || t.status.Offset_dport >= threshold ||
			t.status.Offset_netns >= threshold || t.status.Offset_family >= threshold ||
			t.status.Offset_daddr_ipv6 >= threshold || t.status.Offset_rtt >= thresholdInetSock ||
			t.status.Offset_socket_sk >= threshold || t.status.Offset_sk_buff_sock >= threshold ||
			t.status.Offset_sk_buff_transport_header >= threshold || t.status.Offset_sk_buff_head >= threshold {
			return nil, fmt.Errorf("overflow while guessing %v, bailing out", whatString[GuessWhat(t.status.What)])
		}
	}

	return t.getConstantEditors(), nil
}

func (t *tracerOffsetGuesser) getConstantEditors() []manager.ConstantEditor {
	return []manager.ConstantEditor{
		{Name: "offset_saddr", Value: t.status.Offset_saddr},
		{Name: "offset_daddr", Value: t.status.Offset_daddr},
		{Name: "offset_sport", Value: t.status.Offset_sport},
		{Name: "offset_dport", Value: t.status.Offset_dport},
		{Name: "offset_netns", Value: t.status.Offset_netns},
		{Name: "offset_ino", Value: t.status.Offset_ino},
		{Name: "offset_family", Value: t.status.Offset_family},
		{Name: "offset_rtt", Value: t.status.Offset_rtt},
		{Name: "offset_rtt_var", Value: t.status.Offset_rtt_var},
		{Name: "offset_daddr_ipv6", Value: t.status.Offset_daddr_ipv6},
		{Name: "offset_saddr_fl4", Value: t.status.Offset_saddr_fl4},
		{Name: "offset_daddr_fl4", Value: t.status.Offset_daddr_fl4},
		{Name: "offset_sport_fl4", Value: t.status.Offset_sport_fl4},
		{Name: "offset_dport_fl4", Value: t.status.Offset_dport_fl4},
		{Name: "fl4_offsets", Value: uint64(t.status.Fl4_offsets)},
		{Name: "offset_saddr_fl6", Value: t.status.Offset_saddr_fl6},
		{Name: "offset_daddr_fl6", Value: t.status.Offset_daddr_fl6},
		{Name: "offset_sport_fl6", Value: t.status.Offset_sport_fl6},
		{Name: "offset_dport_fl6", Value: t.status.Offset_dport_fl6},
		{Name: "fl6_offsets", Value: uint64(t.status.Fl6_offsets)},
		{Name: "offset_socket_sk", Value: t.status.Offset_socket_sk},
		{Name: "offset_sk_buff_sock", Value: t.status.Offset_sk_buff_sock},
		{Name: "offset_sk_buff_transport_header", Value: t.status.Offset_sk_buff_transport_header},
		{Name: "offset_sk_buff_head", Value: t.status.Offset_sk_buff_head},
	}
}

type tracerEventGenerator struct {
	listener net.Listener
	conn     net.Conn
	udpConn  net.Conn
	udp6Conn *net.UDPConn
	udpDone  func()
}

func newTracerEventGenerator(flowi6 bool) (*tracerEventGenerator, error) {
	eg := &tracerEventGenerator{}

	// port 0 means we let the kernel choose a free port
	var err error
	addr := fmt.Sprintf("%s:0", listenIPv4)
	eg.listener, err = net.Listen("tcp4", addr)
	if err != nil {
		return nil, err
	}

	go acceptHandler(eg.listener)

	// Spin up UDP server
	var udpAddr string
	udpAddr, eg.udpDone, err = newUDPServer(addr)
	if err != nil {
		eg.Close()
		return nil, err
	}

	// Establish connection that will be used in the offset guessing
	eg.conn, err = net.Dial(eg.listener.Addr().Network(), eg.listener.Addr().String())
	if err != nil {
		eg.Close()
		return nil, err
	}

	eg.udpConn, err = net.Dial("udp", udpAddr)
	if err != nil {
		eg.Close()
		return nil, err
	}

	eg.udp6Conn, err = getUDP6Conn(flowi6)
	if err != nil {
		eg.Close()
		return nil, err
	}

	return eg, nil
}

func getUDP6Conn(flowi6 bool) (*net.UDPConn, error) {
	if !flowi6 {
		return nil, nil
	}

	linkLocals, err := GetIPv6LinkLocalAddress()
	if err != nil {
		// TODO: Find a offset guessing method that doesn't need an available IPv6 interface
		log.Debugf("unable to find ipv6 device for udp6 flow offset guessing. unconnected udp6 flows won't be traced: %s", err)
		return nil, nil
	}
	var conn *net.UDPConn
	for _, linkLocalAddr := range linkLocals {
		conn, err = net.ListenUDP("udp6", linkLocalAddr)
		if err == nil {
			return conn, err
		}
	}
	return nil, err
}

// Generate an event for offset guessing
func (e *tracerEventGenerator) Generate(status GuessWhat, expected *fieldValues) error {
	// Are we guessing the IPv6 field?
	if status == GuessDAddrIPv6 {
		// For ipv6, we don't need the source port because we already guessed it doing ipv4 connections so
		// we use a random destination address and try to connect to it.
		var err error
		addr := generateRandomIPv6Address()
		expected.daddrIPv6, err = uint32ArrayFromIPv6(addr)
		if err != nil {
			return err
		}

		bindAddress := fmt.Sprintf("[%s]:9092", addr.String())

		// Since we connect to a random IP, this will most likely fail. In the unlikely case where it connects
		// successfully, we close the connection to avoid a leak.
		if conn, err := net.DialTimeout("tcp6", bindAddress, 10*time.Millisecond); err == nil {
			conn.Close()
		}

		return nil
	} else if status == GuessSAddrFl4 ||
		status == GuessDAddrFl4 ||
		status == GuessSPortFl4 ||
		status == GuessDPortFl4 ||
		status == GuessSKBuffSock ||
		status == GuessSKBuffTransportHeader ||
		status == GuessSKBuffHead {
		payload := []byte("test")
		_, err := e.udpConn.Write(payload)

		return err
	} else if e.udp6Conn != nil &&
		(status == GuessSAddrFl6 ||
			status == GuessDAddrFl6 ||
			status == GuessSPortFl6 ||
			status == GuessDPortFl6) {
		payload := []byte("test")
		remoteAddr := &net.UDPAddr{IP: net.ParseIP(InterfaceLocalMulticastIPv6), Port: 53}
		_, err := e.udp6Conn.WriteTo(payload, remoteAddr)
		if err != nil {
			return err
		}

		expected.daddrFl6, err = uint32ArrayFromIPv6(remoteAddr.IP)
		if err != nil {
			return err
		}
		expected.dportFl6 = uint16(remoteAddr.Port)

		return nil
	}

	// This triggers the KProbe handler attached to `tcp_getsockopt`
	_, err := TcpGetInfo(e.conn)
	return err
}

func (e *tracerEventGenerator) populateUDPExpectedValues(expected *fieldValues) error {
	saddr, daddr, sport, dport, err := extractIPsAndPorts(e.udpConn)
	if err != nil {
		return err
	}
	expected.saddrFl4 = saddr
	expected.sportFl4 = sport
	expected.daddrFl4 = daddr
	expected.dportFl4 = dport

	if e.udp6Conn != nil {
		saddr6, sport6, err := extractIPv6AddressAndPort(e.udp6Conn.LocalAddr())
		if err != nil {
			return err
		}
		expected.saddrFl6 = saddr6
		expected.sportFl6 = sport6
	}

	return nil
}

func (e *tracerEventGenerator) Close() {
	if e.conn != nil {
		e.conn.Close()
	}

	if e.listener != nil {
		_ = e.listener.Close()
	}

	if e.udpConn != nil {
		_ = e.udpConn.Close()
	}

	if e.udp6Conn != nil {
		_ = e.udp6Conn.Close()
	}

	if e.udpDone != nil {
		e.udpDone()
	}
}

func acceptHandler(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}

		_, _ = io.Copy(io.Discard, conn)
		if tcpc, ok := conn.(*net.TCPConn); ok {
			_ = tcpc.SetLinger(0)
		}
		conn.Close()
	}
}

// TcpGetInfo obtains information from a TCP socket via GETSOCKOPT(2) system call.
// The motivation for using this is twofold: 1) it is a way of triggering the kprobe
// responsible for the V4 offset guessing in kernel-space and 2) using it we can obtain
// in user-space TCP socket information such as RTT and use it for setting the expected
// values in the `fieldValues` struct.
func TcpGetInfo(conn net.Conn) (*unix.TCPInfo, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("not a TCPConn")
	}

	sysc, err := tcpConn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("error getting syscall connection: %w", err)
	}

	var tcpInfo *unix.TCPInfo
	ctrlErr := sysc.Control(func(fd uintptr) {
		tcpInfo, err = unix.GetsockoptTCPInfo(int(fd), syscall.SOL_TCP, syscall.TCP_INFO)
	})
	if err != nil {
		return nil, fmt.Errorf("error calling syscall.SYS_GETSOCKOPT: %w", err)
	}
	if ctrlErr != nil {
		return nil, fmt.Errorf("error controlling TCP connection: %w", ctrlErr)
	}
	return tcpInfo, nil
}

func (t *tracerOffsetGuesser) logAndAdvance(offset uint64, next GuessWhat) {
	guess := GuessWhat(t.status.What)
	if offset != notApplicable {
		log.Debugf("Successfully guessed %v with offset of %d bytes", whatString[guess], offset)
	} else {
		log.Debugf("Could not guess offset for %v", whatString[guess])
	}
	if next != GuessNotApplicable {
		log.Debugf("Started offset guessing for %v", whatString[next])
		t.status.What = uint64(next)
	}
}

func newUDPServer(addr string) (string, func(), error) {
	ln, err := net.ListenPacket("udp", addr)
	if err != nil {
		return "", nil, err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		b := make([]byte, 10)
		for {
			_ = ln.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			_, _, err := ln.ReadFrom(b)
			if err != nil && !os.IsTimeout(err) {
				return
			}
		}
	}()

	doneFn := func() {
		_ = ln.Close()
		<-done
	}
	return ln.LocalAddr().String(), doneFn, nil
}

var TracerOffsets tracerOffsets

type tracerOffsets struct {
	offsets []manager.ConstantEditor
	err     error
}

func boolConst(name string, value bool) manager.ConstantEditor {
	c := manager.ConstantEditor{
		Name:  name,
		Value: uint64(1),
	}
	if !value {
		c.Value = uint64(0)
	}

	return c
}

func (o *tracerOffsets) Offsets(cfg *config.Config) ([]manager.ConstantEditor, error) {
	fromConfig := func(c *config.Config, offsets []manager.ConstantEditor) []manager.ConstantEditor {
		var foundTcp, foundUdp bool
		for o := range offsets {
			switch offsets[o].Name {
			case "tcpv6_enabled":
				offsets[o] = boolConst("tcpv6_enabled", c.CollectTCPv6Conns)
				foundTcp = true
			case "udpv6_enabled":
				offsets[o] = boolConst("udpv6_enabled", c.CollectUDPv6Conns)
				foundUdp = true
			}
			if foundTcp && foundUdp {
				break
			}
		}
		if !foundTcp {
			offsets = append(offsets, boolConst("tcpv6_enabled", c.CollectTCPv6Conns))
		}
		if !foundUdp {
			offsets = append(offsets, boolConst("udpv6_enabled", c.CollectUDPv6Conns))
		}

		return offsets
	}

	if o.err != nil {
		return nil, o.err
	}

	if cfg.CollectUDPv6Conns {
		kv, err := kernel.HostVersion()
		if err != nil {
			return nil, err
		}

		if kv >= kernel.VersionCode(5, 18, 0) {
			_cfg := *cfg
			_cfg.CollectUDPv6Conns = false
			cfg = &_cfg
		}
	}

	if len(o.offsets) > 0 {
		// already run
		return fromConfig(cfg, o.offsets), o.err
	}

	offsetBuf, err := netebpf.ReadOffsetBPFModule(cfg.BPFDir, cfg.BPFDebug)
	if err != nil {
		o.err = fmt.Errorf("could not read offset bpf module: %s", err)
		return nil, o.err
	}
	defer offsetBuf.Close()

	o.offsets, o.err = RunOffsetGuessing(cfg, offsetBuf, NewTracerOffsetGuesser)
	return fromConfig(cfg, o.offsets), o.err
}

func (o *tracerOffsets) Reset() {
	o.err = nil
	o.offsets = nil
}
