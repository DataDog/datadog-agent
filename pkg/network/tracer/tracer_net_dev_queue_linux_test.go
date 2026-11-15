// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"bytes"
	"io"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	netlinktestutil "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	tracertestutil "github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestNetDevQueueSKBConnTuple checks the exact contents of conn_tuple_to_socket_skb_conn_tuple, which the
// net_dev_queue probe fills when an egress skb's tuple differs from its socket's tuple (DNAT here).
func (s *TracerSuite) TestNetDevQueueSKBConnTuple() {
	t := s.T()
	cfg := testConfig()
	skipEbpflessTodo(t, cfg)
	if ebpftest.GetBuildMode() == ebpftest.SK {
		t.Skip("net_dev_queue probe is not used by the SK tracer")
	}
	if !kprobe.ClassificationSupported(cfg) {
		t.Skip("protocol classification not supported")
	}

	netlinktestutil.SetupDNAT(t)
	v6 := cfg.CollectTCPv6Conns && kernel.IsIPv6Enabled()
	if v6 {
		netlinktestutil.SetupDNAT6(t)
	}

	tr := setupTracer(t, cfg)
	skbMap, err := tr.GetMap(probes.ConnectionTupleToSocketSKBConnMap)
	require.NoError(t, err)

	t.Run("IPv4 TCP", func(t *testing.T) {
		port := startEchoServer(t, "1.1.1.1:0", "tcp4")
		conn, err := tracertestutil.DialTCP("tcp4", net.JoinHostPort("2.2.2.2", port))
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })

		assertSKBConnTuples(t, skbMap, conn, net.ParseIP("2.2.2.2"), net.ParseIP("1.1.1.1"))
	})

	t.Run("IPv4-mapped IPv6 TCP", func(t *testing.T) {
		if !v6 {
			t.Skip("IPv6 not enabled")
		}
		port := startEchoServer(t, "1.1.1.1:0", "tcp4")
		conn := dialDualStackTCP(t, net.ParseIP("2.2.2.2"), port)
		t.Cleanup(func() { conn.Close() })

		// The AF_INET6 socket tuple maps back to IPv4, matching the IPv4 skb tuple.
		assertSKBConnTuples(t, skbMap, conn, net.ParseIP("2.2.2.2"), net.ParseIP("1.1.1.1"))
	})

	t.Run("IPv6 TCP", func(t *testing.T) {
		if !v6 {
			t.Skip("IPv6 not enabled")
		}
		port := startEchoServer(t, "[fd00::1]:0", "tcp6")
		conn, err := tracertestutil.DialTCP("tcp6", net.JoinHostPort("fd00::2", port))
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })

		assertSKBConnTuples(t, skbMap, conn, net.ParseIP("fd00::2"), net.ParseIP("fd00::1"))
	})

	t.Run("UDP", func(t *testing.T) {
		send := func(t *testing.T, network, listenAddr, natAddr string) {
			ln, err := net.ListenPacket(network, listenAddr)
			require.NoError(t, err)
			t.Cleanup(func() { ln.Close() })
			_, port, err := net.SplitHostPort(ln.LocalAddr().String())
			require.NoError(t, err)

			c, err := net.Dial(network, net.JoinHostPort(natAddr, port))
			require.NoError(t, err)
			t.Cleanup(func() { c.Close() })
			const datagrams = 10
			for i := 0; i < datagrams; i++ {
				_, err = c.Write(bytes.Repeat([]byte{'u'}, 200))
				require.NoError(t, err)
			}
			// net_dev_queue fires on egress before loopback delivers the datagram, so once every datagram has
			// arrived, the probe has run for all of them
			require.NoError(t, ln.SetReadDeadline(time.Now().Add(5*time.Second)))
			buf := make([]byte, 512)
			for i := 0; i < datagrams; i++ {
				_, _, err = ln.ReadFrom(buf)
				require.NoError(t, err, "datagram %d of %d not received", i+1, datagrams)
			}
		}
		send(t, "udp4", "1.1.1.1:0", "2.2.2.2")
		if v6 {
			send(t, "udp6", "[fd00::1]:0", "fd00::2")
		}

		// every UDP datagram has been through net_dev_queue, so only the TCP subtests' entries can be there
		var k, v netebpf.ConnTuple
		it := skbMap.Iterate()
		for it.Next(unsafe.Pointer(&k), unsafe.Pointer(&v)) {
			assert.NotZero(t, k.Metadata&uint32(netebpf.TCP), "unexpected non-TCP key %+v", k)
			assert.NotZero(t, v.Metadata&uint32(netebpf.TCP), "unexpected non-TCP value %+v", v)
		}
		require.NoError(t, it.Err())
	})
}

// startEchoServer starts a TCP echo server and returns its port.
func startEchoServer(t *testing.T, addr, network string) string {
	server := tracertestutil.NewTCPServerOnAddress(addr, func(c net.Conn) {
		defer c.Close()
		_, _ = io.Copy(c, c)
	})
	server.Network = network
	require.NoError(t, server.Run())
	t.Cleanup(server.Shutdown)
	_, port, err := net.SplitHostPort(server.Address())
	require.NoError(t, err)
	return port
}

// dialDualStackTCP connects an AF_INET6 socket with IPV6_V6ONLY off to the IPv4-mapped form of ip.
func dialDualStackTCP(t *testing.T, ip net.IP, port string) net.Conn {
	p, err := strconv.Atoi(port)
	require.NoError(t, err)
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	require.NoError(t, err)
	f := os.NewFile(uintptr(fd), "dual-stack-tcp")
	defer f.Close()
	require.NoError(t, unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, 0))
	sa := &unix.SockaddrInet6{Port: p}
	copy(sa.Addr[:], ip.To16())
	require.NoError(t, unix.Connect(fd, sa))

	conn, err := net.FileConn(f)
	require.NoError(t, err)
	require.NoError(t, tracertestutil.SetTestDeadline(conn))
	return conn
}

// assertSKBConnTuples sends data over conn and checks that both directions have an entry mapping the socket
// tuple (destination natIP) to the skb tuple (destination realIP), byte for byte.
func assertSKBConnTuples(t *testing.T, skbMap *ebpf.Map, conn net.Conn, natIP, realIP net.IP) {
	local := conn.LocalAddr().(*net.TCPAddr)
	remote := conn.RemoteAddr().(*net.TCPAddr)
	require.True(t, remote.IP.Equal(natIP), "expected remote %v, got %v", natIP, remote.IP)
	lport, rport := uint16(local.Port), uint16(remote.Port)

	// Client egress: the socket sees natIP, the skb carries realIP after DNAT in OUTPUT.
	clientSock := tcpConnTuple(local.IP, lport, natIP, rport)
	clientSKB := tcpConnTuple(local.IP, lport, realIP, rport)
	// Server egress: the socket is bound to realIP, the skb is translated back to natIP in POSTROUTING.
	serverSock := tcpConnTuple(realIP, rport, local.IP, lport)
	serverSKB := tcpConnTuple(natIP, rport, local.IP, lport)

	// Payloads over 40 bytes so the IPv6 data-length check in sk_buff_to_tuple passes.
	payload := bytes.Repeat([]byte{'p'}, 200)
	buf := make([]byte, len(payload))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := conn.Write(payload)
		require.NoError(c, err)
		_, err = io.ReadFull(conn, buf)
		require.NoError(c, err)

		got, ok := lookupSKBConnTuple(t, skbMap, clientSock)
		if assert.True(c, ok, "no entry for client socket tuple %+v", clientSock) {
			assert.Equal(c, clientSKB, got, "client skb tuple")
		}
		got, ok = lookupSKBConnTuple(t, skbMap, serverSock)
		if assert.True(c, ok, "no entry for server socket tuple %+v", serverSock) {
			assert.Equal(c, serverSKB, got, "server skb tuple")
		}
	}, 5*time.Second, 100*time.Millisecond)
}

func tcpConnTuple(src net.IP, sport uint16, dst net.IP, dport uint16) netebpf.ConnTuple {
	srcAddr, dstAddr := util.AddressFromNetIP(src), util.AddressFromNetIP(dst)
	family := netebpf.IPv4
	if srcAddr.Is6() {
		family = netebpf.IPv6
	}
	sl, sh := util.ToLowHigh(srcAddr)
	dl, dh := util.ToLowHigh(dstAddr)
	return netebpf.ConnTuple{
		Saddr_h:  sh,
		Saddr_l:  sl,
		Daddr_h:  dh,
		Daddr_l:  dl,
		Sport:    sport,
		Dport:    dport,
		Metadata: uint32(netebpf.TCP) | uint32(family),
	}
}

func flipConnTuple(c netebpf.ConnTuple) netebpf.ConnTuple {
	c.Saddr_h, c.Daddr_h = c.Daddr_h, c.Saddr_h
	c.Saddr_l, c.Daddr_l = c.Daddr_l, c.Saddr_l
	c.Sport, c.Dport = c.Dport, c.Sport
	return c
}

// lookupSKBConnTuple finds the skb tuple for sockTup. Both key and value are normalized by port, and DNAT keeps
// ports, so a flipped key always comes with a flipped value.
func lookupSKBConnTuple(t *testing.T, skbMap *ebpf.Map, sockTup netebpf.ConnTuple) (netebpf.ConnTuple, bool) {
	var v netebpf.ConnTuple
	if err := skbMap.Lookup(unsafe.Pointer(&sockTup), unsafe.Pointer(&v)); err == nil {
		return v, true
	}
	flipped := flipConnTuple(sockTup)
	if err := skbMap.Lookup(unsafe.Pointer(&flipped), unsafe.Pointer(&v)); err == nil {
		return flipConnTuple(v), true
	}
	t.Logf("no entry for %+v", sockTup)
	return netebpf.ConnTuple{}, false
}
