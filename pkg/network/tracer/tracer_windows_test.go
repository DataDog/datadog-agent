// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package tracer

import (
	"testing"
)

func dnsSupported(t *testing.T) bool {
	return true
}

func httpSupported(t *testing.T) bool {
	return false
}

func httpsSupported(t *testing.T) bool {
	return false
}

func TestTCPEstablishedPreExistingConn(t *testing.T) {
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err := server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	laddr, raddr := c.LocalAddr(), c.RemoteAddr()

	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()
	cfg.TCPClosedTimeout = 500 * time.Millisecond

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	c.Write([]byte("hello"))
	c.Close()
	// Wait for the connection to be sent from the perf buffer
	time.Sleep(cfg.TCPClosedTimeout)
	connections := getConnections(t, tr)
	conn, ok := findConnection(laddr, raddr, connections)

	require.True(t, ok)
	assert.Equal(t, uint32(0), conn.Monotonic.TCPEstablished)
	assert.Equal(t, uint32(1), conn.Monotonic.TCPClosed)
}
