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

func TestReporterIntervalValidation(t *testing.T) {
	for _, interval := range []time.Duration{0, 60*time.Second - time.Nanosecond, 60 * time.Second, 2 * time.Minute} {
		t.Run(interval.String(), func(t *testing.T) {
			cfg := defaultConfig(profilerName).(Config)
			cfg.EbpfCollectorConfig.ReporterInterval = interval
			cfg.SymbolUploader.Enabled = false

			err := cfg.Validate()
			if interval < 60*time.Second {
				require.ErrorContains(t, err, "reporter_interval must be at least 1m0s")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSendErrorFramesDefault(t *testing.T) {
	cfg := defaultConfig(profilerName).(Config)
	require.False(t, cfg.EbpfCollectorConfig.SendErrorFrames)
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
	require.Contains(t, strings.Split(cfg.EbpfCollectorConfig.IncludeEnvVars, ","), "DD_CRASHTRACKING_IS_MONITOR_PROCESS")
}

func TestAdditionalEnvVars(t *testing.T) {
	for _, envVars := range []string{
		"CUSTOM_ENV_VAR",
		"CUSTOM_ENV_VAR,ANOTHER_ENV_VAR",
	} {
		t.Run(envVars, func(t *testing.T) {
			cfg := defaultConfig(profilerName).(Config)
			err := confmap.NewFromStringMap(map[string]any{
				"include_env_vars": envVars,
				"symbol_uploader":  map[string]any{"enabled": false},
			}).Unmarshal(&cfg)
			require.NoError(t, err)
			require.NoError(t, cfg.Validate())

			require.Equal(t, strings.Join(defaultEnvVars, ",")+","+envVars, cfg.EbpfCollectorConfig.IncludeEnvVars)
		})
	}
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
		"reporter_interval": "120s",
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

	require.Equal(t, 120*time.Second, cfg.EbpfCollectorConfig.ReporterInterval)
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

func TestSymbolUploaderReportDefaults(t *testing.T) {
	input := map[string]any{
		"symbol_uploader": map[string]any{
			"enabled": true,
			"symbol_endpoints": []any{
				map[string]any{"site": "datadoghq.com", "api_key": "test-key"},
			},
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	require.Equal(t, symboluploader.DefaultReportIntervalSeconds, cfg.SymbolUploader.Report.Interval)
	require.Equal(t, symboluploader.DefaultReportMaxPathsPerReport, cfg.SymbolUploader.Report.MaxPathsPerReport)
}

func TestSymbolUploaderReportOverride(t *testing.T) {
	input := map[string]any{
		"symbol_uploader": map[string]any{
			"enabled": true,
			"symbol_endpoints": []any{
				map[string]any{"site": "datadoghq.com", "api_key": "test-key"},
			},
			"report": map[string]any{
				"interval":             600,
				"max_paths_per_report": 42,
			},
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	require.Equal(t, 600, cfg.SymbolUploader.Report.Interval)
	require.Equal(t, 42, cfg.SymbolUploader.Report.MaxPathsPerReport)
}

func TestSymbolUploaderReportInvalidIsRejected(t *testing.T) {
	input := map[string]any{
		"symbol_uploader": map[string]any{
			"enabled": true,
			"symbol_endpoints": []any{
				map[string]any{"site": "datadoghq.com", "api_key": "test-key"},
			},
			"report": map[string]any{
				"interval": "abc",
			},
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.ErrorContains(t, err, "'symbol_uploader.report.interval' expected type 'int', got unconvertible type 'string'")
}

func TestSymbolUploaderReportNonPositiveIsRejected(t *testing.T) {
	input := map[string]any{
		"symbol_uploader": map[string]any{
			"enabled": true,
			"symbol_endpoints": []any{
				map[string]any{"site": "datadoghq.com", "api_key": "test-key"},
			},
			"report": map[string]any{
				"max_paths_per_report": 0,
			},
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)

	err = cfg.Validate()
	require.ErrorContains(t, err, errSymbolUploaderReportMaxInvalid().Error())
}

func TestSymbolUploaderReportIntervalAboveCeilingIsRejected(t *testing.T) {
	input := map[string]any{
		"symbol_uploader": map[string]any{
			"enabled": true,
			"symbol_endpoints": []any{
				map[string]any{"site": "datadoghq.com", "api_key": "test-key"},
			},
			"report": map[string]any{
				"interval": symboluploader.MaxReportIntervalSeconds + 1,
			},
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)

	err = cfg.Validate()
	require.ErrorContains(t, err, errSymbolUploaderReportIntervalInvalid().Error())
}

func TestSymbolUploaderReportMaxAboveCeilingIsRejected(t *testing.T) {
	input := map[string]any{
		"symbol_uploader": map[string]any{
			"enabled": true,
			"symbol_endpoints": []any{
				map[string]any{"site": "datadoghq.com", "api_key": "test-key"},
			},
			"report": map[string]any{
				"max_paths_per_report": symboluploader.MaxReportMaxPathsPerReport + 1,
			},
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)

	err = cfg.Validate()
	require.ErrorContains(t, err, errSymbolUploaderReportMaxInvalid().Error())
}

func TestSymbolUploaderReportDisabledSkipsValidation(t *testing.T) {
	input := map[string]any{
		"symbol_uploader": map[string]any{
			"enabled": false,
			"symbol_endpoints": []any{
				map[string]any{"site": "datadoghq.com", "api_key": "test-key"},
			},
			"report": map[string]any{
				"interval":             0,
				"max_paths_per_report": 0,
			},
		},
	}

	cfg := defaultConfig(profilerName).(Config)
	err := confmap.NewFromStringMap(input).Unmarshal(&cfg)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	require.Equal(t, 0, cfg.SymbolUploader.Report.Interval)
	require.Equal(t, 0, cfg.SymbolUploader.Report.MaxPathsPerReport)
}
