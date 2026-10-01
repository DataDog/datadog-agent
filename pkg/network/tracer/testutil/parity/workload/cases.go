// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package workload

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	vnetns "github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/network"
	netlinktestutil "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/parity"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
)

const ioTimeout = 10 * time.Second

// loopbacks are the loopback addresses each family-parameterized case runs on
var loopbacks = []struct {
	name string
	ip   string
}{
	{"v4", "127.0.0.1"},
	{"v6", "::1"},
}

// Default returns the first-cut set of cases: TCP short-lived, long-lived and
// large transfers, sendfile, refused and reset connects, connected and
// unconnected UDP, MSG_PEEK, all over loopback in v4 and v6, plus TCP in a
// second network namespace.
func Default() []Case {
	var cases []Case
	for _, lo := range loopbacks {
		ip := lo.ip
		cases = append(cases,
			Case{"tcp_short_" + lo.name, func(tb testing.TB, rec *Recorder) { tcpExchange(tb, rec, ip, 4096, 8192) }},
			Case{"tcp_long_lived_" + lo.name, func(tb testing.TB, rec *Recorder) { tcpLongLived(tb, rec, ip, 10, 1500) }},
			Case{"tcp_large_" + lo.name, func(tb testing.TB, rec *Recorder) { tcpExchange(tb, rec, ip, 8<<20, 1<<20) }},
			Case{"tcp_sendfile_" + lo.name, func(tb testing.TB, rec *Recorder) { tcpSendfile(tb, rec, ip, 256<<10) }},
			Case{"tcp_refused_" + lo.name, func(tb testing.TB, rec *Recorder) { tcpRefused(tb, rec, ip) }},
			Case{"tcp_reset_" + lo.name, func(tb testing.TB, rec *Recorder) { tcpReset(tb, rec, ip) }},
			Case{"udp_connected_" + lo.name, func(tb testing.TB, rec *Recorder) { udpExchange(tb, rec, ip, true, 5, 512) }},
			Case{"udp_unconnected_" + lo.name, func(tb testing.TB, rec *Recorder) { udpExchange(tb, rec, ip, false, 5, 512) }},
			Case{"udp_peek_" + lo.name, func(tb testing.TB, rec *Recorder) { udpPeek(tb, rec, ip, 300) }},
		)
	}
	cases = append(cases, Case{"tcp_netns_v4", tcpInNetNS})
	return cases
}

func requireLoopback(tb testing.TB, ip string) {
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		tb.Skipf("loopback %s unavailable: %v", ip, err)
	}
	ln.Close()
}

func payload(n int) []byte {
	return bytes.Repeat([]byte("parity-workload-"), n/16+1)[:n]
}

func tcpBytes(sent, recv int) parity.Expected {
	return parity.Expected{"SentBytes": uint64(sent), "RecvBytes": uint64(recv)}
}

// tcpServer starts a listener whose handler is run for each accepted connection
func tcpServer(tb testing.TB, ip string, handler func(net.Conn)) net.Listener {
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	require.NoError(tb, err)
	tb.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(ioTimeout))
				handler(c)
			}()
		}
	}()
	return ln
}

// tcpExchange sends req bytes, half-closes, then reads resp bytes until the
// server closes, and closes the connection
func tcpExchange(tb testing.TB, rec *Recorder, ip string, req, resp int) {
	requireLoopback(tb, ip)
	ln := tcpServer(tb, ip, func(c net.Conn) {
		if _, err := io.Copy(io.Discard, c); err != nil {
			return
		}
		_, _ = c.Write(payload(resp))
	})

	c, err := net.DialTimeout("tcp", ln.Addr().String(), ioTimeout)
	require.NoError(tb, err)
	require.NoError(tb, c.SetDeadline(time.Now().Add(ioTimeout)))

	_, err = c.Write(payload(req))
	require.NoError(tb, err)
	require.NoError(tb, c.(*net.TCPConn).CloseWrite())
	n, err := io.Copy(io.Discard, c)
	require.NoError(tb, err)
	require.EqualValues(tb, resp, n)

	rec.ExpectPair(network.TCP, addrPort(c.LocalAddr()), addrPort(c.RemoteAddr()), tcpBytes(req, resp), tcpBytes(resp, req))
	c.Close()
}

// tcpLongLived does several request/response rounds on one connection and
// leaves it open, so the connection is compared as active
func tcpLongLived(tb testing.TB, rec *Recorder, ip string, rounds, size int) {
	requireLoopback(tb, ip)
	ln := tcpServer(tb, ip, func(c net.Conn) {
		_ = c.SetDeadline(time.Time{})
		buf := make([]byte, size)
		for {
			if _, err := io.ReadFull(c, buf); err != nil {
				return
			}
			if _, err := c.Write(buf); err != nil {
				return
			}
		}
	})

	c, err := net.DialTimeout("tcp", ln.Addr().String(), ioTimeout)
	require.NoError(tb, err)
	tb.Cleanup(func() { c.Close() })
	require.NoError(tb, c.SetDeadline(time.Now().Add(ioTimeout)))

	msg, buf := payload(size), make([]byte, size)
	for range rounds {
		_, err = c.Write(msg)
		require.NoError(tb, err)
		_, err = io.ReadFull(c, buf)
		require.NoError(tb, err)
	}
	total := rounds * size
	rec.ExpectPair(network.TCP, addrPort(c.LocalAddr()), addrPort(c.RemoteAddr()), tcpBytes(total, total), tcpBytes(total, total))
}

// tcpSendfile sends size bytes with sendfile(2), exercising tcp_sendpage /
// splice paths rather than tcp_sendmsg
func tcpSendfile(tb testing.TB, rec *Recorder, ip string, size int) {
	requireLoopback(tb, ip)
	received := make(chan int64, 1)
	ln := tcpServer(tb, ip, func(c net.Conn) {
		n, _ := io.Copy(io.Discard, c)
		received <- n
	})

	path := filepath.Join(tb.TempDir(), "sendfile_source")
	require.NoError(tb, os.WriteFile(path, payload(size), 0o600))
	f, err := os.Open(path)
	require.NoError(tb, err)
	defer f.Close()

	c, err := net.DialTimeout("tcp", ln.Addr().String(), ioTimeout)
	require.NoError(tb, err)
	raw, err := c.(*net.TCPConn).SyscallConn()
	require.NoError(tb, err)
	sent := 0
	var serr error
	require.NoError(tb, raw.Write(func(fd uintptr) bool {
		var n int
		n, serr = syscall.Sendfile(int(fd), int(f.Fd()), nil, size-sent)
		if n > 0 {
			sent += n
		}
		if errors.Is(serr, syscall.EAGAIN) {
			return false
		}
		return serr != nil || sent >= size
	}))
	require.NoError(tb, serr)
	require.Equal(tb, size, sent)
	client, server := addrPort(c.LocalAddr()), addrPort(c.RemoteAddr())
	c.Close()

	select {
	case n := <-received:
		require.EqualValues(tb, size, n)
	case <-time.After(ioTimeout):
		require.Fail(tb, "sendfile server did not receive data")
	}
	rec.ExpectPair(network.TCP, client, server, tcpBytes(size, 0), tcpBytes(0, size))
}

// tcpRefused connects to a port with no listener (ECONNREFUSED)
func tcpRefused(tb testing.TB, rec *Recorder, ip string) {
	requireLoopback(tb, ip)
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	require.NoError(tb, err)
	addr := ln.Addr().String()
	port := addrPort(ln.Addr()).Port()
	ln.Close()

	rec.UsePorts(port)
	_, err = net.DialTimeout("tcp", addr, ioTimeout)
	require.ErrorIs(tb, err, syscall.ECONNREFUSED)
}

// tcpReset has the server abort the connection with an RST (SO_LINGER 0)
// after reading the client's request
func tcpReset(tb testing.TB, rec *Recorder, ip string) {
	requireLoopback(tb, ip)
	const req = 1024
	ln := tcpServer(tb, ip, func(c net.Conn) {
		_, _ = io.ReadFull(c, make([]byte, req))
		_ = c.(*net.TCPConn).SetLinger(0)
	})

	c, err := net.DialTimeout("tcp", ln.Addr().String(), ioTimeout)
	require.NoError(tb, err)
	defer c.Close()
	require.NoError(tb, c.SetDeadline(time.Now().Add(ioTimeout)))
	rec.UsePorts(addrPort(c.LocalAddr()).Port(), addrPort(c.RemoteAddr()).Port())

	_, err = c.Write(payload(req))
	require.NoError(tb, err)
	_, err = c.Read(make([]byte, 1))
	require.ErrorIs(tb, err, syscall.ECONNRESET)
}

func udpCounts(sentBytes, sentPkts, recvBytes, recvPkts int) parity.Expected {
	return parity.Expected{
		"SentBytes":   uint64(sentBytes),
		"SentPackets": uint64(sentPkts),
		"RecvBytes":   uint64(recvBytes),
		"RecvPackets": uint64(recvPkts),
	}
}

// udpExchange sends n datagrams of size bytes, each echoed by the server. The
// client socket is connect(2)ed when connected is true, and uses sendto(2)
// on an unconnected socket otherwise.
func udpExchange(tb testing.TB, rec *Recorder, ip string, connected bool, n, size int) {
	requireLoopback(tb, ip)
	srv, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(netipAddrPort(ip, 0)))
	require.NoError(tb, err)
	defer srv.Close()
	go func() {
		buf := make([]byte, 65535)
		for range n {
			_ = srv.SetDeadline(time.Now().Add(ioTimeout))
			m, from, err := srv.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			_, _ = srv.WriteToUDPAddrPort(buf[:m], from)
		}
	}()
	srvAddr := addrPort(srv.LocalAddr())

	var c *net.UDPConn
	if connected {
		c, err = net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(srvAddr))
	} else {
		c, err = net.ListenUDP("udp", net.UDPAddrFromAddrPort(netipAddrPort(ip, 0)))
	}
	require.NoError(tb, err)
	defer c.Close()
	require.NoError(tb, c.SetDeadline(time.Now().Add(ioTimeout)))

	msg, buf := payload(size), make([]byte, 65535)
	for range n {
		if connected {
			_, err = c.Write(msg)
		} else {
			_, err = c.WriteToUDPAddrPort(msg, srvAddr)
		}
		require.NoError(tb, err)
		m, _, err := c.ReadFromUDPAddrPort(buf)
		require.NoError(tb, err)
		require.Equal(tb, size, m)
	}
	total := n * size
	rec.ExpectPair(network.UDP, addrPort(c.LocalAddr()), srvAddr,
		udpCounts(total, n, total, n), udpCounts(total, n, total, n))
}

// udpPeek has the server MSG_PEEK a datagram before reading it, which must not
// be counted twice
func udpPeek(tb testing.TB, rec *Recorder, ip string, size int) {
	requireLoopback(tb, ip)
	srv, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(netipAddrPort(ip, 0)))
	require.NoError(tb, err)
	defer srv.Close()

	c, err := net.DialUDP("udp", nil, srv.LocalAddr().(*net.UDPAddr))
	require.NoError(tb, err)
	defer c.Close()
	_, err = c.Write(payload(size))
	require.NoError(tb, err)

	raw, err := srv.SyscallConn()
	require.NoError(tb, err)
	buf := make([]byte, 65535)
	for _, flags := range []int{syscall.MSG_PEEK, 0} {
		var n int
		var rerr error
		require.NoError(tb, raw.Read(func(fd uintptr) bool {
			n, _, rerr = syscall.Recvfrom(int(fd), buf, flags)
			return !errors.Is(rerr, syscall.EAGAIN)
		}))
		require.NoError(tb, rerr)
		require.Equal(tb, size, n)
	}
	rec.ExpectPair(network.UDP, addrPort(c.LocalAddr()), addrPort(srv.LocalAddr()),
		udpCounts(size, 1, 0, 0), udpCounts(0, 0, size, 1))
}

// tcpInNetNS runs a short TCP exchange entirely inside a new network namespace
func tcpInNetNS(tb testing.TB, rec *Recorder) {
	ns := netlinktestutil.AddNS(tb)
	testutil.RunCommands(tb, []string{fmt.Sprintf("ip -n %s link set lo up", ns)}, false)
	h, err := vnetns.GetFromName(ns)
	require.NoError(tb, err)
	defer h.Close()

	err = netns.WithNS(h, func() error {
		tcpExchange(tb, rec, "127.0.0.1", 2048, 4096)
		return nil
	})
	require.NoError(tb, err)
}
