// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
)

// startFakePacketCaptureServer starts a unix-socket HTTP server registered at
// the same socket-path config key run_capture_socket.go reads, serving
// /packet_capture/capture with handler.
func startFakePacketCaptureServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	systemProbeConfig := configmock.NewSystemProbe(t)

	// Use /tmp directly (not t.TempDir()) to keep the socket path short
	// enough for the unix domain socket path length limit.
	tempDir, err := os.MkdirTemp("/tmp", "pcaptest")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	socketPath := tempDir + "/capture.sock"
	systemProbeConfig.SetInTest("system_probe_config.sysprobe_socket", socketPath)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /packet_capture/capture", handler)

	listener, err := server.NewListener(socketPath)
	require.NoError(t, err)
	httpServer := &http.Server{Handler: mux}
	go httpServer.Serve(listener) //nolint:errcheck
	t.Cleanup(func() { httpServer.Close() })
}

func TestSocketCaptureTrigger_Success(t *testing.T) {
	const pcapBody = "fake-pcap-bytes"

	startFakePacketCaptureServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Trailer", "X-Packet-Count, X-Packets-Dropped, X-Headers-Truncated, X-Capture-Errors")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pcapBody))
		w.Header().Set("X-Packet-Count", "42")
		w.Header().Set("X-Packets-Dropped", "7")
		w.Header().Set("X-Headers-Truncated", "3")
		w.Header().Set("X-Capture-Errors", "1")
	})

	trigger := &socketCaptureTrigger{}
	out, err := trigger.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "tcp port 443",
		DurationSecs: 1,
	})
	require.NoError(t, err)
	defer os.Remove(out.PcapPath)

	assert.Equal(t, 42, out.PacketCount)
	assert.Equal(t, 7, out.PacketsDropped)
	assert.Equal(t, 3, out.HeadersTruncated)
	assert.Equal(t, 1, out.Errors)
	assert.Equal(t, int64(len(pcapBody)), out.FileSizeBytes)

	data, err := os.ReadFile(out.PcapPath)
	require.NoError(t, err)
	assert.Equal(t, pcapBody, string(data))
}

func TestSocketCaptureTrigger_ErrorStatus(t *testing.T) {
	startFakePacketCaptureServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "durationSecs must be positive", http.StatusBadRequest)
	})

	trigger := &socketCaptureTrigger{}
	_, err := trigger.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "tcp port 443",
		DurationSecs: 1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "durationSecs must be positive")
}

func TestSocketCaptureTrigger_SocketPathNotConfigured(t *testing.T) {
	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetInTest("system_probe_config.sysprobe_socket", "")

	trigger := &socketCaptureTrigger{}
	_, err := trigger.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "tcp port 443",
		DurationSecs: 1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}
