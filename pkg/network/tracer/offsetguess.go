// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
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
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/native"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	// The source port is much further away in the inet sock.
	thresholdInetSock = 2000

	notApplicable = 99999 // An arbitrary large number to indicate that the value should be ignored
)

var stateString = map[netebpf.TracerState]string{
	netebpf.StateUninitialized: "uninitialized",
	netebpf.StateChecking:      "checking",
	netebpf.StateChecked:       "checked",
	netebpf.StateReady:         "ready",
}

// These constants should be in sync with the equivalent definitions in the ebpf program.
const (
	disabled uint8 = 0
	enabled  uint8 = 1
)

var whatString = map[netebpf.GuessWhat]string{
	netebpf.GuessSAddr:     "source address",
	netebpf.GuessDAddr:     "destination address",
	netebpf.GuessFamily:    "family",
	netebpf.GuessSPort:     "source port",
	netebpf.GuessDPort:     "destination port",
	netebpf.GuessNetNS:     "network namespace",
	netebpf.GuessRTT:       "Round Trip Time",
	netebpf.GuessDAddrIPv6: "destination address IPv6",

	// Guess offsets in struct flowi4
	netebpf.GuessSAddrFl4: "source address flowi4",
	netebpf.GuessDAddrFl4: "destination address flowi4",
	netebpf.GuessSPortFl4: "source port flowi4",
	netebpf.GuessDPortFl4: "destination port flowi4",

	// Guess offsets in struct flowi6
	netebpf.GuessSAddrFl6: "source address flowi6",
	netebpf.GuessDAddrFl6: "destination address flowi6",
	netebpf.GuessSPortFl6: "source port flowi6",
	netebpf.GuessDPortFl6: "destination port flowi6",

	netebpf.GuessSocketSK:              "sk field on struct socket",
	netebpf.GuessSKBuffSock:            "sk field on struct sk_buff",
	netebpf.GuessSKBuffTransportHeader: "transport header field on struct sk_buff",
	netebpf.GuessSKBuffHead:            "head field on struct sk_buff",
}

const (
	tcpGetSockOptKProbeNotCalled uint64 = 0
	tcpGetSockOptKProbeCalled    uint64 = 1
)

var tcpKprobeCalledString = map[uint64]string{
	tcpGetSockOptKProbeNotCalled: "tcp_getsockopt kprobe not executed",
	tcpGetSockOptKProbeCalled:    "tcp_getsockopt kprobe executed",
}

const listenIPv4 = "127.0.0.2"
const interfaceLocalMulticastIPv6 = "ff01::1"

var zero uint64

type fieldValues struct {
	saddr     uint32
	daddr     uint32
	sport     uint16
	dport     uint16
	netns     uint32
	family    uint16
	rtt       uint32
	rttVar    uint32
	daddrIPv6 [4]uint32

	// Used for guessing offsets in struct flowi4
	saddrFl4 uint32
	daddrFl4 uint32
	sportFl4 uint16
	dportFl4 uint16

	// Used for guessing offsets in struct flowi6
	saddrFl6 [4]uint32
	daddrFl6 [4]uint32
	sportFl6 uint16
	dportFl6 uint16
}

func idPair(name probes.ProbeFuncName) manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		EBPFFuncName: name,
		UID:          "offset",
	}
}

func newOffsetManager() *manager.Manager {
	return &manager.Manager{
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
			{ProbeIdentificationPair: idPair(probes.IP6MakeSkbPre470), MatchFuncName: "^ip6_make_skb$"},
			{ProbeIdentificationPair: idPair(probes.TCPv6ConnectReturn), KProbeMaxActive: 128},
			{ProbeIdentificationPair: idPair(probes.NetDevQueue)},
		},
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
	netns, err := ownNetNS()
	if err != nil {
		return nil, err
	}

	tcpInfo, err := tcpGetInfo(conn)
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
		netns:  uint32(netns),
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

func enableProbe(enabled map[probes.ProbeFuncName]struct{}, name probes.ProbeFuncName) {
	enabled[name] = struct{}{}
}

func offsetGuessProbes(c *config.Config) (map[probes.ProbeFuncName]struct{}, error) {
	p := map[probes.ProbeFuncName]struct{}{}
	enableProbe(p, probes.TCPGetSockOpt)
	enableProbe(p, probes.SockGetSockOpt)
	enableProbe(p, probes.IPMakeSkb)
	if kprobe.ClassificationSupported(c) {
		enableProbe(p, probes.NetDevQueue)
	}

	if c.CollectIPv6Conns {
		enableProbe(p, probes.TCPv6Connect)
		enableProbe(p, probes.TCPv6ConnectReturn)

		kv, err := kernel.HostVersion()
		if err != nil {
			return nil, err
		}

		if kv < kernel.VersionCode(4, 7, 0) {
			enableProbe(p, probes.IP6MakeSkbPre470)
		} else {
			enableProbe(p, probes.IP6MakeSkb)
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

func ownNetNS() (uint64, error) {
	var s syscall.Stat_t
	if err := syscall.Stat("/proc/self/ns/net", &s); err != nil {
		return 0, err
	}
	return s.Ino, nil
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
	rand.Read(addr[4:])

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

func getIPv6LinkLocalAddress() (*net.UDPAddr, error) {
	ints, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, i := range ints {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if strings.HasPrefix(a.String(), "fe80::") {
				// this address *may* have CIDR notation
				if ar, _, err := net.ParseCIDR(a.String()); err == nil {
					return &net.UDPAddr{IP: ar, Zone: i.Name}, nil
				}
				return &net.UDPAddr{IP: net.ParseIP(a.String()), Zone: i.Name}, nil
			}
		}
	}
	return nil, fmt.Errorf("no IPv6 link local address found")
}

// checkAndUpdateCurrentOffset checks the value for the current offset stored
// in the eBPF map against the expected value, incrementing the offset if it
// doesn't match, or going to the next field to guess if it does
func checkAndUpdateCurrentOffset(mp *ebpf.Map, status *netebpf.TracerStatus, expected *fieldValues, maxRetries *int, threshold uint64, protocolClassificationSupported bool) error {
	// get the updated map value so we can check if the current offset is
	// the right one
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return fmt.Errorf("error reading tracer_status: %v", err)
	}

	if netebpf.TracerState(status.State) != netebpf.StateChecked {
		if *maxRetries == 0 {
			return fmt.Errorf("invalid guessing state while guessing %v, got %v expected %v. %v",
				whatString[netebpf.GuessWhat(status.What)], stateString[netebpf.TracerState(status.State)], stateString[netebpf.StateChecked], tcpKprobeCalledString[status.Tcp_info_kprobe_status])
		}
		*maxRetries--
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	switch netebpf.GuessWhat(status.What) {
	case netebpf.GuessSAddr:
		if status.Saddr == expected.saddr {
			logAndAdvance(status, status.Offset_saddr, netebpf.GuessDAddr)
			break
		}
		status.Offset_saddr++
		status.Saddr = expected.saddr
	case netebpf.GuessDAddr:
		if status.Daddr == expected.daddr {
			logAndAdvance(status, status.Offset_daddr, netebpf.GuessDPort)
			break
		}
		status.Offset_daddr++
		status.Daddr = expected.daddr
	case netebpf.GuessDPort:
		if status.Dport == htons(expected.dport) {
			logAndAdvance(status, status.Offset_dport, netebpf.GuessFamily)
			// we know the family ((struct __sk_common)->skc_family) is
			// after the skc_dport field, so we start from there
			status.Offset_family = status.Offset_dport
			break
		}
		status.Offset_dport++
	case netebpf.GuessFamily:
		if status.Family == expected.family {
			logAndAdvance(status, status.Offset_family, netebpf.GuessSPort)
			// we know the sport ((struct inet_sock)->inet_sport) is
			// after the family field, so we start from there
			status.Offset_sport = status.Offset_family
			break
		}
		status.Offset_family++
	case netebpf.GuessSPort:
		if status.Sport == htons(expected.sport) {
			logAndAdvance(status, status.Offset_sport, netebpf.GuessSAddrFl4)
			break
		}
		status.Offset_sport++
	case netebpf.GuessSAddrFl4:
		if status.Saddr_fl4 == expected.saddrFl4 {
			logAndAdvance(status, status.Offset_saddr_fl4, netebpf.GuessDAddrFl4)
			break
		}
		status.Offset_saddr_fl4++
		if status.Offset_saddr_fl4 >= threshold {
			// Let's skip all other flowi4 fields
			logAndAdvance(status, notApplicable, flowi6EntryState(status))
			status.Fl4_offsets = disabled
			break
		}
	case netebpf.GuessDAddrFl4:
		if status.Daddr_fl4 == expected.daddrFl4 {
			logAndAdvance(status, status.Offset_daddr_fl4, netebpf.GuessSPortFl4)
			break
		}
		status.Offset_daddr_fl4++
		if status.Offset_daddr_fl4 >= threshold {
			logAndAdvance(status, notApplicable, flowi6EntryState(status))
			status.Fl4_offsets = disabled
			break
		}
	case netebpf.GuessSPortFl4:
		if status.Sport_fl4 == htons(expected.sportFl4) {
			logAndAdvance(status, status.Offset_sport_fl4, netebpf.GuessDPortFl4)
			break
		}
		status.Offset_sport_fl4++
		if status.Offset_sport_fl4 >= threshold {
			logAndAdvance(status, notApplicable, flowi6EntryState(status))
			status.Fl4_offsets = disabled
			break
		}
	case netebpf.GuessDPortFl4:
		if status.Dport_fl4 == htons(expected.dportFl4) {
			logAndAdvance(status, status.Offset_dport_fl4, flowi6EntryState(status))
			status.Fl4_offsets = enabled
			break
		}
		status.Offset_dport_fl4++
		if status.Offset_dport_fl4 >= threshold {
			logAndAdvance(status, notApplicable, flowi6EntryState(status))
			status.Fl4_offsets = disabled
			break
		}
	case netebpf.GuessSAddrFl6:
		if compareIPv6(status.Saddr_fl6, expected.saddrFl6) {
			logAndAdvance(status, status.Offset_saddr_fl6, netebpf.GuessDAddrFl6)
			break
		}
		status.Offset_saddr_fl6++
		if status.Offset_saddr_fl6 >= threshold {
			// Let's skip all other flowi6 fields
			logAndAdvance(status, notApplicable, netebpf.GuessNetNS)
			status.Fl6_offsets = disabled
			break
		}
	case netebpf.GuessDAddrFl6:
		if compareIPv6(status.Daddr_fl6, expected.daddrFl6) {
			logAndAdvance(status, status.Offset_daddr_fl6, netebpf.GuessSPortFl6)
			break
		}
		status.Offset_daddr_fl6++
		if status.Offset_daddr_fl6 >= threshold {
			logAndAdvance(status, notApplicable, netebpf.GuessNetNS)
			status.Fl6_offsets = disabled
			break
		}
	case netebpf.GuessSPortFl6:
		if status.Sport_fl6 == htons(expected.sportFl6) {
			logAndAdvance(status, status.Offset_sport_fl6, netebpf.GuessDPortFl6)
			break
		}
		status.Offset_sport_fl6++
		if status.Offset_sport_fl6 >= threshold {
			logAndAdvance(status, notApplicable, netebpf.GuessNetNS)
			status.Fl6_offsets = disabled
			break
		}
	case netebpf.GuessDPortFl6:
		if status.Dport_fl6 == htons(expected.dportFl6) {
			logAndAdvance(status, status.Offset_dport_fl6, netebpf.GuessNetNS)
			status.Fl6_offsets = enabled
			break
		}
		status.Offset_dport_fl6++
		if status.Offset_dport_fl6 >= threshold {
			logAndAdvance(status, notApplicable, netebpf.GuessNetNS)
			status.Fl6_offsets = disabled
			break
		}
	case netebpf.GuessNetNS:
		if status.Netns == expected.netns {
			logAndAdvance(status, status.Offset_netns, netebpf.GuessRTT)
			break
		}
		status.Offset_ino++
		// go to the next offset_netns if we get an error
		if status.Err != 0 || status.Offset_ino >= threshold {
			status.Offset_ino = 0
			status.Offset_netns++
		}
	case netebpf.GuessRTT:
		// For more information on the bit shift operations see:
		// https://elixir.bootlin.com/linux/v4.6/source/net/ipv4/tcp.c#L2686
		if status.Rtt>>3 == expected.rtt && status.Rtt_var>>2 == expected.rttVar {
			logAndAdvance(status, status.Offset_rtt, netebpf.GuessSocketSK)
			break
		}
		// We know that these two fields are always next to each other, 4 bytes apart:
		// https://elixir.bootlin.com/linux/v4.6/source/include/linux/tcp.h#L232
		// rtt -> srtt_us
		// rtt_var -> mdev_us
		status.Offset_rtt++
		status.Offset_rtt_var = status.Offset_rtt + 4

	case netebpf.GuessSocketSK:
		if status.Sport_via_sk == htons(expected.sport) && status.Dport_via_sk == htons(expected.dport) {
			// if protocol classification is disabled, its hooks will not be activated, and thus we should skip
			// the guessing of their relevant offsets. The problem is with compatibility with older kernel versions
			// where `struct sk_buff` have changed, and it does not match our current guessing.
			next := netebpf.GuessSKBuffSock
			if !protocolClassificationSupported {
				next = netebpf.GuessDAddrIPv6
			}
			logAndAdvance(status, status.Offset_socket_sk, next)
			break
		}
		status.Offset_socket_sk++
	case netebpf.GuessSKBuffSock:
		if status.Sport_via_sk_via_sk_buf == htons(expected.sportFl4) && status.Dport_via_sk_via_sk_buf == htons(expected.dportFl4) {
			logAndAdvance(status, status.Offset_sk_buff_sock, netebpf.GuessSKBuffTransportHeader)
			break
		}
		status.Offset_sk_buff_sock++
	case netebpf.GuessSKBuffTransportHeader:
		networkDiffFromMac := status.Network_header - status.Mac_header
		transportDiffFromNetwork := status.Transport_header - status.Network_header
		if networkDiffFromMac == 14 && transportDiffFromNetwork == 20 {
			logAndAdvance(status, status.Offset_sk_buff_transport_header, netebpf.GuessSKBuffHead)
			break
		}
		status.Offset_sk_buff_transport_header++
	case netebpf.GuessSKBuffHead:
		if status.Sport_via_sk_via_sk_buf == htons(expected.sportFl4) && status.Dport_via_sk_via_sk_buf == htons(expected.dportFl4) {
			logAndAdvance(status, status.Offset_sk_buff_head, netebpf.GuessDAddrIPv6)
			break
		}
		status.Offset_sk_buff_head++
	case netebpf.GuessDAddrIPv6:
		if compareIPv6(status.Daddr_ipv6, expected.daddrIPv6) {
			logAndAdvance(status, status.Offset_rtt, netebpf.GuessNotApplicable)
			// at this point, we've guessed all the offsets we need,
			// set the status to "stateReady"
			return setReadyState(mp, status)
		}
		status.Offset_daddr_ipv6++
	default:
		return fmt.Errorf("unexpected field to guess: %v", whatString[netebpf.GuessWhat(status.What)])
	}

	// This assumes `GuessDAddrIPv6` is the last stage of the process.
	if status.What == uint64(netebpf.GuessDAddrIPv6) && status.Ipv6_enabled == disabled {
		return setReadyState(mp, status)
	}

	status.State = uint64(netebpf.StateChecking)
	// update the map with the new offset/field to check
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}

	return nil
}

func setReadyState(mp *ebpf.Map, status *netebpf.TracerStatus) error {
	status.State = uint64(netebpf.StateReady)
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}
	return nil
}

func flowi6EntryState(status *netebpf.TracerStatus) netebpf.GuessWhat {
	if status.Ipv6_enabled == disabled {
		return netebpf.GuessNetNS
	}
	return netebpf.GuessSAddrFl6
}

// guessOffsets expects manager.Manager to contain a map named tracer_status and helps initialize the
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
func guessOffsets(m *manager.Manager, cfg *config.Config) ([]manager.ConstantEditor, error) {
	mp, _, err := m.GetMap(probes.TracerStatusMap)
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
	if len(processName) > netebpf.ProcCommMaxLen { // Truncate process name if needed
		processName = processName[:netebpf.ProcCommMaxLen]
	}

	cProcName := [netebpf.ProcCommMaxLen + 1]int8{} // Last char has to be null character, so add one
	for i, ch := range processName {
		cProcName[i] = int8(ch)
	}

	status := &netebpf.TracerStatus{
		State:        uint64(netebpf.StateChecking),
		Proc:         netebpf.Proc{Comm: cProcName},
		Ipv6_enabled: enabled,
		Fl6_offsets:  enabled,
	}
	if !cfg.CollectIPv6Conns {
		status.Ipv6_enabled = disabled
		status.Fl6_offsets = disabled
	}

	// if we already have the offsets, just return
	err = mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(status))
	if err == nil && netebpf.TracerState(status.State) == netebpf.StateReady {
		return getConstantEditors(status), nil
	}

	eventGenerator, err := newEventGenerator(cfg.CollectIPv6Conns)
	if err != nil {
		return nil, err
	}
	defer eventGenerator.Close()

	if eventGenerator.udp6Conn == nil {
		status.Fl6_offsets = disabled
	}

	// initialize map
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return nil, fmt.Errorf("error initializing tracer_status map: %v", err)
	}

	// If the kretprobe for tcp_v4_connect() is configured with a too-low maxactive, some kretprobe might be missing.
	// In this case, we detect it and try again. See: https://github.com/weaveworks/tcptracer-bpf/issues/24
	maxRetries := 100

	// Retrieve expected values from local connection
	expected, err := waitUntilStable(eventGenerator.conn, 200*time.Millisecond, 5)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving expected value")
	}

	err = eventGenerator.populateUDPExpectedValues(expected)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving expected value")
	}

	protocolClassificationSupported := kprobe.ClassificationSupported(cfg)
	log.Debugf("Checking for offsets with threshold of %d", threshold)
	for netebpf.TracerState(status.State) != netebpf.StateReady {
		if err := eventGenerator.Generate(status, expected); err != nil {
			return nil, err
		}

		if err := checkAndUpdateCurrentOffset(mp, status, expected, &maxRetries, threshold, protocolClassificationSupported); err != nil {
			return nil, err
		}

		// Stop at a reasonable offset so we don't run forever.
		// Reading too far away in kernel memory is not a big deal:
		// probe_kernel_read() handles faults gracefully.
		if status.Offset_saddr >= threshold || status.Offset_daddr >= threshold ||
			status.Offset_sport >= thresholdInetSock || status.Offset_dport >= threshold ||
			status.Offset_netns >= threshold || status.Offset_family >= threshold ||
			status.Offset_daddr_ipv6 >= threshold || status.Offset_rtt >= thresholdInetSock ||
			status.Offset_socket_sk >= threshold || status.Offset_sk_buff_sock >= threshold ||
			status.Offset_sk_buff_transport_header >= threshold || status.Offset_sk_buff_head >= threshold {
			return nil, fmt.Errorf("overflow while guessing %v, bailing out", whatString[netebpf.GuessWhat(status.What)])
		}
	}

	return getConstantEditors(status), nil
}

func getConstantEditors(status *netebpf.TracerStatus) []manager.ConstantEditor {
	return []manager.ConstantEditor{
		{Name: "offset_saddr", Value: status.Offset_saddr},
		{Name: "offset_daddr", Value: status.Offset_daddr},
		{Name: "offset_sport", Value: status.Offset_sport},
		{Name: "offset_dport", Value: status.Offset_dport},
		{Name: "offset_netns", Value: status.Offset_netns},
		{Name: "offset_ino", Value: status.Offset_ino},
		{Name: "offset_family", Value: status.Offset_family},
		{Name: "offset_rtt", Value: status.Offset_rtt},
		{Name: "offset_rtt_var", Value: status.Offset_rtt_var},
		{Name: "offset_daddr_ipv6", Value: status.Offset_daddr_ipv6},
		{Name: "ipv6_enabled", Value: uint64(status.Ipv6_enabled)},
		{Name: "offset_saddr_fl4", Value: status.Offset_saddr_fl4},
		{Name: "offset_daddr_fl4", Value: status.Offset_daddr_fl4},
		{Name: "offset_sport_fl4", Value: status.Offset_sport_fl4},
		{Name: "offset_dport_fl4", Value: status.Offset_dport_fl4},
		{Name: "fl4_offsets", Value: uint64(status.Fl4_offsets)},
		{Name: "offset_saddr_fl6", Value: status.Offset_saddr_fl6},
		{Name: "offset_daddr_fl6", Value: status.Offset_daddr_fl6},
		{Name: "offset_sport_fl6", Value: status.Offset_sport_fl6},
		{Name: "offset_dport_fl6", Value: status.Offset_dport_fl6},
		{Name: "fl6_offsets", Value: uint64(status.Fl6_offsets)},
		{Name: "offset_socket_sk", Value: status.Offset_socket_sk},
		{Name: "offset_sk_buff_sock", Value: status.Offset_sk_buff_sock},
		{Name: "offset_sk_buff_transport_header", Value: status.Offset_sk_buff_transport_header},
		{Name: "offset_sk_buff_head", Value: status.Offset_sk_buff_head},
	}
}

type eventGenerator struct {
	listener net.Listener
	conn     net.Conn
	udpConn  net.Conn
	udp6Conn *net.UDPConn
	udpDone  func()
}

func newEventGenerator(ipv6 bool) (*eventGenerator, error) {
	eg := &eventGenerator{}

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

	eg.udp6Conn, err = getUDP6Conn(ipv6)
	if err != nil {
		eg.Close()
		return nil, err
	}

	return eg, nil
}

func getUDP6Conn(ipv6 bool) (c *net.UDPConn, err error) {
	if !ipv6 {
		return nil, nil
	}

	// it is necessary to run the following code in
	// the root network namespace, as the network
	// namespace where we're running may not have
	// ipv6 enabled. we are here because the host
	// does have ipv6 enabled (ipv6 bool is true).
	// if system-probe's network namespace does
	// not have ipv6 enabled, the below code will fail
	// since none of the interfaces will have an
	// ipv6 link local address assigned
	err = util.WithRootNS(util.GetProcRoot(), func() error {
		linkLocal, err := getIPv6LinkLocalAddress()
		if err != nil {
			// TODO: Find a offset guessing method that doesn't need an available IPv6 interface
			log.Warnf("unable to find ipv6 device for udp6 flow offset guessing. unconnected udp6 flows won't be traced: %s", err)
			return nil
		}

		c, err = net.ListenUDP("udp6", linkLocal)
		return err
	})

	return c, err
}

// Generate an event for offset guessing
func (e *eventGenerator) Generate(status *netebpf.TracerStatus, expected *fieldValues) error {
	// Are we guessing the IPv6 field?
	if netebpf.GuessWhat(status.What) == netebpf.GuessDAddrIPv6 {
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
	} else if netebpf.GuessWhat(status.What) == netebpf.GuessSAddrFl4 ||
		netebpf.GuessWhat(status.What) == netebpf.GuessDAddrFl4 ||
		netebpf.GuessWhat(status.What) == netebpf.GuessSPortFl4 ||
		netebpf.GuessWhat(status.What) == netebpf.GuessDPortFl4 ||
		netebpf.GuessWhat(status.What) == netebpf.GuessSKBuffSock ||
		netebpf.GuessWhat(status.What) == netebpf.GuessSKBuffTransportHeader ||
		netebpf.GuessWhat(status.What) == netebpf.GuessSKBuffHead {
		payload := []byte("test")
		_, err := e.udpConn.Write(payload)

		return err
	} else if e.udp6Conn != nil &&
		(netebpf.GuessWhat(status.What) == netebpf.GuessSAddrFl6 ||
			netebpf.GuessWhat(status.What) == netebpf.GuessDAddrFl6 ||
			netebpf.GuessWhat(status.What) == netebpf.GuessSPortFl6 ||
			netebpf.GuessWhat(status.What) == netebpf.GuessDPortFl6) {
		payload := []byte("test")
		remoteAddr := &net.UDPAddr{IP: net.ParseIP(interfaceLocalMulticastIPv6), Port: 53}
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
	_, err := tcpGetInfo(e.conn)
	return err
}

func (e *eventGenerator) populateUDPExpectedValues(expected *fieldValues) error {
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

func (e *eventGenerator) Close() {
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

// tcpGetInfo obtains information from a TCP socket via GETSOCKOPT(2) system call.
// The motivation for using this is twofold: 1) it is a way of triggering the kprobe
// responsible for the V4 offset guessing in kernel-space and 2) using it we can obtain
// in user-space TCP socket information such as RTT and use it for setting the expected
// values in the `fieldValues` struct.
func tcpGetInfo(conn net.Conn) (*unix.TCPInfo, error) {
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

func logAndAdvance(status *netebpf.TracerStatus, offset uint64, next netebpf.GuessWhat) {
	guess := netebpf.GuessWhat(status.What)
	if offset != notApplicable {
		log.Debugf("Successfully guessed %v with offset of %d bytes", whatString[guess], offset)
	} else {
		log.Debugf("Could not guess offset for %v", whatString[guess])
	}
	if next != netebpf.GuessNotApplicable {
		log.Debugf("Started offset guessing for %v", whatString[next])
		status.What = uint64(next)
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
