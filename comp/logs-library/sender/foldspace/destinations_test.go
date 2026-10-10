// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestFactoryOrderMainThenAdditional(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443, "is_reliable": true})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443, "is_reliable": true})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	require.Len(t, dest.Senders, 2)
	assert.Equal(t, SenderID(0), dest.Senders[0].ID)
	assert.Equal(t, "main.example:443", dest.Senders[0].Address)
	assert.Equal(t, SenderID(1), dest.Senders[1].ID)
	assert.Equal(t, "extra.example:443", dest.Senders[1].Address)
	assert.Equal(t, Reliable, dest.Senders[0].Class)
	assert.Equal(t, Reliable, dest.Senders[1].Class)
}

func TestUnreliableExtra(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443, "is_reliable": true})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443, "is_reliable": false})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	require.Len(t, dest.Senders, 2)
	assert.Equal(t, Reliable, dest.Senders[0].Class)
	assert.Equal(t, Unreliable, dest.Senders[1].Class)
}

func TestMRFSenderReinstated(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443, "is_reliable": true})
	mrf := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "mrf.example", "port": 443, "is_reliable": true})
	mrf.IsMRF = true
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, mrf})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	require.Len(t, dest.Senders, 2)
	assert.Equal(t, "main.example:443", dest.Senders[0].Address)
	assert.False(t, dest.Senders[0].IsMRF)
	assert.Equal(t, "mrf.example:443", dest.Senders[1].Address)
	assert.True(t, dest.Senders[1].IsMRF)
}

func TestWindowingRejected(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 4)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	_, err := BuildDestinationConfig(cfg, endpoints)
	require.Error(t, err)
	var window ErrWindowing
	require.ErrorAs(t, err, &window)
	assert.Equal(t, 2, window.Senders)
	assert.Equal(t, 8, window.PipelineDepth)
	assert.Equal(t, 4, window.MaxInflight)
}

func TestDDURLOverridesMainOnly(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)
	cfg.SetInTest("logs_config.foldspace.dd_url", "foldspace.internal:9999")

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	assert.Equal(t, "foldspace.internal:9999", dest.Senders[0].Address)
	assert.Equal(t, "extra.example:443", dest.Senders[1].Address)
}

func TestGRPCMethod(t *testing.T) {
	build := func(t *testing.T, method *string) (*DestinationConfig, error) {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
		cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)
		if method != nil {
			cfg.SetInTest("logs_config.foldspace.grpc_method", *method)
		}
		main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
		endpoints := config.NewMockEndpoints([]config.Endpoint{main})
		endpoints.Main = main
		return BuildDestinationConfig(cfg, endpoints)
	}
	ptr := func(s string) *string { return &s }

	t.Run("default", func(t *testing.T) {
		dest, err := build(t, nil)
		require.NoError(t, err)
		assert.Equal(t, statefulStreamFullMethod, dest.StreamMethod)
	})
	t.Run("empty selects the default", func(t *testing.T) {
		dest, err := build(t, ptr("  "))
		require.NoError(t, err)
		assert.Equal(t, statefulStreamFullMethod, dest.StreamMethod)
	})
	t.Run("override", func(t *testing.T) {
		dest, err := build(t, ptr(" /datadog.intake.stateful.StatefulLogsService/LogsStream "))
		require.NoError(t, err)
		assert.Equal(t, "/datadog.intake.stateful.StatefulLogsService/LogsStream", dest.StreamMethod)
	})
	for _, bad := range []string{
		"datadog.intake.stateful.StatefulLogsService/LogsStream",
		"/datadog.intake.stateful.StatefulLogsService",
		"/datadog.intake.stateful.StatefulLogsService/",
		"//LogsStream",
		"/a/b/c",
	} {
		t.Run("rejects "+bad, func(t *testing.T) {
			_, err := build(t, ptr(bad))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "logs_config.foldspace.grpc_method")
		})
	}
}

func TestStreamLifetimeIgnoresConnectionReset(t *testing.T) {
	build := func(t *testing.T, reset time.Duration) time.Duration {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
		cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

		main := config.NewMockEndpointWithOptions(map[string]interface{}{
			"host": "main.example", "port": 443, "is_reliable": true,
		})
		main.ConnectionResetInterval = reset
		endpoints := config.NewMockEndpoints([]config.Endpoint{main})
		endpoints.Main = main

		dest, err := BuildDestinationConfig(cfg, endpoints)
		require.NoError(t, err)
		return dest.Core.StreamLifetime
	}

	// connection_reset_interval governs HTTP connection recycling, which shares
	// neither its cost nor its bound with ending an interning epoch. Whatever it
	// is set to, the default stream lifetime is the library's.
	assert.Equal(t, defaultStreamLifetime, build(t, 0))
	assert.Equal(t, defaultStreamLifetime, build(t, 30*time.Second))
	assert.Equal(t, defaultStreamLifetime, build(t, 900*time.Second))
}

func TestStreamLifetimeConfigured(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"15m", 15 * time.Minute},
		{"90s", 90 * time.Second},
		{"299.7s", 299700 * time.Millisecond},
		{"299700000000ns", 299700 * time.Millisecond},
	} {
		t.Run(tc.value, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("logs_config.foldspace.stream_lifetime", tc.value)
			main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
			main.ConnectionResetInterval = 30 * time.Second
			endpoints := config.NewMockEndpoints([]config.Endpoint{main})
			endpoints.Main = main
			dest, err := BuildDestinationConfig(cfg, endpoints)
			require.NoError(t, err)
			assert.Equal(t, tc.want, dest.Core.StreamLifetime)
			assert.Equal(t, 30*time.Second, main.ConnectionResetInterval)
		})
	}
}

// Zero must never reach the library, where it means rotate on every open
// rather than the default, so anything but a positive duration with units is
// rejected instead of silently replaced.
func TestStreamLifetimeInvalid(t *testing.T) {
	for _, value := range []string{"", "0", "0s", "-1s", "900", "NaN", "bogus", "999999999999999999h"} {
		t.Run(value, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("logs_config.foldspace.stream_lifetime", value)
			main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
			endpoints := config.NewMockEndpoints([]config.Endpoint{main})
			endpoints.Main = main
			dest, err := BuildDestinationConfig(cfg, endpoints)
			require.Error(t, err)
			assert.Nil(t, dest)
			assert.Contains(t, err.Error(), "logs_config.foldspace.stream_lifetime")
		})
	}
}

func TestStreamTimeouts(t *testing.T) {
	build := func(t *testing.T, settings map[string]interface{}) *DestinationConfig {
		t.Helper()
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
		cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)
		for k, v := range settings {
			cfg.SetInTest(k, v)
		}
		main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
		endpoints := config.NewMockEndpoints([]config.Endpoint{main})
		endpoints.Main = main
		dest, err := BuildDestinationConfig(cfg, endpoints)
		require.NoError(t, err)
		return dest
	}

	t.Run("defaults", func(t *testing.T) {
		dest := build(t, nil)
		assert.Equal(t, defaultKeepaliveTime, dest.KeepaliveTime)
		assert.Equal(t, defaultKeepaliveTimeout, dest.KeepaliveTimeout)
		assert.Equal(t, defaultAckTimeout, dest.AckTimeout)
	})

	t.Run("configured", func(t *testing.T) {
		dest := build(t, map[string]interface{}{
			"logs_config.http_timeout":                30,
			"logs_config.foldspace.keepalive_time":    "10m",
			"logs_config.foldspace.keepalive_timeout": "45s",
			"logs_config.foldspace.ack_timeout":       "2m",
		})
		assert.Equal(t, 30*time.Second, dest.ConnectTimeout)
		assert.Equal(t, 30*time.Second, dest.SendTimeout)
		assert.Equal(t, 10*time.Minute, dest.KeepaliveTime)
		assert.Equal(t, 45*time.Second, dest.KeepaliveTimeout)
		assert.Equal(t, 2*time.Minute, dest.AckTimeout)
	})
}

func TestStreamLifetimeEnvOverride(t *testing.T) {
	t.Setenv("DD_LOGS_CONFIG_FOLDSPACE_STREAM_LIFETIME", "90s")
	cfg := configmock.New(t)
	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main})
	endpoints.Main = main
	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, dest.Core.StreamLifetime)
}
