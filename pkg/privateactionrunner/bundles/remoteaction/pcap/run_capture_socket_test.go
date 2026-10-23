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

	pcapuploadermock "github.com/DataDog/datadog-agent/comp/networkpcap/uploader/mock"
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
		w.Header().Set("X-Capture-Errors", "0")
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
	assert.Equal(t, 0, out.Errors)
	assert.Equal(t, int64(len(pcapBody)), out.FileSizeBytes)

	data, err := os.ReadFile(out.PcapPath)
	require.NoError(t, err)
	assert.Equal(t, pcapBody, string(data))
}

// TestSocketCaptureTrigger_CaptureErrorsTrailer covers the case where
// system-probe has already committed HTTP 200 and streamed a (possibly
// empty or partial) PCAP body before capturer.Start failed or hit per-packet
// errors: the only signal for that is the X-Capture-Errors trailer, which
// the client must reject on rather than treating as a successful capture.
func TestSocketCaptureTrigger_CaptureErrorsTrailer(t *testing.T) {
	const pcapBody = "fake-pcap-bytes"

	startFakePacketCaptureServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Trailer", "X-Packet-Count, X-Packets-Dropped, X-Headers-Truncated, X-Capture-Errors")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pcapBody))
		w.Header().Set("X-Packet-Count", "0")
		w.Header().Set("X-Packets-Dropped", "0")
		w.Header().Set("X-Headers-Truncated", "0")
		w.Header().Set("X-Capture-Errors", "1")
	})

	trigger := &socketCaptureTrigger{}
	out, err := trigger.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "tcp port 443",
		DurationSecs: 1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capture error")
	// No artifact should be left behind for a rejected capture.
	assert.Empty(t, out.PcapPath)
	if out.PcapPath != "" {
		_ = os.Remove(out.PcapPath)
	}
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
	systemProbeConfig.SetInTest("packet_capture.socket", "")

	trigger := &socketCaptureTrigger{}
	_, err := trigger.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "tcp port 443",
		DurationSecs: 1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestCaptureSocketPath(t *testing.T) {
	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetInTest("system_probe_config.sysprobe_socket", "/var/run/sysprobe/sysprobe.sock")

	systemProbeConfig.SetInTest("packet_capture.socket", "")
	assert.Equal(t, "/var/run/sysprobe/sysprobe.sock", captureSocketPath())

	systemProbeConfig.SetInTest("packet_capture.socket", "/var/run/sysprobe-capture/capture.sock")
	assert.Equal(t, "/var/run/sysprobe-capture/capture.sock", captureSocketPath())
}

// runOverSocket runs the action end-to-end over the socket configured by the
// test and returns the outcome counter delta for reason and socket.
func runOverSocket(t *testing.T, reason, socket string) (float64, error) {
	t.Helper()
	before := outcomeCount(reason, socket)
	handler := NewRunCaptureHandler(pcapuploadermock.New())
	_, err := handler.Run(context.Background(), newTask(map[string]interface{}{"durationSecs": 1}), nil)
	return outcomeCount(reason, socket) - before, err
}

func TestRunCaptureOutcome_SuccessOverMainSocket(t *testing.T) {
	startFakePacketCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-pcap-bytes"))
	})

	delta, err := runOverSocket(t, reasonSuccess, socketMain)
	require.NoError(t, err)
	assert.Equal(t, 1.0, delta)
}

func TestRunCaptureOutcome_SuccessOverCaptureSocket(t *testing.T) {
	startFakePacketCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-pcap-bytes"))
	})
	// Point the capture-only socket at the fake server.
	cfg := configmock.NewSystemProbe(t)
	cfg.SetInTest("packet_capture.socket", cfg.GetString("system_probe_config.sysprobe_socket"))

	delta, err := runOverSocket(t, reasonSuccess, socketCapture)
	require.NoError(t, err)
	assert.Equal(t, 1.0, delta)
}

func TestRunCaptureOutcome_SocketUnreachable(t *testing.T) {
	cfg := configmock.NewSystemProbe(t)
	cfg.SetInTest("system_probe_config.sysprobe_socket", "/tmp/pcaptest-does-not-exist.sock")
	cfg.SetInTest("packet_capture.socket", "")

	delta, err := runOverSocket(t, reasonSocketUnreachable, socketMain)
	require.Error(t, err)
	assert.Equal(t, 1.0, delta)
}

func TestRunCaptureOutcome_SocketNotConfigured(t *testing.T) {
	cfg := configmock.NewSystemProbe(t)
	cfg.SetInTest("system_probe_config.sysprobe_socket", "")
	cfg.SetInTest("packet_capture.socket", "")

	delta, err := runOverSocket(t, reasonSocketUnreachable, socketNone)
	require.Error(t, err)
	assert.Equal(t, 1.0, delta)
}

func TestRunCaptureOutcome_SystemProbeStatus(t *testing.T) {
	for code, reason := range map[int]string{
		http.StatusBadRequest:          reasonBadRequest,
		http.StatusConflict:            reasonBusy,
		http.StatusInternalServerError: reasonCaptureError,
	} {
		t.Run(reason, func(t *testing.T) {
			startFakePacketCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "rejected", code)
			})

			delta, err := runOverSocket(t, reason, socketMain)
			require.Error(t, err)
			assert.Equal(t, 1.0, delta)
		})
	}
}

func TestRunCaptureOutcome_CaptureErrorsTrailer(t *testing.T) {
	startFakePacketCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Trailer", "X-Capture-Errors")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-pcap-bytes"))
		w.Header().Set("X-Capture-Errors", "1")
	})

	delta, err := runOverSocket(t, reasonCaptureError, socketMain)
	require.Error(t, err)
	assert.Equal(t, 1.0, delta)
}
