// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"strings"

	"github.com/DataDog/dd-otel-host-profiler/config"
	"github.com/DataDog/dd-otel-host-profiler/reporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
	"go.opentelemetry.io/ebpf-profiler/tracer/types"
)

// ReporterConfig is the configuration for the reporter.
type ReporterConfig struct {
	CollectContext bool `mapstructure:"collect_context"`
}

// Config is the configuration for the profiles receiver.
type Config struct {
	Ebpfcollector        *ebpfcollector.Config         `mapstructure:"ebpfcollector"`
	SymbolUploader       reporter.SymbolUploaderConfig `mapstructure:"symbol_uploader"`
	ReporterConfig       ReporterConfig                `mapstructure:"reporter"`
	EnableSplitByService bool                          `mapstructure:"enable_split_by_service"`
}

var _ xconfmap.Validator = (*Config)(nil)

// Validate validates the config.
// This is automatically called by the config parser as it implements the xconfmap.Validator interface.
func (c *Config) Validate() error {
	if c.ReporterConfig.CollectContext {
		includeTracers, err := types.Parse(c.Ebpfcollector.Tracers)
		if err != nil {
			return err
		}
		includeTracers.Enable(types.Labels)
		c.Ebpfcollector.Tracers = includeTracers.String()
	}
	if c.EnableSplitByService {
		includeEnvVars := reporter.ServiceNameEnvVars
		if c.Ebpfcollector.IncludeEnvVars != "" {
			includeEnvVars = append(includeEnvVars, c.Ebpfcollector.IncludeEnvVars)
		}
		c.Ebpfcollector.IncludeEnvVars = strings.Join(includeEnvVars, ",")
	}

	return nil
}

// This is the default config for the profiles receiver
func defaultConfig() component.Config {
	cfg := ebpfcollector.NewFactory().CreateDefaultConfig().(*ebpfcollector.Config)
	cfg.Tracers = getDefaultTracersString()

	return Config{
		Ebpfcollector: cfg,
		SymbolUploader: reporter.SymbolUploaderConfig{
			SymbolUploaderOptions: reporter.SymbolUploaderOptions{
				Enabled:              config.DefaultUploadSymbols,
				UploadDynamicSymbols: config.DefaultUploadDynamicSymbols,
				UploadGoPCLnTab:      config.DefaultUploadGoPCLnTab,
				UseHTTP2:             config.DefaultUploadSymbolsHTTP2,
				SymbolQueryInterval:  config.DefaultSymbolQueryInterval,
				DryRun:               config.DefaultUploadSymbolsDryRun,
				SymbolEndpoints:      nil,
			},
			Version:                        "0.0.0",
			DisableDebugSectionCompression: false,
		},

		EnableSplitByService: config.DefaultSplitByService,
		ReporterConfig: ReporterConfig{
			CollectContext: config.DefaultCollectContext,
		},
	}
}

func getDefaultTracersString() string {
	tracers := types.AllTracers()

	// Disable Go interpreter by default because we are doing Go symbolization remotely.
	tracers.Disable(types.GoTracer)

	// Disable Labels by default. It will be enabled if ReporterConfig.CollectContext is true.
	tracers.Disable(types.Labels)

	return tracers.String()
}
