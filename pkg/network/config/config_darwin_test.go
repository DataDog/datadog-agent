// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestDarwinConnectionTracerBackendConfiguration(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)

		cfg := New()

		require.Equal(t, DarwinConnectionTracerEbpfless, cfg.DarwinConnectionTracerBackend)
		require.True(t, cfg.DarwinConnectionTracerPacketEnabled)
		require.True(t, cfg.DarwinConnectionTracerLibprocEnabled)
		require.Equal(t, darwinPacketSnaplenDefault, cfg.DarwinConnectionTracerPacketSnaplen)
		require.Equal(t, darwinPacketBufferSizeDefault, cfg.DarwinConnectionTracerPacketBufferSize)
		require.Equal(t, darwinLibprocIntervalDefault, cfg.DarwinConnectionTracerLibprocInterval)
		require.Equal(t, darwinLibprocMaxPIDsDefault, cfg.DarwinConnectionTracerLibprocMaxPIDs)
		require.Equal(t, darwinLibprocMaxFDsPerPIDDefault, cfg.DarwinConnectionTracerLibprocMaxFDsPerPID)
		require.Equal(t, darwinLibprocObservationsDefault, cfg.DarwinConnectionTracerLibprocMaxObservations)
	})

	t.Run("nstat", func(t *testing.T) {
		systemProbe := mock.NewSystemProbe(t)
		systemProbe.SetInTest("network_config.darwin_connection_tracer_backend", DarwinConnectionTracerNStat)

		cfg := New()

		require.Equal(t, DarwinConnectionTracerNStat, cfg.DarwinConnectionTracerBackend)
	})
}

func TestDarwinConnectionTracerTuningConfiguration(t *testing.T) {
	systemProbe := mock.NewSystemProbe(t)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_backend", DarwinConnectionTracerNStatPcap)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_packet_enabled", false)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_libproc_enabled", false)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_packet_snaplen", 4096)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_packet_buffer_size", 1024*1024)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_libproc_interval", "5s")
	systemProbe.SetInTest("network_config.darwin_connection_tracer_libproc_max_pids", 100)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_libproc_max_fds_per_pid", 200)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_libproc_max_observations", 300)

	cfg := New()

	require.Equal(t, DarwinConnectionTracerNStatPcap, cfg.DarwinConnectionTracerBackend)
	require.False(t, cfg.DarwinConnectionTracerPacketEnabled)
	require.False(t, cfg.DarwinConnectionTracerLibprocEnabled)
	require.Equal(t, 4096, cfg.DarwinConnectionTracerPacketSnaplen)
	require.Equal(t, 1024*1024, cfg.DarwinConnectionTracerPacketBufferSize)
	require.Equal(t, 5*time.Second, cfg.DarwinConnectionTracerLibprocInterval)
	require.Equal(t, 100, cfg.DarwinConnectionTracerLibprocMaxPIDs)
	require.Equal(t, 200, cfg.DarwinConnectionTracerLibprocMaxFDsPerPID)
	require.Equal(t, 300, cfg.DarwinConnectionTracerLibprocMaxObservations)
}

func TestDarwinConnectionTracerTuningIsBounded(t *testing.T) {
	cfg := &Config{
		DarwinConnectionTracerPacketSnaplen:          -1,
		DarwinConnectionTracerPacketBufferSize:       darwinPacketBufferSizeMax + 1,
		DarwinConnectionTracerLibprocInterval:        time.Hour,
		DarwinConnectionTracerLibprocMaxPIDs:         0,
		DarwinConnectionTracerLibprocMaxFDsPerPID:    darwinLibprocMaxFDsPerPIDMax + 1,
		DarwinConnectionTracerLibprocMaxObservations: darwinLibprocObservationsMax + 1,
	}

	cfg.normalizeDarwinConnectionTracerConfig()

	require.Equal(t, darwinPacketSnaplenDefault, cfg.DarwinConnectionTracerPacketSnaplen)
	require.Equal(t, darwinPacketBufferSizeDefault, cfg.DarwinConnectionTracerPacketBufferSize)
	require.Equal(t, darwinLibprocIntervalDefault, cfg.DarwinConnectionTracerLibprocInterval)
	require.Equal(t, darwinLibprocMaxPIDsDefault, cfg.DarwinConnectionTracerLibprocMaxPIDs)
	require.Equal(t, darwinLibprocMaxFDsPerPIDDefault, cfg.DarwinConnectionTracerLibprocMaxFDsPerPID)
	require.Equal(t, darwinLibprocObservationsDefault, cfg.DarwinConnectionTracerLibprocMaxObservations)
}
