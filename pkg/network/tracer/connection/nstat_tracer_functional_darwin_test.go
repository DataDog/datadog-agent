// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
	processutil "github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestNStatTracerFunctionalLoopbackPID(t *testing.T) {
	if os.Getenv("RUN_NSTAT_FUNCTIONAL_TEST") != "1" {
		t.Skip("set RUN_NSTAT_FUNCTIONAL_TEST=1 to exercise the private kernel control")
	}

	control, err := nstat.OpenControl()
	require.NoError(t, err)
	cfg := testNStatConfig()
	cfg.MaxTrackedConnections = 16384
	tracer := newNStatTracerWithControl(cfg, control)
	tracer.includeLoopback = true
	require.NoError(t, tracer.Start(func(*network.ConnectionStats) {}))
	defer tracer.Stop()

	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer listener.Close()
	client, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	require.NoError(t, err)
	defer client.Close()
	server, err := listener.AcceptTCP()
	require.NoError(t, err)
	defer server.Close()

	_, err = client.Write([]byte("nstat-client"))
	require.NoError(t, err)
	payload := make([]byte, len("nstat-client"))
	_, err = io.ReadFull(server, payload)
	require.NoError(t, err)

	clientLocal := client.LocalAddr().(*net.TCPAddr).AddrPort()
	clientRemote := client.RemoteAddr().(*net.TCPAddr).AddrPort()
	var lastErr error
	var ownConnections []network.ConnectionTuple
	var matchingPorts []network.ConnectionTuple
	found := assert.Eventually(t, func() bool {
		var buffer network.ConnectionBuffer
		lastErr = tracer.GetConnections(&buffer, nil)
		if lastErr != nil {
			return false
		}
		for _, conn := range buffer.Connections() {
			if conn.Pid == uint32(os.Getpid()) && len(ownConnections) < 20 {
				ownConnections = append(ownConnections, conn.ConnectionTuple)
			}
			if (conn.SPort == clientLocal.Port() || conn.DPort == clientLocal.Port()) &&
				len(matchingPorts) < 20 {
				matchingPorts = append(matchingPorts, conn.ConnectionTuple)
			}
			if conn.Pid == uint32(os.Getpid()) &&
				conn.Source.Addr == clientLocal.Addr() &&
				conn.SPort == clientLocal.Port() &&
				conn.Dest.Addr == clientRemote.Addr() &&
				conn.DPort == clientRemote.Port() {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	tracer.mu.Lock()
	sourceCount := len(tracer.sources)
	describedCount := 0
	for _, source := range tracer.sources {
		if source.flow != nil {
			describedCount++
		}
	}
	descriptionQueueLength := len(tracer.descriptionQueue)
	tracer.mu.Unlock()
	require.Truef(
		t,
		found,
		"NStat tracer did not emit loopback client %s -> %s for PID %d (last error: %v, sources: %d, described: %d, description queue: %d, own connections: %v, matching ports: %v)",
		clientLocal,
		clientRemote,
		os.Getpid(),
		lastErr,
		sourceCount,
		describedCount,
		descriptionQueueLength,
		ownConnections,
		matchingPorts,
	)
}

func TestNStatTracerFunctionalRecordingDomains(t *testing.T) {
	if os.Getenv("RUN_NSTAT_FUNCTIONAL_TEST") != "1" {
		t.Skip("set RUN_NSTAT_FUNCTIONAL_TEST=1 to exercise the private kernel control")
	}
	if !darwinKernelRecordsDomains(darwinKernelRelease()) {
		t.Skip("connection domains are recorded on macOS 12.1 and later")
	}

	control, err := nstat.OpenControl()
	require.NoError(t, err)
	cfg := testNStatConfig()
	cfg.MaxTrackedConnections = 16384
	tracer := newNStatTracerWithControl(cfg, control)
	tracer.includeLoopback = true
	// Sources are then described with update requests, after which the
	// kernel reports each close as a single update.
	tracer.recordDomain = func(processutil.Address, uint16, string) {}
	domainItems := func() float64 {
		return nstatTracerTelemetry.domains.WithValues("domain").Get() +
			nstatTracerTelemetry.domains.WithValues("empty").Get()
	}
	domainItemsBefore := domainItems()
	var mu sync.Mutex
	var closed []network.ConnectionStats
	require.NoError(t, tracer.Start(func(conn *network.ConnectionStats) {
		mu.Lock()
		defer mu.Unlock()
		closed = append(closed, *conn)
	}))
	defer tracer.Stop()

	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer listener.Close()
	client, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	require.NoError(t, err)
	defer client.Close()
	server, err := listener.AcceptTCP()
	require.NoError(t, err)
	defer server.Close()
	_, err = client.Write([]byte("nstat-client"))
	require.NoError(t, err)
	payload := make([]byte, len("nstat-client"))
	_, err = io.ReadFull(server, payload)
	require.NoError(t, err)

	clientLocal := client.LocalAddr().(*net.TCPAddr).AddrPort()
	clientRemote := client.RemoteAddr().(*net.TCPAddr).AddrPort()
	isClient := func(conn *network.ConnectionStats) bool {
		return conn.Source.Addr == clientLocal.Addr() && conn.SPort == clientLocal.Port() &&
			conn.Dest.Addr == clientRemote.Addr() && conn.DPort == clientRemote.Port()
	}
	require.Eventually(t, func() bool {
		tracer.mu.Lock()
		defer tracer.mu.Unlock()
		for _, source := range tracer.sources {
			if source.conn != nil && isClient(source.conn) {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "the client connection was not published")
	if os.Geteuid() == 0 {
		require.Eventually(t, func() bool {
			return domainItems() > domainItemsBefore
		}, 5*time.Second, 10*time.Millisecond, "no update carried a domain item")
	}

	require.NoError(t, client.Close())
	require.NoError(t, server.Close())
	var final network.ConnectionStats
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range closed {
			if isClient(&conn) {
				final = conn
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "the client connection close was not reported")
	require.True(t, final.IsClosed)
	require.Equal(t, uint32(os.Getpid()), final.Pid)
	require.Equal(t, uint64(len("nstat-client")), final.Monotonic.SentBytes)
}
