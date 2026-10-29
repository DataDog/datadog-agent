// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
	ebpfconfig "go.opentelemetry.io/ebpf-profiler/collector/config"
	"go.opentelemetry.io/ebpf-profiler/interpreter"
	"go.opentelemetry.io/ebpf-profiler/interpreter/interpreterconfig"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader"
)

// 19 Hz avoids lockstep sampling with periodic activity.
const defaultSamplesPerSecond = 19

const minReporterInterval = 60 * time.Second

// Config is the configuration for the profiles receiver.
type Config struct {
	EbpfCollectorConfig *ebpfconfig.Config                  `mapstructure:",squash"`
	SymbolUploader      symboluploader.SymbolUploaderConfig `mapstructure:"symbol_uploader"`
}

// defaultEnvVars lists environment variables read from profiled processes to populate
// unified service tags (service, env, version) and the crashtracking monitor process marker
// in OTLP resource attributes.
// The order indicates which environment variable takes precedence.
var defaultEnvVars = []string{
	"DD_SERVICE", "OTEL_SERVICE_NAME", "DD_ENV", "DD_VERSION",
	"OTEL_RESOURCE_ATTRIBUTES", // TODO: remove once we rebase our fork on upstream.
	"DD_CRASHTRACKING_IS_MONITOR_PROCESS",
}

var _ confmap.Validator = (*Config)(nil)

func errSymbolEndpointsRequired() error {
	return errors.New("symbol_endpoints is required")
}
func errSymbolEndpointsSiteRequired() error {
	return errors.New("symbol_endpoints.site is required")
}
func errSymbolEndpointsAPIKeyRequired() error {
	return errors.New("symbol_endpoints.api_key is required")
}
func errSymbolUploaderReportIntervalInvalid() error {
	return fmt.Errorf("symbol_uploader.report.interval must be positive and at most %s", symboluploader.MaxReportInterval)
}
func errSymbolUploaderReportMaxInvalid() error {
	return fmt.Errorf("symbol_uploader.report.max_paths_per_report must be positive and at most %d", symboluploader.MaxReportMaxPathsPerReport)
}

// Validate validates the config.
// This is automatically called by the config parser as it implements the confmap.Validator interface.
func (c *Config) Validate() error {
	// Normalize before upstream validation so even non-positive overrides use the supported frequency.
	if c.EbpfCollectorConfig.SamplesPerSecond != defaultSamplesPerSecond {
		slog.Warn("Datadog Host Profiler will use its supported sampling frequency instead of the configured samples_per_second value. "+
			"If your use case requires a different sampling frequency, please contact Datadog Support at https://docs.datadoghq.com/help/.",
			slog.Int("configured_samples_per_second", c.EbpfCollectorConfig.SamplesPerSecond),
			slog.Int("effective_samples_per_second", defaultSamplesPerSecond))
		c.EbpfCollectorConfig.SamplesPerSecond = defaultSamplesPerSecond
	}
	if c.EbpfCollectorConfig.ReporterInterval < minReporterInterval {
		return fmt.Errorf("reporter_interval must be at least %s", minReporterInterval)
	}
	if err := c.EbpfCollectorConfig.Validate(); err != nil {
		return err
	}
	if len(c.EbpfCollectorConfig.Probes) > 0 {
		slog.Warn("Datadog Host Profiler probe extensions are not supported at the time of this release; profiles collected by these probes may be interpreted incorrectly.",
			slog.Any("probes", c.EbpfCollectorConfig.Probes))
	}
	includeEnvVars := append([]string{}, defaultEnvVars...)
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
		if c.SymbolUploader.Report.Interval <= 0 || c.SymbolUploader.Report.Interval > symboluploader.MaxReportIntervalSeconds {
			return errSymbolUploaderReportIntervalInvalid()
		}
		if c.SymbolUploader.Report.MaxPathsPerReport <= 0 || c.SymbolUploader.Report.MaxPathsPerReport > symboluploader.MaxReportMaxPathsPerReport {
			return errSymbolUploaderReportMaxInvalid()
		}
	}
	return nil
}

// This is the default config for the profiles receiver
func defaultConfig(profilerName string) component.Config {
	cfg := ebpfcollector.NewFactory().CreateDefaultConfig().(*ebpfconfig.Config)
	cfg.Interpreters = defaultInterpretersConfig()
	// 60s batches more samples per report, improving compression and reducing upload bandwidth
	cfg.ReporterInterval = minReporterInterval
	// Default jitter is 20%, which makes sense for 5s intervals (~1s variation).
	// With 60s intervals, 20% would mean ~12s variation, so we reduce to 5% (~3s).
	cfg.ReporterJitter = 0.05
	cfg.SendErrorFrames = false
	cfg.SamplesPerSecond = defaultSamplesPerSecond

	symbolUploaderConfig := symboluploader.DefaultSymbolUploaderConfig(profilerName)
	return Config{
		EbpfCollectorConfig: cfg,
		SymbolUploader:      symbolUploaderConfig,
	}
}

func defaultInterpretersConfig() interpreterconfig.Config {
	cfg := interpreterconfig.AllInterpreters()
	// Disable Go symbolization by default because we do Go symbolization remotely.
	cfg.Go.Symbolization = interpreter.BaseConfig{Disabled: true}
	// Disable Go labels by default; enable them with interpreters.go.labels.disabled: false.
	cfg.Go.Labels = interpreter.BaseConfig{Disabled: true}
	// CRuby has lots of Ruby->native->Ruby transitions which runs out of ebpf tail calls very fast
	// and captures only a small part of the stack. This config fixes this by skipping native frames
	// (and tail calls) for Ruby methods implemented in C. Leaf native frames are still captured.
	// See https://github.com/open-telemetry/opentelemetry-ebpf-profiler/pull/1335
	cfg.Ruby.SkipNativeResume = true
	return cfg
}
