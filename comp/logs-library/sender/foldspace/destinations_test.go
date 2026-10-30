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
	"github.com/DataDog/datadog-agent/pkg/config/model"
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

func TestCredentialStateFollowsDelegatedAuth(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	const directive = "DELA[some-org-uuid, aws]"
	main := config.NewEndpoint("0123456789abcdef", "api_key", "main.example", 443, config.EmptyPathPrefix, true)
	pending := config.NewEndpoint(directive, model.DelaLogsEndpointsKey, "awaiting.example", 443, config.EmptyPathPrefix, true)
	stranded := config.NewEndpoint(directive, "logs_config.api_key", "stranded.example", 443, config.EmptyPathPrefix, true)
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, pending, stranded})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	transport := NewGRPCTransport(dest)

	for sender, want := range []struct{ ready, awaiting bool }{
		{ready: true},
		{awaiting: true},
		{},
	} {
		ready, awaiting := transport.CredentialState(SenderID(sender))
		assert.Equal(t, want.ready, ready, "sender %d ready", sender)
		assert.Equal(t, want.awaiting, awaiting, "sender %d awaiting", sender)
	}
	assert.Empty(t, dest.Senders[1].APIKey(), "a pending directive is never sent as a key")

	var _ CredentialGate = transport
	ready, awaiting := (&GRPCTransport{specs: []SenderSpec{{}}}).CredentialState(0)
	assert.True(t, ready, "a sender with no credential state is always ready")
	assert.False(t, awaiting)
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

// The library refuses a payload ceiling above the protocol's batch cap, so a
// larger batch_max_content_size is held to the cap rather than failing the
// core's construction.
func TestMaxPayloadHeldToProtocolCap(t *testing.T) {
	for _, tc := range []struct {
		configured, want int
	}{
		{0, 1024 * 1024},
		{5_000_000, 5_000_000},
		{maxProtocolBatchBytes, maxProtocolBatchBytes},
		{10 * 1024 * 1024, maxProtocolBatchBytes},
	} {
		cfg := configmock.New(t)
		main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
		endpoints := config.NewMockEndpoints([]config.Endpoint{main})
		endpoints.Main = main
		endpoints.BatchMaxContentSize = tc.configured
		dest, err := BuildDestinationConfig(cfg, endpoints)
		require.NoError(t, err)
		assert.Equal(t, tc.want, dest.Core.MaxPayloadBytes, "batch_max_content_size %d", tc.configured)
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

func TestCompressionKind(t *testing.T) {
	build := func(t *testing.T, kind string) (*DestinationConfig, error) {
		cfg := configmock.New(t)
		if kind != "" {
			cfg.SetInTest("logs_config.foldspace.compression_kind", kind)
		}
		main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
		main.UseCompression = true
		main.CompressionKind = "zstd"
		main.CompressionLevel = 3
		endpoints := config.NewMockEndpoints([]config.Endpoint{main})
		endpoints.Main = main
		return BuildDestinationConfig(cfg, endpoints)
	}

	t.Run("empty follows logs_config compression", func(t *testing.T) {
		dest, err := build(t, "")
		require.NoError(t, err)
		assert.Equal(t, Zstd, dest.Core.Compression)
		assert.Equal(t, 3, dest.Core.ZstdLevel)
	})
	for _, kind := range []string{"jsplit", " JSplit "} {
		t.Run(kind, func(t *testing.T) {
			dest, err := build(t, kind)
			require.NoError(t, err)
			assert.Equal(t, JSplit, dest.Core.Compression)
			assert.Zero(t, dest.Core.ZstdLevel)
		})
	}
	for _, kind := range []string{"zstd", "gzip", "bogus"} {
		t.Run(kind, func(t *testing.T) {
			dest, err := build(t, kind)
			require.ErrorContains(t, err, "logs_config.foldspace.compression_kind")
			assert.Nil(t, dest)
		})
	}
}

func TestCompressionKindEnvOverride(t *testing.T) {
	t.Setenv("DD_LOGS_CONFIG_FOLDSPACE_COMPRESSION_KIND", "jsplit")
	cfg := configmock.New(t)
	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main})
	endpoints.Main = main
	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	assert.Equal(t, JSplit, dest.Core.Compression)
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
