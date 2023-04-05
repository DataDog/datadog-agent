// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"math"
	"net"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	manager "github.com/DataDog/ebpf-manager"
)

//go:generate go run ../../../pkg/ebpf/include_headers.go ../../../pkg/network/ebpf/c/runtime/offsetguess-test.c ../../../pkg/ebpf/bytecode/build/runtime/offsetguess-test.c ../../../pkg/ebpf/c ../../../pkg/ebpf/c/protocols ../../../pkg/network/ebpf/c/runtime ../../../pkg/network/ebpf/c
//go:generate go run ../../../pkg/ebpf/bytecode/runtime/integrity.go ../../../pkg/ebpf/bytecode/build/runtime/offsetguess-test.c ../../../pkg/ebpf/bytecode/runtime/offsetguess-test.go runtime

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
	offsetCtStatus
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
	case offsetCtStatus:
		return "offset_ct_status"
	case offsetCtNetns:
		return "offset_ct_netns"
	case offsetCtIno:
		return "offset_ct_ino"
	}

	return "unknown offset"
}

func TestOffsetGuess(t *testing.T) {
	cfg := testConfig()
	if !cfg.EnableRuntimeCompiler {
		t.Skip("runtime compilation is not enabled")
	}

	offsetBuf, err := netebpf.ReadOffsetBPFModule(cfg.BPFDir, cfg.BPFDebug)
	require.NoError(t, err, "could not read offset bpf module")
	t.Cleanup(func() { offsetBuf.Close() })

	_consts, err := runOffsetGuessing(cfg, offsetBuf, offsetguess.NewTracerOffsetGuesser)
	require.NoError(t, err)
	cts, err := runOffsetGuessing(cfg, offsetBuf, func() (offsetguess.OffsetGuesser, error) {
		return offsetguess.NewConntrackOffsetGuesser(_consts)
	})
	require.NoError(t, err)
	_consts = append(_consts, cts...)

	consts := map[offsetT]uint64{}
	for _, c := range _consts {
		value := c.Value.(uint64)
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
		case "offset_ct_status":
			consts[offsetCtStatus] = value
		case "offset_ct_netns":
			consts[offsetCtNetns] = value
		case "offset_ct_ino":
			consts[offsetCtIno] = value
		}
	}

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

	server := NewTCPServer(func(c net.Conn) {})
	require.NoError(t, server.Run())
	t.Cleanup(func() { server.Shutdown() })

	var c net.Conn
	require.Eventually(t, func() bool {
		c, err = net.Dial("tcp4", server.address)
		if err == nil {
			return true
		}

		return false
	}, time.Second, 100*time.Millisecond)
	t.Cleanup(func() { c.Close() })

	f, err := c.(*net.TCPConn).File()
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	_, err = unix.GetsockoptByte(int(f.Fd()), unix.IPPROTO_TCP, unix.TCP_INFO)
	require.NoError(t, err)

	mp, _, err := mgr.GetMap("offsets")
	require.NoError(t, err)

	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	for o := offsetSaddr; o < offsetMax; o++ {
		switch o {
		case offsetSkBuffHead, offsetSkBuffSock, offsetSkBuffTransportHeader:
			if !kprobe.ClassificationSupported(cfg) {
				continue
			}
		case offsetSaddrFl6, offsetDaddrFl6, offsetSportFl6, offsetDportFl6:
			// TODO: offset guessing for these fields is currently broken on kernels 5.18+
			// see https://datadoghq.atlassian.net/browse/NET-2984
			if kv >= kernel.VersionCode(5, 18, 0) {
				continue
			}
		}

		var offset uint64
		var name offsetT = o
		require.NoError(t, mp.Lookup(unsafe.Pointer(&name), unsafe.Pointer(&offset)))
		assert.Equal(t, offset, consts[o], "unexpected offset for %s", o)
	}
}
