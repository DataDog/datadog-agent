// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"fmt"
	"math"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	tracertestutil "github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

//go:generate $GOPATH/bin/include_headers pkg/network/ebpf/c/runtime/offsetguess-test.c pkg/ebpf/bytecode/build/runtime/offsetguess-test.c pkg/ebpf/c pkg/ebpf/c/protocols pkg/network/ebpf/c/runtime pkg/network/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/offsetguess-test.c pkg/ebpf/bytecode/runtime/offsetguess-test.go runtime

type offsetT int

const (
	offsetSaddr offsetT = iota
	offsetDaddr
	offsetSport
	offsetDport
	offsetNetns
	offsetIno
	offsetFamily
	offsetRtt
	offsetRttVar
	offsetDaddrIpv6
	offsetSaddrFl4
	offsetDaddrFl4
	offsetSportFl4
	offsetDportFl4
	offsetSaddrFl6
	offsetDaddrFl6
	offsetSportFl6
	offsetDportFl6
	offsetSocketSk
	offsetSkBuffSock
	offsetSkBuffTransportHeader
	offsetSkBuffHead
	offsetCtOrigin
	offsetCtReply
	offsetCtNetns
	offsetCtIno
	offsetMax
)

func (o offsetT) String() string {
	switch o {
	case offsetSaddr:
		return "offset_saddr"
	case offsetDaddr:
		return "offset_daddr"
	case offsetSport:
		return "offset_sport"
	case offsetDport:
		return "offset_dport"
	case offsetNetns:
		return "offset_netns"
	case offsetIno:
		return "offset_ino"
	case offsetFamily:
		return "offset_family"
	case offsetRtt:
		return "offset_rtt"
	case offsetRttVar:
		return "offset_rtt_var"
	case offsetDaddrIpv6:
		return "offset_daddr_ipv6"
	case offsetSaddrFl4:
		return "offset_saddr_fl4"
	case offsetDaddrFl4:
		return "offset_daddr_fl4"
	case offsetSportFl4:
		return "offset_sport_fl4"
	case offsetDportFl4:
		return "offset_dport_fl4"
	case offsetSaddrFl6:
		return "offset_saddr_fl6"
	case offsetDaddrFl6:
		return "offset_daddr_fl6"
	case offsetSportFl6:
		return "offset_sport_fl6"
	case offsetDportFl6:
		return "offset_dport_fl6"
	case offsetSocketSk:
		return "offset_socket_sk"
	case offsetSkBuffSock:
		return "offset_sk_buff_sock"
	case offsetSkBuffTransportHeader:
		return "offset_sk_buff_transport_header"
	case offsetSkBuffHead:
		return "offset_sk_buff_head"
	case offsetCtOrigin:
		return "offset_ct_origin"
	case offsetCtReply:
		return "offset_ct_reply"
	case offsetCtNetns:
		return "offset_ct_netns"
	case offsetCtIno:
		return "offset_ct_ino"
	}

	return "unknown offset"
}

func TestOffsetGuess(t *testing.T) {
	if prebuilt.IsDeprecated() {
		t.Skip("skipping because prebuilt is deprecated on this platform")
	}
	ebpftest.LogLevel(t, "trace")
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", testOffsetGuess)
}

func testOffsetGuess(t *testing.T) {
	cfg := testConfig()
	buf, err := runtime.OffsetguessTest.Compile(&cfg.Config, getCFlags(cfg), statsd.Client)
	require.NoError(t, err)
	defer buf.Close()

	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: "offsets"},
		},
		PerfMaps: []*manager.PerfMap{},
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe__tcp_getsockopt",
					UID:          "offsetguess",
				},
			},
		},
	}

	opts := manager.Options{
		// Extend RLIMIT_MEMLOCK (8) size
		// On some systems, the default for RLIMIT_MEMLOCK may be as low as 64 bytes.
		// This will result in an EPERM (Operation not permitted) error, when trying to create an eBPF map
		// using bpf(2) with BPF_MAP_CREATE.
		//
		// We are setting the limit to infinity until we have a better handle on the true requirements.
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapEditors: make(map[string]*ebpf.Map),
	}

	require.NoError(t, mgr.InitWithOptions(buf, opts))
	require.NoError(t, mgr.Start())
	t.Cleanup(func() { mgr.Stop(manager.CleanAll) })

	server := tracertestutil.NewTCPServer(func(_ net.Conn) {})
	require.NoError(t, server.Run())
	t.Cleanup(func() { server.Shutdown() })

	var c net.Conn
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var err error
		c, err = net.Dial("tcp4", server.Address())
		assert.NoError(collect, err)
	}, time.Second, 100*time.Millisecond)
	t.Cleanup(func() { c.Close() })

	f, err := c.(*net.TCPConn).File()
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	_, err = unix.GetsockoptByte(int(f.Fd()), unix.IPPROTO_TCP, unix.TCP_INFO)
	require.NoError(t, err)

	mp, err := maps.GetMap[offsetT, uint64](mgr, "offsets")
	require.NoError(t, err)

	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	// offset guessing used to rely on this previously,
	// but doesn't anymore
	cfg.ProtocolClassificationEnabled = false

	offsetBuf, err := netebpf.ReadOffsetBPFModule(cfg.BPFDir, cfg.BPFDebug)
	require.NoError(t, err, "could not read offset bpf module")
	t.Cleanup(func() { offsetBuf.Close() })

	// prebuilt on 5.18+ does not support UDPv6
	if kv >= kernel.VersionCode(5, 18, 0) {
		cfg.CollectUDPv6Conns = false
	}

	offsetguess.TracerOffsets.Reset()
	_consts, err := offsetguess.TracerOffsets.Offsets(cfg)
	require.NoError(t, err)
	cts, err := offsetguess.RunOffsetGuessing(cfg, offsetBuf, func() (offsetguess.OffsetGuesser, error) {
		return offsetguess.NewConntrackOffsetGuesser(cfg)
	})
	_consts = append(_consts, cts...)
	require.NoError(t, err, "guessed offsets: %+v", _consts)

	consts := map[offsetT]uint64{}
	for _, c := range _consts {
		value := c.Value.(uint64)
		t.Logf("Guessed offset %v with value %v", c.Name, value)
		switch c.Name {
		case "offset_saddr":
			consts[offsetSaddr] = value
		case "offset_daddr":
			consts[offsetDaddr] = value
		case "offset_sport":
			consts[offsetSport] = value
		case "offset_dport":
			consts[offsetDport] = value
		case "offset_netns":
			consts[offsetNetns] = value
		case "offset_ino":
			consts[offsetIno] = value
		case "offset_family":
			consts[offsetFamily] = value
		case "offset_rtt":
			consts[offsetRtt] = value
		case "offset_rtt_var":
			consts[offsetRttVar] = value
		case "offset_daddr_ipv6":
			consts[offsetDaddrIpv6] = value
		case "offset_saddr_fl4":
			consts[offsetSaddrFl4] = value
		case "offset_daddr_fl4":
			consts[offsetDaddrFl4] = value
		case "offset_sport_fl4":
			consts[offsetSportFl4] = value
		case "offset_dport_fl4":
			consts[offsetDportFl4] = value
		case "offset_saddr_fl6":
			consts[offsetSaddrFl6] = value
		case "offset_daddr_fl6":
			consts[offsetDaddrFl6] = value
		case "offset_sport_fl6":
			consts[offsetSportFl6] = value
		case "offset_dport_fl6":
			consts[offsetDportFl6] = value
		case "offset_socket_sk":
			consts[offsetSocketSk] = value
		case "offset_sk_buff_sock":
			consts[offsetSkBuffSock] = value
		case "offset_sk_buff_transport_header":
			consts[offsetSkBuffTransportHeader] = value
		case "offset_sk_buff_head":
			consts[offsetSkBuffHead] = value
		case "offset_ct_origin":
			consts[offsetCtOrigin] = value
		case "offset_ct_reply":
			consts[offsetCtReply] = value
		case "offset_ct_netns":
			consts[offsetCtNetns] = value
		case "offset_ct_ino":
			consts[offsetCtIno] = value
		}
	}

	testOffsets := func(t *testing.T, includeOffsets, excludeOffsets []offsetT) {
		for o := offsetSaddr; o < offsetMax; o++ {
			if slices.Contains(excludeOffsets, o) {
				continue
			}

			if len(includeOffsets) > 0 && !slices.Contains(includeOffsets, o) {
				continue
			}

			switch o {
			case offsetSkBuffHead, offsetSkBuffSock, offsetSkBuffTransportHeader:
				if kv < kernel.VersionCode(4, 7, 0) {
					continue
				}
			case offsetSaddrFl6, offsetDaddrFl6, offsetSportFl6, offsetDportFl6:
				// TODO: offset guessing for these fields is currently broken on kernels 5.18+
				// see https://datadoghq.atlassian.net/browse/NET-2984
				if kv >= kernel.VersionCode(5, 18, 0) {
					continue
				}
			case offsetCtOrigin, offsetCtIno, offsetCtNetns, offsetCtReply:
				// offset guessing for conntrack fields is broken on pre-4.14 kernels
				if !ebpfPrebuiltConntrackerSupportedOnKernelT(t) {
					continue
				}
			}

			var offset uint64
			var name offsetT = o
			require.NoError(t, mp.Lookup(&name, &offset))
			assert.Equal(t, offset, consts[o], "unexpected offset for %s", o)
			t.Logf("offset %s expected: %d guessed: %d", o, offset, consts[o])
		}
	}

	t.Run("without RTT offsets", func(t *testing.T) {
		testOffsets(t, nil, []offsetT{offsetRtt, offsetRttVar})
	})

	t.Run("only RTT offsets", func(t *testing.T) {
		flake.Mark(t)
		testOffsets(t, []offsetT{offsetRtt, offsetRttVar}, nil)
	})

}

func TestOffsetGuessPortIPv6Overlap(t *testing.T) {
	if prebuilt.IsDeprecated() {
		t.Skip("skipping because prebuilt is deprecated on this platform")
	}

	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		addrs, err := offsetguess.GetIPv6LinkLocalAddress()
		require.NoError(t, err)

		// force preference for the added prefix. 0x35 (53) = DNS port
		const portMatchingPrefix = "fe80::35:"
		oldPrefix := offsetguess.IPv6LinkLocalPrefix
		t.Cleanup(func() { offsetguess.IPv6LinkLocalPrefix = oldPrefix })
		offsetguess.IPv6LinkLocalPrefix = portMatchingPrefix

		// add IPv6 link-local addresses with 0x35 (53) bytes to each interface
		for i, addr := range addrs {
			// so we capture i and addr.Zone correctly in the closure below
			z := addr.Zone
			ii := i + 1
			_, err := nettestutil.RunCommand(fmt.Sprintf("ip -6 addr add %s%d/64 dev %s scope link nodad", portMatchingPrefix, ii, z))
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err = nettestutil.RunCommand(fmt.Sprintf("ip -6 addr del %s%d/64 dev %s scope link", portMatchingPrefix, ii, z))
				if err != nil {
					t.Logf("remove link-local error: %s\n", err)
				}
			})
		}

		showout, err := nettestutil.RunCommand("ip -6 addr show")
		require.NoError(t, err)
		t.Log(showout)

		testOffsetGuess(t)
	})
}
