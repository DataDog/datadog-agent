// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
	ebpfconfig "go.opentelemetry.io/ebpf-profiler/collector/config"
	"go.opentelemetry.io/ebpf-profiler/tracer/types"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader"
)

type MemoryProfilingConfig struct {
	Enabled bool `mapstructure:"enabled"`
	// Mode: "filtered" (only processes with EnvVar=1) or "all"
	Mode                 string `mapstructure:"mode"`
	EnvVar               string `mapstructure:"env_var"`
	AllocationSampleRate uint32 `mapstructure:"allocation_sample_rate"`
}

type Config struct {
	EbpfCollectorConfig *ebpfconfig.Config                  `mapstructure:",squash"`
	SymbolUploader      symboluploader.SymbolUploaderConfig `mapstructure:"symbol_uploader"`
	CollectContext      bool                                `mapstructure:"collect_context"`
	MemoryProfiling     MemoryProfilingConfig               `mapstructure:"memory_profiling"`
}

// defaultEnvVars lists environment variables read from profiled processes to populate
// unified service tags (service, env, version) in OTLP resource attributes.
// The order indicates which environment variable takes precedence.
var defaultEnvVars = []string{"DD_SERVICE", "OTEL_SERVICE_NAME", "DD_ENV", "DD_VERSION"}

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
func errMemoryProfilingInvalidMode() error {
	return errors.New("memory_profiling.mode must be 'filtered' or 'all'")
}
func errMemoryProfilingInvalidSampleRate() error {
	return errors.New("memory_profiling.allocation_sample_rate must be between 0 and 100")
}
func errMemoryProfilingFilteredModeRequiresEnvVar() error {
	return errors.New("memory_profiling.env_var must be set when mode is 'filtered'")
}

// Validate validates the config.
// This is automatically called by the config parser as it implements the xconfmap.Validator interface.
func (c *Config) Validate() error {
	if err := c.EbpfCollectorConfig.Validate(); err != nil {
		return err
	}
	if c.CollectContext {
		includeTracers, err := types.Parse(c.EbpfCollectorConfig.Tracers)
		if err != nil {
			return err
		}
		includeTracers.Enable(types.Labels)
		c.EbpfCollectorConfig.Tracers = includeTracers.String()
	}

	includeEnvVars := append([]string{}, defaultEnvVars...)
	if c.EbpfCollectorConfig.IncludeEnvVars != "" {
		includeEnvVars = append(includeEnvVars, c.EbpfCollectorConfig.IncludeEnvVars)
	}
	c.EbpfCollectorConfig.IncludeEnvVars = strings.Join(includeEnvVars, ",")

	if c.MemoryProfiling.Enabled {
		if c.MemoryProfiling.Mode != "filtered" && c.MemoryProfiling.Mode != "all" {
			return errMemoryProfilingInvalidMode()
		}
		if c.MemoryProfiling.AllocationSampleRate > 100 {
			return errMemoryProfilingInvalidSampleRate()
		}
		if c.MemoryProfiling.Mode == "filtered" && c.MemoryProfiling.EnvVar == "" {
			return errMemoryProfilingFilteredModeRequiresEnvVar()
		}
		if c.MemoryProfiling.EnvVar != "" {
			includeEnvVars = append(includeEnvVars, c.MemoryProfiling.EnvVar)
			c.EbpfCollectorConfig.IncludeEnvVars = strings.Join(includeEnvVars, ",")
		}
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
		}
	}
	return nil
}

// This is the default config for the profiles receiver
func defaultConfig() component.Config {
	cfg := ebpfcollector.NewFactory().CreateDefaultConfig().(*ebpfconfig.Config)
	cfg.Tracers = getDefaultTracersString()
	// 60s batches more samples per report, improving compression and reducing upload bandwidth
	cfg.ReporterInterval = 60 * time.Second
	// Default jitter is 20%, which makes sense for 5s intervals (~1s variation).
	// With 60s intervals, 20% would mean ~12s variation, so we reduce to 5% (~3s).
	cfg.ReporterJitter = 0.05

	symbolUploaderConfig := symboluploader.DefaultSymbolUploaderConfig()
	return Config{
		EbpfCollectorConfig: cfg,
		SymbolUploader:      symbolUploaderConfig,
		CollectContext:      false,
		MemoryProfiling: MemoryProfilingConfig{
			Enabled:              false,
			Mode:                 "filtered",
			EnvVar:               "MEMPROF",
			AllocationSampleRate: 10,
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
