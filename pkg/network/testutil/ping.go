// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"fmt"
	"net"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MustPingTCP is a wrapper around PingTCP that guarantees a non-nil
// connection is returned
func MustPingTCP(tb require.TestingT, ip net.IP, port int) net.Conn {
	c := PingTCP(tb, ip, port)
	require.NotNil(tb, c, "pinged tcp connection must not be nil")
	return c
}

// MustPingUDP is a wrapper around PingUDP that guarantees a non-nil
// connection is returned
func MustPingUDP(tb require.TestingT, ip net.IP, port int) net.Conn {
	c := PingUDP(tb, ip, port)
	require.NotNil(tb, c, "pinged udp connection must not be nil")
	return c
}

// PingTCP connects to the provided IP address over TCP/TCPv6, sends the string "ping",
// reads from the connection, and returns the open connection for further use/inspection.
func PingTCP(tb require.TestingT, ip net.IP, port int) net.Conn {
	addr := fmt.Sprintf("%s:%d", ip, port)
	network := "tcp"
	if isIpv6(ip) {
		network = "tcp6"
		addr = fmt.Sprintf("[%s]:%d", ip, port)
	}

	conn, err := net.DialTimeout(network, addr, time.Second)
	if !assert.NoError(tb, err) {
		return nil
	}

	_, err = conn.Write([]byte("ping"))
	if !assert.NoError(tb, err) {
		return nil
	}

	bs := make([]byte, 10)
	_, err = conn.Read(bs)

	if !assert.NoError(tb, err) {
		return nil
	}

	return conn
}

// PingUDP connects to the provided IP address over UDP/UDPv6, sends the string "ping",
// and returns the open connection for further use/inspection.
func PingUDP(tb require.TestingT, ip net.IP, port int) net.Conn {
	network := "udp"
	if isIpv6(ip) {
		network = "udp6"
	}
	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}
	conn, err := net.DialUDP(network, nil, addr)
	if !assert.NoError(tb, err) {
		return nil
	}

	_, err = conn.Write([]byte("ping"))
	if !assert.NoError(tb, err) {
		return nil
	}

	bs := make([]byte, 10)
	n, err := conn.Read(bs)
	assert.NoError(tb, err)
	assert.Equal(tb, []byte("pong"), bs[:n])

	return conn
}

func isIpv6(ip net.IP) bool {
	return ip.To4() == nil
}
