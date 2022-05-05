// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && !android
// +build linux_bpf,!android

package netlink

import (
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

func TestConntrackExists(t *testing.T) {
	defer testutil.TeardownCrossNsDNAT(t)
	testutil.SetupCrossNsDNAT(t)

	tcpCloser := nettestutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 8080, "test")
	defer tcpCloser.Close()

	udpCloser := nettestutil.StartServerUDPNs(t, net.ParseIP("2.2.2.4"), 8080, "test")
	defer udpCloser.Close()

	tcpConn := nettestutil.PingTCP(t, net.ParseIP("2.2.2.4"), 80)
	defer tcpConn.Close()

	udpConn := nettestutil.PingUDP(t, net.ParseIP("2.2.2.4"), 80)
	defer udpConn.Close()

	testNs, err := netns.GetFromName("test")
	require.NoError(t, err)
	defer testNs.Close()

	ctrks := map[int]Conntrack{}
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

func TestConntrackExists6(t *testing.T) {
	defer testutil.TeardownCrossNsDNAT6(t)
	testutil.SetupCrossNsDNAT6(t)

	tcpCloser := nettestutil.StartServerTCPNs(t, net.ParseIP("fd00::2"), 8080, "test")
	defer tcpCloser.Close()

	udpCloser := nettestutil.StartServerUDPNs(t, net.ParseIP("fd00::2"), 8080, "test")
	defer udpCloser.Close()

	tcpConn := nettestutil.PingTCP(t, net.ParseIP("fd00::2"), 80)
	defer tcpConn.Close()

	udpConn := nettestutil.PingUDP(t, net.ParseIP("fd00::2"), 80)
	defer udpConn.Close()

	testNs, err := netns.GetFromName("test")
	require.NoError(t, err)
	defer testNs.Close()

	ctrks := map[int]Conntrack{}
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
	defer testutil.TeardownCrossNsDNAT(t)
	testutil.SetupCrossNsDNAT(t)
	defer nettestutil.RunCommands(t, []string{
		"iptables --table nat --delete CLUSTERIPS --destination 10.10.1.1 --protocol tcp --match tcp --dport 80 --jump DNAT --to-destination 2.2.2.4:80",
		"iptables --table nat --delete PREROUTING --jump CLUSTERIPS",
		"iptables --table nat --delete OUTPUT --jump CLUSTERIPS",
		"iptables --table nat --delete-chain CLUSTERIPS",
	}, true)
	nettestutil.RunCommands(t, []string{
		"iptables --table nat --new-chain CLUSTERIPS",
		"iptables --table nat --append PREROUTING --jump CLUSTERIPS",
		"iptables --table nat --append OUTPUT --jump CLUSTERIPS",
		"iptables --table nat --append CLUSTERIPS --destination 10.10.1.1 --protocol tcp --match tcp --dport 80 --jump DNAT --to-destination 2.2.2.4:80",
	}, false)

	testNs, err := netns.GetFromName("test")
	require.NoError(t, err)
	defer testNs.Close()

	rootNs, err := util.GetRootNetNamespace("/proc")
	require.NoError(t, err)
	defer rootNs.Close()

	destIP := "10.10.1.1"
	destPort := 80
	var tcpCloser io.Closer
	_ = util.WithNS("/proc", testNs, func() error {
		tcpCloser = nettestutil.StartServerTCP(t, net.ParseIP("2.2.2.4"), 8080)
		return nil
	})
	defer tcpCloser.Close()

	tcpConn := nettestutil.PingTCP(t, net.ParseIP(destIP), destPort)
	defer tcpConn.Close()

	rootck, err := NewConntrack(int(rootNs))
	require.NoError(t, err)

	testck, err := NewConntrack(int(testNs))
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

func testConntrackExists(t *testing.T, laddrIP string, laddrPort int, proto string, testNs netns.NsHandle, ctrks map[int]Conntrack) {
	rootNs, err := util.GetRootNetNamespace("/proc")
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
		ns     int
	}{
		{
			desc: fmt.Sprintf("net ns 0, origin exists, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns 0, reply exists, proto %s", proto),
			c: Con{
				Reply: newIPTuple("2.2.2.4", laddrIP, 80, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns 0, origin does not exist, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, origin exists, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     int(testNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, reply exists, proto %s", int(testNs), proto),
			c: Con{
				Reply: newIPTuple("2.2.2.4", laddrIP, 8080, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     int(testNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, origin does not exist, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     int(testNs),
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

func testConntrackExists6(t *testing.T, laddrIP string, laddrPort int, proto string, testNs netns.NsHandle, ctrks map[int]Conntrack) {
	rootNs, err := util.GetRootNetNamespace("/proc")
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
		ns     int
	}{
		{
			desc: fmt.Sprintf("net ns 0, origin exists, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::2", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns 0, reply exists, proto %s", proto),
			c: Con{
				Reply: newIPTuple("fd00::2", laddrIP, 80, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns 0, origin does not exist, proto %s", proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::1", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, origin exists, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::2", uint16(laddrPort), 80, ipProto),
			},
			exists: true,
			ns:     int(testNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, reply exists, proto %s", int(testNs), proto),
			c: Con{
				Reply: newIPTuple("fd00::2", laddrIP, 8080, uint16(laddrPort), ipProto),
			},
			exists: true,
			ns:     int(testNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, origin does not exist, proto %s", int(testNs), proto),
			c: Con{
				Origin: newIPTuple(laddrIP, "fd00::1", uint16(laddrPort), 80, ipProto),
			},
			exists: false,
			ns:     int(testNs),
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
