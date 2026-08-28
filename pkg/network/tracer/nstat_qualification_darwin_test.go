// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"bytes"
	"io"
	"net"
	"os"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	networkmarshal "github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	networkunmarshal "github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
)

func TestNStatQualificationConnectionsPayload(t *testing.T) {
	if os.Getenv("RUN_NSTAT_FUNCTIONAL_TEST") != "1" {
		t.Skip("set RUN_NSTAT_FUNCTIONAL_TEST=1 to exercise the complete /connections model path")
	}

	cfg := config.New()
	cfg.DarwinConnectionTracerBackend = config.DarwinConnectionTracerNStat
	cfg.DarwinConnectionTracerPacketEnabled = false
	cfg.DarwinConnectionTracerLibprocEnabled = false
	cfg.DNSInspection = false
	tr, err := NewTracer(cfg, nil, nil)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)
	require.NoError(t, tr.RegisterClient("nstat-qualification"))

	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer listener.Close()
	client, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	require.NoError(t, err)
	defer client.Close()
	server, err := listener.AcceptTCP()
	require.NoError(t, err)
	defer server.Close()

	payload := []byte("nstat-product-path")
	_, err = client.Write(payload)
	require.NoError(t, err)
	received := make([]byte, len(payload))
	_, err = io.ReadFull(server, received)
	require.NoError(t, err)

	localPort := int32(client.LocalAddr().(*net.TCPAddr).Port)
	remotePort := int32(client.RemoteAddr().(*net.TCPAddr).Port)
	require.Eventually(t, func() bool {
		var buffer network.ConnectionBuffer
		if err := tr.connTracer.GetConnections(&buffer, nil); err != nil {
			return false
		}
		for _, conn := range buffer.Connections() {
			if conn.Pid == uint32(os.Getpid()) &&
				int32(conn.SPort) == localPort &&
				int32(conn.DPort) == remotePort &&
				conn.Monotonic.SentBytes == uint64(len(payload)) &&
				conn.Direction == network.OUTGOING {
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond)

	conns, cleanup, err := tr.GetActiveConnections("nstat-qualification")
	require.NoError(t, err)
	defer cleanup()
	found := false
	for _, conn := range conns.Conns {
		if conn.Pid == uint32(os.Getpid()) &&
			int32(conn.SPort) == localPort &&
			int32(conn.DPort) == remotePort {
			require.Equal(t, uint64(len(payload)), conn.Last.SentBytes)
			require.Equal(t, network.OUTGOING, conn.Direction)
			found = true
			break
		}
	}
	require.True(t, found)

	var encoded bytes.Buffer
	modeler, err := networkmarshal.NewConnectionsModeler(conns)
	require.NoError(t, err)
	defer modeler.Close()
	require.NoError(t, networkmarshal.GetMarshaler(networkmarshal.ContentTypeProtobuf).Marshal(conns, &encoded, modeler))
	decoded, err := networkunmarshal.GetUnmarshaler(networkunmarshal.ContentTypeProtobuf).Unmarshal(encoded.Bytes())
	require.NoError(t, err)
	found = false
	for _, conn := range decoded.Conns {
		if conn.Pid == int32(os.Getpid()) &&
			conn.Laddr.Port == localPort &&
			conn.Raddr.Port == remotePort {
			require.Equal(t, uint64(len(payload)), conn.LastBytesSent)
			require.Equal(t, model.ConnectionDirection_outgoing, conn.Direction)
			found = true
			break
		}
	}
	require.True(t, found)
}
