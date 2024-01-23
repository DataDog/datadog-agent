// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// GetOpenPortTCP lets the OS pick an open port for TCP and
// returns it
func GetOpenPortTCP(t testing.TB) int {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

// GetOpenPortUDP lets the OS pick an open port for UDP and
// returns it
func GetOpenPortUDP(t testing.TB) int {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn.LocalAddr().(*net.UDPAddr).Port
}
