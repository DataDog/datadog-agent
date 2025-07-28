// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && linux && linux_bpf

package packets

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
)

type tcpTestCase struct {
	filterConfig  func(server, client netip.AddrPort) TCP4FilterConfig
	shouldCapture bool
}

func doTestCase(t *testing.T, tc tcpTestCase) {
	// we use bound ports on the server and the client so this should be safe to parallelize
	t.Parallel()

	server := testutil.NewTCPServerOnAddress("127.0.0.42:0", func(c net.Conn) {
		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
		c.Write([]byte("foo\n"))
		testutil.GracefulCloseTCP(c)
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	dialer := net.Dialer{
		Timeout: time.Minute,
		LocalAddr: &net.TCPAddr{
			// make it different from the server IP
			IP: net.ParseIP("127.0.0.43"),
		},
	}

	conn, err := dialer.Dial("tcp", server.Address())
	require.NoError(t, err)
	defer testutil.GracefulCloseTCP(conn)

	serverAddrPort, err := netip.ParseAddrPort(server.Address())
	require.NoError(t, err)
	clientAddrPort, err := netip.ParseAddrPort(conn.LocalAddr().String())
	require.NoError(t, err)

	cfg := tc.filterConfig(serverAddrPort, clientAddrPort)
	filter, err := cfg.GenerateTCP4Filter()
	require.NoError(t, err)

	lc := &net.ListenConfig{
		Control: func(_network, _address string, c syscall.RawConn) error {
			err := SetBPFAndDrain(c, filter)
			require.NoError(t, err)
			return err
		},
	}

	rawConn, err := MakeRawConn(context.Background(), lc, "ip:tcp", clientAddrPort.Addr())
	require.NoError(t, err)

	conn.Write([]byte("bar\n"))

	buffer := make([]byte, 1024)

	rawConn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	n, addr, err := rawConn.ReadFromIP(buffer)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		// ErrDeadlineExceeded is what the test checks for, so we should only blow up on real errors
		require.NoError(t, err)
		require.NotZero(t, n)
	}

	hasCaptured := !errors.Is(err, os.ErrDeadlineExceeded)
	if tc.shouldCapture {
		require.True(t, hasCaptured, "expected to see a packet, but found nothing")
		require.Equal(t, addr.IP, net.IP(cfg.Src.Addr().AsSlice()))
	} else {
		require.False(t, hasCaptured, "expected not to see a packet, but found one from %s", addr)
	}

}
func TestTCPFilterMatch(t *testing.T) {
	doTestCase(t, tcpTestCase{
		filterConfig: func(server, client netip.AddrPort) TCP4FilterConfig {
			return TCP4FilterConfig{Src: server, Dst: client}
		},
		shouldCapture: true,
	})
}

func mangleIP(ap netip.AddrPort) netip.AddrPort {
	reservedIP := netip.MustParseAddr("233.252.0.0")
	return netip.AddrPortFrom(reservedIP, ap.Port())
}
func manglePort(ap netip.AddrPort) netip.AddrPort {
	const reservedPort = 47
	return netip.AddrPortFrom(ap.Addr(), reservedPort)
}

func TestTCPFilterBadServerIP(t *testing.T) {
	doTestCase(t, tcpTestCase{
		filterConfig: func(server, client netip.AddrPort) TCP4FilterConfig {
			return TCP4FilterConfig{Src: mangleIP(server), Dst: client}
		},
		shouldCapture: false,
	})
}

func TestTCPFilterBadServerPort(t *testing.T) {
	doTestCase(t, tcpTestCase{
		filterConfig: func(server, client netip.AddrPort) TCP4FilterConfig {
			return TCP4FilterConfig{Src: manglePort(server), Dst: client}
		},
		shouldCapture: false,
	})
}

func TestTCPFilterBadClientIP(t *testing.T) {
	doTestCase(t, tcpTestCase{
		filterConfig: func(server, client netip.AddrPort) TCP4FilterConfig {
			return TCP4FilterConfig{Src: server, Dst: mangleIP(client)}
		},
		shouldCapture: false,
	})
}

func TestTCPFilterBadClientPort(t *testing.T) {
	doTestCase(t, tcpTestCase{
		filterConfig: func(server, client netip.AddrPort) TCP4FilterConfig {
			return TCP4FilterConfig{Src: server, Dst: manglePort(client)}
		},
		shouldCapture: false,
	})
}
