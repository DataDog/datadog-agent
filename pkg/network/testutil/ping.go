// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// PingTCP connects to the provided IP address over TCP/TCPv6, sends the string "ping",
// reads from the connection, and returns the open connection for further use/inspection.
func PingTCP(tb testing.TB, ip net.IP, port int) net.Conn {
	addr := fmt.Sprintf("%s:%d", ip, port)
	network := "tcp"
	if isIpv6(ip) {
		network = "tcp6"
		addr = fmt.Sprintf("[%s]:%d", ip, port)
	}

	conn, err := net.DialTimeout(network, addr, time.Second)
	require.NoError(tb, err)

	_, err = conn.Write([]byte("ping"))
	require.NoError(tb, err)
	bs := make([]byte, 10)
	_, err = conn.Read(bs)
	require.NoError(tb, err)

	return conn
}

// PingUDP connects to the provided IP address over UDP/UDPv6, sends the string "ping",
// and returns the open connection for further use/inspection.
func PingUDP(t *testing.T, ip net.IP, port int) net.Conn {
	network := "udp"
	if isIpv6(ip) {
		network = "udp6"
	}
	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}
	conn, err := net.DialUDP(network, nil, addr)
	require.NoError(t, err)

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	return conn
}

func isIpv6(ip net.IP) bool {
	return ip.To4() == nil
}
