// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"errors"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/dd-otel-host-profiler/config"
	"github.com/DataDog/dd-otel-host-profiler/reporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
	ebpfconfig "go.opentelemetry.io/ebpf-profiler/collector/config"
	"go.opentelemetry.io/ebpf-profiler/tracer/types"
)

// ReporterConfig is the configuration for the reporter.
type ReporterConfig struct {
	CollectContext bool `mapstructure:"collect_context"`
}

// Config is the configuration for the profiles receiver.
type Config struct {
	EbpfCollectorConfig  *ebpfconfig.Config            `mapstructure:"ebpf_collector"`
	SymbolUploader       reporter.SymbolUploaderConfig `mapstructure:"symbol_uploader"`
	ReporterConfig       ReporterConfig                `mapstructure:"reporter"`
	EnableSplitByService bool                          `mapstructure:"enable_split_by_service"`
}

var _ xconfmap.Validator = (*Config)(nil)

func errSymbolEndpointsRequired() error {
	return errors.New("symbol_endpoints is required")
}
func errSymbolEndpointsSiteRequired() error {
	return errors.New("symbol_endpoints.site is required")
}
func errSymbolEndpointsAPIKeyRequired() error {
	return errors.New("symbol_endpoints.api_key is required")
}
func errSymbolEndpointsAppKeyRequired() error {
	return errors.New("symbol_endpoints.app_key is required")
}

// Validate validates the config.
// This is automatically called by the config parser as it implements the xconfmap.Validator interface.
func (c *Config) Validate() error {
	if err := c.EbpfCollectorConfig.Validate(); err != nil {
		return err
	}
	if c.ReporterConfig.CollectContext {
		includeTracers, err := types.Parse(c.EbpfCollectorConfig.Tracers)
		if err != nil {
			return err
		}
		includeTracers.Enable(types.Labels)
		c.EbpfCollectorConfig.Tracers = includeTracers.String()
	}
	if c.EnableSplitByService {
		includeEnvVars := reporter.ServiceNameEnvVars
		if c.EbpfCollectorConfig.IncludeEnvVars != "" {
			includeEnvVars = append(includeEnvVars, c.EbpfCollectorConfig.IncludeEnvVars)
		}
		c.EbpfCollectorConfig.IncludeEnvVars = strings.Join(includeEnvVars, ",")
	}

	if c.SymbolUploader.Enabled {
		if len(c.SymbolUploader.SymbolEndpoints) == 0 {
			return errSymbolEndpointsRequired()
		}
		for _, endpoint := range c.SymbolUploader.SymbolEndpoints {
			if endpoint.Site == "" {
				return errSymbolEndpointsSiteRequired()
			}
			if endpoint.APIKey == "" {
				return errSymbolEndpointsAPIKeyRequired()
			}
			if endpoint.AppKey == "" {
				return errSymbolEndpointsAppKeyRequired()
			}
		}
	}
	return nil
}

// This is the default config for the profiles receiver
func defaultConfig() component.Config {
	cfg := ebpfcollector.NewFactory().CreateDefaultConfig().(*ebpfconfig.Config)
	cfg.Tracers = getDefaultTracersString()

	return Config{
		EbpfCollectorConfig: cfg,
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
			Version:                        version.AgentVersion,
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
