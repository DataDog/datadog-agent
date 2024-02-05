// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package netlink

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)
	os.Exit(m.Run())
}

func TestConntrackExists(t *testing.T) {
	ns := testutil.SetupCrossNsDNAT(t)

	tcpCloser := nettestutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 8080, ns)
	defer tcpCloser.Close()

	udpCloser := nettestutil.StartServerUDPNs(t, net.ParseIP("2.2.2.4"), 8080, ns)
	defer udpCloser.Close()

	tcpConn := nettestutil.MustPingTCP(t, net.ParseIP("2.2.2.4"), 80)
	defer tcpConn.Close()

	udpConn := nettestutil.MustPingUDP(t, net.ParseIP("2.2.2.4"), 80)
	defer udpConn.Close()

	testNs, err := netns.GetFromName(ns)
	require.NoError(t, err)
	defer testNs.Close()

	ctrks := map[netns.NsHandle]Conntrack{}
	defer func() {
		for _, ctrk := range ctrks {
			ctrk.Close()
		}
	}()

	tcpLaddr := tcpConn.LocalAddr().(*net.TCPAddr)
	udpLaddr := udpConn.LocalAddr().(*net.UDPAddr)
	// test a combination of (tcp, udp) x (root ns, test ns)
	testConntrackExists(t, tcpLaddr.IP.String(), tcpLaddr.Port, "tcp", testNs, ctrks)
	testConntrackExists(t, udpLaddr.IP.String(), udpLaddr.Port, "udp", testNs, ctrks)
}

func BenchmarkConntrackExists(b *testing.B) {
	ns := testutil.SetupCrossNsDNAT(b)

	tcpCloser := nettestutil.StartServerTCPNs(b, net.ParseIP("2.2.2.4"), 8080, ns)
	defer tcpCloser.Close()

	tcpConn := nettestutil.MustPingTCP(b, net.ParseIP("2.2.2.4"), 80)
	defer tcpConn.Close()

	testNs, err := netns.GetFromName(ns)
	require.NoError(b, err)
	defer testNs.Close()

	ctrks := map[netns.NsHandle]Conntrack{}
	defer func() {
		for _, ctrk := range ctrks {
			ctrk.Close()
		}
	}()

	tcpAddr := tcpConn.LocalAddr().(*net.TCPAddr)
	laddrIP := tcpAddr.IP.String()
	laddrPort := tcpAddr.Port
	rootNs, err := kernel.GetRootNetNamespace("/proc")
	require.NoError(b, err)
	defer rootNs.Close()

	var ipProto uint8 = unix.IPPROTO_TCP
	tests := []struct {
		c  Con
		ns netns.NsHandle
	}{
		{
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
			},
			ns: rootNs,
		},
		{
			c: Con{
				Reply: newIPTuple("2.2.2.4", laddrIP, 80, uint16(laddrPort), ipProto),
			},
			ns: rootNs,
		},
		{
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
			},
			ns: rootNs,
		},
		{
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
			},
			ns: testNs,
		},
		{
			c: Con{
				Reply: newIPTuple("2.2.2.4", laddrIP, 8080, uint16(laddrPort), ipProto),
			},
			ns: testNs,
		},
		{
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
			},
			ns: testNs,
		},
	}

	ctrkRoot, err := NewConntrack(rootNs)
	require.NoError(b, err)
	b.Cleanup(func() { ctrkRoot.Close() })

	ctrkTest, err := NewConntrack(testNs)
	require.NoError(b, err)
	b.Cleanup(func() { ctrkTest.Close() })

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, te := range tests {
			switch te.ns {
			case rootNs:
				_, _ = ctrkRoot.Exists(&te.c)
			case testNs:
				_, _ = ctrkTest.Exists(&te.c)
			}
		}
	}
}

func TestConntrackExists6(t *testing.T) {
	flake.Mark(t)
	ns := testutil.SetupCrossNsDNAT6(t)

	tcpCloser := nettestutil.StartServerTCPNs(t, net.ParseIP("fd00::2"), 8080, ns)
	defer tcpCloser.Close()

	udpCloser := nettestutil.StartServerUDPNs(t, net.ParseIP("fd00::2"), 8080, ns)
	defer udpCloser.Close()

	tcpConn := nettestutil.MustPingTCP(t, net.ParseIP("fd00::2"), 80)
	defer tcpConn.Close()

	udpConn := nettestutil.MustPingUDP(t, net.ParseIP("fd00::2"), 80)
	defer udpConn.Close()

	testNs, err := netns.GetFromName(ns)
	require.NoError(t, err)
	defer testNs.Close()

	ctrks := map[netns.NsHandle]Conntrack{}
	defer func() {
		for _, ctrk := range ctrks {
			ctrk.Close()
		}
	}()

	tcpLaddr := tcpConn.LocalAddr().(*net.TCPAddr)
	udpLaddr := udpConn.LocalAddr().(*net.UDPAddr)
	// test a combination of (tcp, udp) x (root ns, test ns)
	testConntrackExists6(t, tcpLaddr.IP.String(), tcpLaddr.Port, "tcp", testNs, ctrks)
	testConntrackExists6(t, udpLaddr.IP.String(), udpLaddr.Port, "udp", testNs, ctrks)
}

func TestConntrackExistsRootDNAT(t *testing.T) {
	destIP := "10.10.1.1"
	destPort := 80
	listenIP := "2.2.2.4"
	listenPort := 8080
	ns := testutil.SetupCrossNsDNATWithPorts(t, destPort, listenPort)

	nettestutil.IptablesSave(t)
	nettestutil.RunCommands(t, []string{
		"iptables --table nat --new-chain CLUSTERIPS",
		"iptables --table nat --append PREROUTING --jump CLUSTERIPS",
		"iptables --table nat --append OUTPUT --jump CLUSTERIPS",
		fmt.Sprintf("iptables --table nat --append CLUSTERIPS --destination %s --protocol tcp --match tcp --dport %d --jump DNAT --to-destination %s:%d", destIP, destPort, listenIP, destPort),
		fmt.Sprintf("ip route add %s dev veth1", destIP),
	}, false)

	testNs, err := netns.GetFromName(ns)
	require.NoError(t, err)
	defer testNs.Close()

	rootNs, err := kernel.GetRootNetNamespace("/proc")
	require.NoError(t, err)
	defer rootNs.Close()

	tcpCloser := nettestutil.StartServerTCPNs(t, net.ParseIP(listenIP), listenPort, ns)
	defer tcpCloser.Close()

	tcpConn := nettestutil.MustPingTCP(t, net.ParseIP(destIP), destPort)
	defer tcpConn.Close()

	rootck, err := NewConntrack(rootNs)
	require.NoError(t, err)

	testck, err := NewConntrack(testNs)
	require.NoError(t, err)

	tcpLaddr := tcpConn.LocalAddr().(*net.TCPAddr)
	c := &Con{
		Origin: newIPTuple(tcpLaddr.IP.String(), destIP, uint16(tcpLaddr.Port), uint16(destPort), unix.IPPROTO_TCP),
	}

	exists, err := rootck.Exists(c)
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = testck.Exists(c)
	require.NoError(t, err)
	assert.False(t, exists)
}

func testConntrackExists(t *testing.T, laddrIP string, laddrPort int, proto string, testNs netns.NsHandle, ctrks map[netns.NsHandle]Conntrack) {
	rootNs, err := kernel.GetRootNetNamespace("/proc")
	require.NoError(t, err)
	defer rootNs.Close()

	var ipProto uint8 = unix.IPPROTO_UDP
	if proto == "tcp" {
		ipProto = unix.IPPROTO_TCP
	}
	tests := []struct {
		desc   string
		c      Con
		exists bool
		ns     netns.NsHandle
	}{
		{
			desc: fmt.Sprintf("net ns 0, origin exists, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     rootNs,
		},
		{
			desc: fmt.Sprintf("net ns 0, reply exists, proto %s", proto),
			c: Con{
				Reply: newIPTuple("2.2.2.4", laddrIP, 80, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     rootNs,
		},
		{
			desc: fmt.Sprintf("net ns 0, origin does not exist, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     rootNs,
		},
		{
			desc: fmt.Sprintf("net ns %d, origin exists, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     testNs,
		},
		{
			desc: fmt.Sprintf("net ns %d, reply exists, proto %s", int(testNs), proto),
			c: Con{
				Reply: newIPTuple("2.2.2.4", laddrIP, 8080, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     testNs,
		},
		{
			desc: fmt.Sprintf("net ns %d, origin does not exist, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     testNs,
		},
	}

	for _, te := range tests {
		t.Run(te.desc, func(t *testing.T) {
			ctrk, ok := ctrks[te.ns]
			if !ok {
				var err error
				ctrk, err = NewConntrack(te.ns)
				require.NoError(t, err)

				ctrks[te.ns] = ctrk
			}

			ok, err := ctrk.Exists(&te.c)
			require.NoError(t, err)
			require.Equal(t, te.exists, ok)
		})
	}
}

func testConntrackExists6(t *testing.T, laddrIP string, laddrPort int, proto string, testNs netns.NsHandle, ctrks map[netns.NsHandle]Conntrack) {
	rootNs, err := kernel.GetRootNetNamespace("/proc")
	require.NoError(t, err)
	defer rootNs.Close()

	var ipProto uint8 = unix.IPPROTO_UDP
	if proto == "tcp" {
		ipProto = unix.IPPROTO_TCP
	}
	tests := []struct {
		desc   string
		c      Con
		exists bool
		ns     netns.NsHandle
	}{
		{
			desc: fmt.Sprintf("net ns 0, origin exists, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::2", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     rootNs,
		},
		{
			desc: fmt.Sprintf("net ns 0, reply exists, proto %s", proto),
			c: Con{
				Reply: newIPTuple("fd00::2", laddrIP, 80, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     rootNs,
		},
		{
			desc: fmt.Sprintf("net ns 0, origin does not exist, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::1", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     rootNs,
		},
		{
			desc: fmt.Sprintf("net ns %d, origin exists, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::2", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     testNs,
		},
		{
			desc: fmt.Sprintf("net ns %d, reply exists, proto %s", int(testNs), proto),
			c: Con{
				Reply: newIPTuple("fd00::2", laddrIP, 8080, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     testNs,
		},
		{
			desc: fmt.Sprintf("net ns %d, origin does not exist, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::1", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     testNs,
		},
	}

	for _, te := range tests {
		t.Run(te.desc, func(t *testing.T) {
			ctrk, ok := ctrks[te.ns]
			if !ok {
				var err error
				ctrk, err = NewConntrack(te.ns)
				require.NoError(t, err)

				ctrks[te.ns] = ctrk
			}

			ok, err := ctrk.Exists(&te.c)
			require.NoError(t, err)
			require.Equal(t, te.exists, ok)
		})
	}
}
