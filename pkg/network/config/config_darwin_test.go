// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestDarwinConnectionTracerBackendConfiguration(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)

		cfg := New()

		require.Equal(t, DarwinConnectionTracerEbpfless, cfg.DarwinConnectionTracerBackend)
		require.True(t, cfg.DarwinConnectionTracerPacketEnabled)
		require.Equal(t, darwinPacketSnaplenDefault, cfg.DarwinConnectionTracerPacketSnaplen)
		require.Equal(t, darwinPacketBufferSizeDefault, cfg.DarwinConnectionTracerPacketBufferSize)
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
	systemProbe.SetInTest("network_config.darwin_connection_tracer_packet_snaplen", 4096)
	systemProbe.SetInTest("network_config.darwin_connection_tracer_packet_buffer_size", 1024*1024)

	cfg := New()

	require.Equal(t, DarwinConnectionTracerNStatPcap, cfg.DarwinConnectionTracerBackend)
	require.False(t, cfg.DarwinConnectionTracerPacketEnabled)
	require.Equal(t, 4096, cfg.DarwinConnectionTracerPacketSnaplen)
	require.Equal(t, 1024*1024, cfg.DarwinConnectionTracerPacketBufferSize)
}

func TestDarwinConnectionTracerTuningIsBounded(t *testing.T) {
	cfg := &Config{
		DarwinConnectionTracerPacketSnaplen:    -1,
		DarwinConnectionTracerPacketBufferSize: darwinPacketBufferSizeMax + 1,
	}

	cfg.normalizeDarwinConnectionTracerConfig()

	require.Equal(t, darwinPacketSnaplenDefault, cfg.DarwinConnectionTracerPacketSnaplen)
	require.Equal(t, darwinPacketBufferSizeDefault, cfg.DarwinConnectionTracerPacketBufferSize)
}
