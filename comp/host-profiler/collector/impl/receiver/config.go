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

// Config is the configuration for the profiles receiver.
type Config struct {
	EbpfCollectorConfig *ebpfconfig.Config                  `mapstructure:",squash"`
	SymbolUploader      symboluploader.SymbolUploaderConfig `mapstructure:"symbol_uploader"`
	CollectContext      bool                                `mapstructure:"collect_context"`
}

// ServiceNameEnvVars is the list of environment variables used to determine the service name.
// The order indicates which environment variable takes precedence.
var serviceNameEnvVars = []string{"DD_SERVICE", "OTEL_SERVICE_NAME"}

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

	includeEnvVars := append([]string{}, serviceNameEnvVars...)
	if c.EbpfCollectorConfig.IncludeEnvVars != "" {
		includeEnvVars = append(includeEnvVars, c.EbpfCollectorConfig.IncludeEnvVars)
	}
	c.EbpfCollectorConfig.IncludeEnvVars = strings.Join(includeEnvVars, ",")

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
