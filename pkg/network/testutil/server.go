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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// StartServerTCPNs is identical to StartServerTCP, but it operates with the
// network namespace provided by name.
func StartServerTCPNs(t testing.TB, ip net.IP, port int, ns string) io.Closer {
	h, err := netns.GetFromName(ns)
	require.NoError(t, err)

	var closer io.Closer
	err = kernel.WithNS(h, func() error {
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

	l, err := net.Listen(network, addr)
	require.NoError(t, err)
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

			_, _ = conn.Write([]byte("hello"))
			conn.Close()
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

// StartServerUDPNs is identical to StartServerUDP, but it operates with the
// network namespace provided by name.
func StartServerUDPNs(t *testing.T, ip net.IP, port int, ns string) io.Closer {
	h, err := netns.GetFromName(ns)
	require.NoError(t, err)

	var closer io.Closer
	err = kernel.WithNS(h, func() error {
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
	assert.Nil(t, err)

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
