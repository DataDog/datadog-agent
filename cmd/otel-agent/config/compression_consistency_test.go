// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestDDOTCompressionConsistency guards the invariant that the DDOT (otel-agent)
// exporter compresses every signal with a single algorithm — zstd — so the
// per-signal compressors cannot silently diverge.
//
// Metrics and logs are config-driven: their algorithm and level come from the
// agent-config keys set in NewConfigComponent, asserted here directly.
//
// Traces are NOT config-driven: cmd/otel-agent/subcommands/run/command.go wires
// comp/trace/compression/fx-zstd (zstd at BestSpeed; the level is intentionally
// fixed for traces). "zstd" below is therefore the shared invariant all three
// signals must satisfy. The on-the-wire guard that traces actually ship zstd
// lives in the OTLP integration test (Content-Encoding assertion).
func TestDDOTCompressionConsistency(t *testing.T) {
	configmock.New(t)
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"})
	require.NoError(t, err)

	const wantAlgo = "zstd"
	const wantLevel = 3

	metricsKind := c.GetString("serializer_compressor_kind")
	logsKind := c.GetString("logs_config.compression_kind")

	// All signals share one algorithm by default.
	assert.Equal(t, wantAlgo, metricsKind, "metrics must default to zstd")
	assert.Equal(t, wantAlgo, logsKind, "logs must default to zstd")
	assert.Equal(t, metricsKind, logsKind, "metrics and logs compression algorithms must not diverge")

	// logs_config.compression_kind must be IsConfigured() (set at a non-default source)
	// so the logs pipeline does NOT fall back to gzip when logs_config.additional_endpoints
	// is set. additional_endpoints are other Datadog endpoints (multi-region / dual-ship /
	// MRF) that accept zstd, so every log endpoint should ship zstd — matching the metrics
	// forwarder, which already fans one zstd payload to all endpoints.
	assert.True(t, c.IsConfigured("logs_config.compression_kind"),
		"logs_config.compression_kind must be configured so the additional_endpoints gzip fallback is bypassed")

	// Levels default to 3 for the signals that support a configurable level.
	assert.Equal(t, wantLevel, c.GetInt("serializer_zstd_compressor_level"), "metrics zstd level should default to 3")
	assert.Equal(t, wantLevel, c.GetInt("logs_config.zstd_compression_level"), "logs zstd level should default to 3")

	// DDOT deliberately stays on the v2 metrics intake (v3 is a separate effort).
	assert.Equal(t, "false", c.GetString("use_v3_api.series.enabled"), "DDOT must stay on the v2 metrics intake")
}

// TestDDOTCompressionLevelOverridable verifies the per-signal compression level
// stays overridable (e.g. via DD_* env vars) rather than being forced, so the
// SourceDefault precedence chosen in NewConfigComponent does not lock operators out.
func TestDDOTCompressionLevelOverridable(t *testing.T) {
	configmock.New(t)
	t.Setenv("DD_SERIALIZER_ZSTD_COMPRESSOR_LEVEL", "6")
	t.Setenv("DD_LOGS_CONFIG_ZSTD_COMPRESSION_LEVEL", "9")

	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"})
	require.NoError(t, err)

	assert.Equal(t, 6, c.GetInt("serializer_zstd_compressor_level"), "metrics zstd level should be overridable via env")
	assert.Equal(t, 9, c.GetInt("logs_config.zstd_compression_level"), "logs zstd level should be overridable via env")

	// Overriding only the level must not change the algorithm.
	assert.Equal(t, "zstd", c.GetString("serializer_compressor_kind"))
	assert.Equal(t, "zstd", c.GetString("logs_config.compression_kind"))
}

// TestDDOTHostMetadataCompression explicitly guards that host metadata — a
// separate submission path from metrics — is compressed with zstd.
//
// The DD exporter pushes host metadata through the agent serializer:
// serializer.SendHostMetadata -> sendMetadata -> split.CheckSizeAndSerialize(m,
// true /*compress*/, s.Strategy). s.Strategy is the serializer's compressor, built
// from the SAME serializer_compressor_kind / serializer_zstd_compressor_level keys
// as the metrics series/sketches path — there is no dedicated host-metadata
// compression knob. So host metadata is zstd (level 3) whenever metrics are; this
// test pins that so the two cannot silently diverge.
func TestDDOTHostMetadataCompression(t *testing.T) {
	configmock.New(t)
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"})
	require.NoError(t, err)

	assert.Equal(t, "zstd", c.GetString("serializer_compressor_kind"),
		"host metadata shares the metrics compressor (serializer_compressor_kind); both must be zstd")
	assert.Equal(t, 3, c.GetInt("serializer_zstd_compressor_level"),
		"host metadata uses the metrics zstd level")
}
