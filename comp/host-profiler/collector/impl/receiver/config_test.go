// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestReporterInterval(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)
	require.Equal(t, 60*time.Second, cfg.EbpfCollectorConfig.ReporterInterval)
}

func TestTracers(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)

	require.Greater(t, len(cfg.EbpfCollectorConfig.Tracers), 0)
	require.NotContains(t, cfg.EbpfCollectorConfig.Tracers, "go")
	require.NotContains(t, cfg.EbpfCollectorConfig.Tracers, "labels")

	cfg.CollectContext = false
	cfg.SymbolUploader.Enabled = false
	require.NoError(t, cfg.Validate())
	require.NotContains(t, cfg.EbpfCollectorConfig.Tracers, "labels")

	cfg.CollectContext = true
	require.NoError(t, cfg.Validate())
	require.Contains(t, cfg.EbpfCollectorConfig.Tracers, "labels")
}

func TestServiceNameEnvVars(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)
	cfg.SymbolUploader.Enabled = false
	require.Equal(t, "", cfg.EbpfCollectorConfig.IncludeEnvVars)

	require.NoError(t, cfg.Validate())
	require.Equal(t, strings.Join(serviceNameEnvVars, ","), cfg.EbpfCollectorConfig.IncludeEnvVars)
}

func TestSymbolUploader(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)
	cfg.SymbolUploader.Enabled = false
	require.NoError(t, cfg.Validate())

	cfg.SymbolUploader.Enabled = true
	require.Error(t, errSymbolEndpointsRequired(), cfg.Validate())

	cfg.SymbolUploader.SymbolEndpoints = []symboluploader.SymbolEndpoint{{}}
	require.Error(t, errSymbolEndpointsSiteRequired(), cfg.Validate())
	cfg.SymbolUploader.SymbolEndpoints[0].Site = "datadoghq.com"
	require.Error(t, errSymbolEndpointsAPIKeyRequired(), cfg.Validate())
	cfg.SymbolUploader.SymbolEndpoints[0].APIKey = "1234567890"
	require.NoError(t, cfg.Validate())
}

func TestFlatConfigParsingIsAccepted(t *testing.T) {
	input := map[string]any{
		"reporter_interval": "30s",
		"tracers":           "native",
		"symbol_uploader": map[string]any{
			"enabled": false,
		},
	}

	cfg := defaultConfig().(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)

	require.Equal(t, 30*time.Second, cfg.EbpfCollectorConfig.ReporterInterval)
	require.Equal(t, "native", cfg.EbpfCollectorConfig.Tracers)
	require.False(t, cfg.SymbolUploader.Enabled)

	require.NoError(t, cfg.Validate())
	require.Equal(t, strings.Join(serviceNameEnvVars, ","), cfg.EbpfCollectorConfig.IncludeEnvVars)
}

func TestNestedConfigIsRejected(t *testing.T) {
	input := map[string]any{
		"ebpf_collector": map[string]any{
			"reporter_interval": "30s",
		},
		"symbol_uploader": map[string]any{
			"enabled": false,
		},
	}

	cfg := defaultConfig().(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ebpf_collector")
}
