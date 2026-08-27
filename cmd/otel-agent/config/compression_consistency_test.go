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
	"github.com/DataDog/datadog-agent/pkg/util/compression"
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
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	const wantAlgo = "zstd"
	const wantLevel = ddotZstdCompressionLevel

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

	// DDOT no longer opts out of v3: now that it compresses with zstd (v3-compatible),
	// it inherits the global use_v3_api.series.enabled default (datadog_only).
	assert.Equal(t, "datadog_only", c.GetString("use_v3_api.series.enabled"), "DDOT inherits the global datadog_only v3 default")
}

// TestDDOTCompressionLevelOverridable verifies the per-signal compression level
// stays overridable (e.g. via DD_* env vars) rather than being forced, so the
// SourceDefault precedence chosen in NewConfigComponent does not lock operators out.
func TestDDOTCompressionLevelOverridable(t *testing.T) {
	configmock.New(t)
	t.Setenv("DD_SERIALIZER_ZSTD_COMPRESSOR_LEVEL", "6")
	t.Setenv("DD_LOGS_CONFIG_ZSTD_COMPRESSION_LEVEL", "9")

	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
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
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "zstd", c.GetString("serializer_compressor_kind"),
		"host metadata shares the metrics compressor (serializer_compressor_kind); both must be zstd")
	assert.Equal(t, ddotZstdCompressionLevel, c.GetInt("serializer_zstd_compressor_level"),
		"host metadata uses the metrics zstd level")
}

// TestDDOTSupportedCompressors covers every compression algorithm DDOT supports
// per signal — not just the zstd default — on two axes:
//
//  1. Selectability: each supported algorithm can be chosen via the DDOT config
//     surface (DD_* env override beats the SourceDefault/SourceFile the
//     otel-agent sets), so operators are not locked into zstd.
//  2. Wire encoding: the Content-Encoding each algorithm emits — the value
//     fakeintake/the intake observe and the e2e utils.TestCompression asserts —
//     is pinned here. Note zlib ships as "deflate" (not "zlib") and none ships
//     as "identity"; those non-obvious mappings are the main regression risk.
//
// Metrics (serializer_compressor_kind, shared by series/sketches/host-metadata)
// support zstd/zlib/gzip/none. Logs (logs_config.compression_kind) support
// zstd/gzip, used verbatim as the Content-Encoding. Traces are compile-time
// (the fx-zstd/fx-gzip module in command.go), not a runtime config key, so they
// are out of scope for this config-level test; the OTLP integration test proves
// traces actually ship valid zstd on the wire.
func TestDDOTSupportedCompressors(t *testing.T) {
	// Pin the on-the-wire Content-Encoding for each algorithm. These are the
	// exact strings e2e/intake see; changing one would silently break the e2e
	// compression assertions, so guard them here.
	assert.Equal(t, "zstd", compression.ZstdEncoding, "zstd must ship Content-Encoding zstd")
	assert.Equal(t, "deflate", compression.ZlibEncoding, "zlib must ship Content-Encoding deflate (not \"zlib\")")
	assert.Equal(t, "gzip", compression.GzipEncoding, "gzip must ship Content-Encoding gzip")

	// metrics: every supported serializer_compressor_kind must be selectable,
	// and each maps to the wire encoding recorded above (none -> "identity").
	metrics := []struct{ kind, wantEncoding string }{
		{compression.ZstdKind, compression.ZstdEncoding},
		{compression.ZlibKind, compression.ZlibEncoding},
		{compression.GzipKind, compression.GzipEncoding},
		{compression.NoneKind, "identity"},
	}
	for _, tc := range metrics {
		t.Run("metrics/"+tc.kind, func(t *testing.T) {
			configmock.New(t)
			t.Setenv("DD_SERIALIZER_COMPRESSOR_KIND", tc.kind)
			c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
			require.NoError(t, err)
			assert.Equalf(t, tc.kind, c.GetString("serializer_compressor_kind"),
				"operators must be able to select %q for metrics via DD_SERIALIZER_COMPRESSOR_KIND (wire encoding %q)", tc.kind, tc.wantEncoding)
		})
	}

	// logs: only zstd and gzip are supported; the pipeline uses the kind
	// verbatim as the Content-Encoding, so the kind is the wire value.
	logs := []string{compression.ZstdKind, compression.GzipKind}
	for _, kind := range logs {
		t.Run("logs/"+kind, func(t *testing.T) {
			configmock.New(t)
			t.Setenv("DD_LOGS_CONFIG_COMPRESSION_KIND", kind)
			c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
			require.NoError(t, err)
			assert.Equalf(t, kind, c.GetString("logs_config.compression_kind"),
				"operators must be able to select %q for logs via DD_LOGS_CONFIG_COMPRESSION_KIND", kind)
			assert.Truef(t, c.IsConfigured("logs_config.compression_kind"),
				"overriding logs_config.compression_kind to %q must keep it IsConfigured() so the additional_endpoints gzip fallback stays bypassed", kind)
		})
	}
}
