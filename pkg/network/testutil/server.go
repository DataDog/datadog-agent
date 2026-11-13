// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package testutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
)

// StartServerTCPNs is identical to StartServerTCP, but it operates with the
// network namespace provided by name.
func StartServerTCPNs(t testing.TB, ip net.IP, port int, ns string) io.Closer {
	h, err := netns.GetFromName(ns)
	require.NoError(t, err)

	var closer io.Closer
	err = netnsutil.WithNS(h, func() error {
		closer = StartServerTCP(t, ip, port)
		return nil
	})
	require.NoError(t, err)

	return closer
}

// StartServerTCP starts a TCP server listening at provided IP address and port.
// It will respond to any connection with "hello" and then close the connection.
// It returns an io.Closer that should be Close'd when you are finished with it.
func StartServerTCP(t testing.TB, ip net.IP, port int) io.Closer {
	ch := make(chan struct{})
	addr := fmt.Sprintf("%s:%d", ip, port)
	network := "tcp"
	if isIpv6(ip) {
		network = "tcp6"
		addr = fmt.Sprintf("[%s]:%d", ip, port)
	}

	ln, err := net.Listen(network, addr)
	require.NoError(t, err)
	l := &tcpTestServer{Listener: ln, conns: make(map[net.Conn]struct{})}
	go func() {
		close(ch)
		for {
			conn, err := l.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				t.Logf("accept error: %s", err)
				continue
			}

			if !l.track(conn) {
				conn.Close()
				return
			}
			go func() {
				defer l.untrack(conn)
				respondAndCloseTCP(conn)
			}()
		}
	}()
	<-ch

	require.EventuallyWithT(t, func(tb *assert.CollectT) {
		conn := PingTCP(tb, ip, l.Addr().(*net.TCPAddr).Port)
		if conn != nil {
			conn.Close()
		}
	}, 3*time.Second, 100*time.Millisecond, "timed out waiting for TCP server to come up")

	return l
}

// tcpTestServer is the net.Listener returned by StartServerTCP. Closing it also
// closes the connections that are still being drained, and waits for their
// goroutines, so nothing outlives the server.
type tcpTestServer struct {
	net.Listener

	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup
}

func (s *tcpTestServer) track(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns[conn] = struct{}{}
	s.wg.Add(1)
	return true
}

func (s *tcpTestServer) untrack(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
	s.wg.Done()
}

// Close stops accepting connections, closes the ones still open and waits for
// their goroutines to exit.
func (s *tcpTestServer) Close() error {
	err := s.Listener.Close()
	s.mu.Lock()
	s.closed = true
	for conn := range s.conns {
		conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

// tcpDrainTimeout bounds how long respondAndCloseTCP waits for the client to
// close its side of the connection.
const tcpDrainTimeout = 5 * time.Second

// respondAndCloseTCP writes "hello" to conn and closes it gracefully.
//
// Closing a socket that still has unread data in its receive buffer (e.g. the
// "ping" sent by PingTCP) makes the kernel abort the connection with a RST
// instead of a FIN, and a RST that overtakes the "hello" segment makes the
// client's read fail with ECONNRESET and lose the data. To avoid that, the
// server half-closes (FIN, which TCP orders correctly behind the data), then
// reads and discards whatever the client sends until it closes its side or the
// drain timeout expires, and only then closes the socket.
func respondAndCloseTCP(conn net.Conn) {
	defer conn.Close()
	_, _ = conn.Write([]byte("hello"))
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(tcpDrainTimeout))
	_, _ = io.Copy(io.Discard, conn)
}

// StartServerUDPNs is identical to StartServerUDP, but it operates with the
// network namespace provided by name.
func StartServerUDPNs(t *testing.T, ip net.IP, port int, ns string) io.Closer {
	h, err := netns.GetFromName(ns)
	require.NoError(t, err)

	var closer io.Closer
	err = netnsutil.WithNS(h, func() error {
		closer = StartServerUDP(t, ip, port)
		return nil
	})
	require.NoError(t, err)

	return closer
}

// StartServerUDP starts a UDP server listening at provided IP address and port.
// It does not respond in any fashion to sent datagrams.
// It returns an io.Closer that should be Close'd when you are finished with it.
func StartServerUDP(t *testing.T, ip net.IP, port int) io.Closer {
	ch := make(chan struct{})
	network := "udp"
	if isIpv6(ip) {
		network = "udp6"
	}

	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}

	udpConn, err := net.ListenUDP(network, addr)
	require.NoError(t, err, "could not listen udp on address %s", addr)

	addrStr := udpConn.LocalAddr().String()
	_, portStr, err := net.SplitHostPort(addrStr)
	assert.Nil(t, err)
	port, err = strconv.Atoi(portStr)
	assert.Nil(t, err)

	go func() {
		close(ch)

		for {
			bs := make([]byte, 10)
			_, addr, err := udpConn.ReadFrom(bs)
			if err != nil {
				return
			}

			_, err = udpConn.WriteTo([]byte("pong"), addr)
			if err != nil {
				return
			}
		}
	}()
	<-ch

	require.EventuallyWithT(t, func(t *assert.CollectT) {
		conn := PingUDP(t, ip, port)
		if conn != nil {
			conn.Close()
		}
	}, 3*time.Second, 10*time.Millisecond, "timed out waiting for UDP server to come up")

	return udpConn
}
