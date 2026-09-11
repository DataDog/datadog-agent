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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
)

const nstatIncompleteQualificationEnv = "RUN_NSTAT_FUNCTIONAL_TEST"

func TestNStatQualificationLoopbackTCPErrorsIncomplete(t *testing.T) {
	if os.Getenv(nstatIncompleteQualificationEnv) == "" {
		t.Skipf("set %s=1 to run live NStat qualification", nstatIncompleteQualificationEnv)
	}

	tracer, err := newNStatTracer(testNStatConfig())
	require.NoError(t, err)
	t.Cleanup(tracer.Stop)
	require.NoError(t, tracer.Start(nil))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	serverAddr := listener.Addr().(*net.TCPAddr)

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		accepted <- conn
	}()

	client, err := net.DialTimeout("tcp", serverAddr.String(), 2*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	_, err = client.Write([]byte("nstat-incomplete"))
	require.NoError(t, err)

	server := <-accepted
	t.Cleanup(func() { _ = server.Close() })
	buf := make([]byte, 16)
	_, err = io.ReadFull(server, buf)
	require.NoError(t, err)

	clientAddr := client.LocalAddr().(*net.TCPAddr)
	require.Eventually(t, func() bool {
		var buffer network.ConnectionBuffer
		if err := tracer.GetConnections(&buffer, nil); err != nil {
			return false
		}
		for _, conn := range buffer.Connections() {
			if conn.Type != network.TCP || conn.Pid == 0 {
				continue
			}
			if (conn.SPort == uint16(clientAddr.Port) && conn.DPort == uint16(serverAddr.Port)) ||
				(conn.SPort == uint16(serverAddr.Port) && conn.DPort == uint16(clientAddr.Port)) {
				return conn.HasTCPErrorsIncomplete()
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "loopback NStat TCP row must carry tcp_errors_incomplete")
}

func TestNStatQualificationPacketSidecarDeathDoesNotFallback(t *testing.T) {
	primary := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	packet := newDarwinPacketSidecar(&fakeDarwinPacketSource{}, primary, 10)
	composite := newDarwinCompositeTracerWithComponents(primary, packet, nil)
	var fallbacks int
	composite.setRuntimeFailureCallback(func(error) { fallbacks++ })
	composite.handlePacketFailure(io.ErrClosedPipe)

	require.Equal(t, 0, fallbacks)
	status := composite.darwinStatus()
	require.Equal(t, darwinSidecarStopped, status.PacketEnrichment)
	require.NotEqual(t, darwinSidecarHealthy, status.PacketEnrichment)
	require.True(t, status.SourceHealthy)
}
