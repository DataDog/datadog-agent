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

const profilerName = "host-profiler"

func TestReporterInterval(t *testing.T) {
	config := defaultConfig(profilerName)
	cfg := config.(Config)
	require.Equal(t, 60*time.Second, cfg.EbpfCollectorConfig.ReporterInterval)
}

func TestInterpreters(t *testing.T) {
	config := defaultConfig(profilerName)
	cfg := config.(Config)

	require.True(t, cfg.EbpfCollectorConfig.Interpreters.Go.IsSymbolizationDisabled())
	require.True(t, cfg.EbpfCollectorConfig.Interpreters.Go.IsLabelsDisabled())

	cfg.CollectContext = false
	cfg.SymbolUploader.Enabled = false
	require.NoError(t, cfg.Validate())
	require.True(t, cfg.EbpfCollectorConfig.Interpreters.Go.IsLabelsDisabled())

	cfg.CollectContext = true
	require.NoError(t, cfg.Validate())
	require.False(t, cfg.EbpfCollectorConfig.Interpreters.Go.IsLabelsDisabled())
}

func TestDefaultEnvVars(t *testing.T) {
	config := defaultConfig(profilerName)
	cfg := config.(Config)
	cfg.SymbolUploader.Enabled = false
	require.Equal(t, "", cfg.EbpfCollectorConfig.IncludeEnvVars)

	require.NoError(t, cfg.Validate())
	require.Equal(t, strings.Join(defaultEnvVars, ","), cfg.EbpfCollectorConfig.IncludeEnvVars)
}

func TestSymbolUploader(t *testing.T) {
	config := defaultConfig(profilerName)
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
		"interpreters": map[string]any{
			"go": map[string]any{
				"symbolization": map[string]any{
					"disabled": true,
				},
			},
		},
		"symbol_uploader": map[string]any{
			"enabled": false,
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)

	require.Equal(t, 30*time.Second, cfg.EbpfCollectorConfig.ReporterInterval)
	require.True(t, cfg.EbpfCollectorConfig.Interpreters.Go.IsSymbolizationDisabled())
	require.False(t, cfg.SymbolUploader.Enabled)

	require.NoError(t, cfg.Validate())
	require.Equal(t, strings.Join(defaultEnvVars, ","), cfg.EbpfCollectorConfig.IncludeEnvVars)
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

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ebpf_collector")
}
