// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

import (
	"context"
	"time"

	ddprofilingextensiondef "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var _ extension.Extension = (*ddExtension)(nil)
var _ component.Config = (*Config)(nil)

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg *Config // Extension configuration.
}

// NewExtension creates a new instance of the extension.
func NewExtension(cfg *Config) (ddprofilingextensiondef.Component, error) {
	return &ddExtension{
		cfg: cfg,
	}, nil
}

func (e *ddExtension) Start(ctx context.Context, _ component.Host) error {
	profilerOptions := []profiler.Option{
		profiler.WithService("opentelemetry-collector"),
		profiler.WithEnv("opentelemetry-collector"),
		profiler.WithPeriod(10 * time.Second),
		profiler.WithProfileTypes(
			profiler.CPUProfile,
			profiler.HeapProfile,
			// The profiles below are disabled by default to keep overhead
			// low, but can be enabled as needed.

			// profiler.BlockProfile,
			// profiler.MutexProfile,
			// profiler.GoroutineProfile,
		),
	}

	if string(e.cfg.API.Key) != "" {
		profilerOptions = append(profilerOptions,
			profiler.WithAgentlessUpload(),
			profiler.WithAPIKey(string(e.cfg.API.Key)),
		)
	}

	if string(e.cfg.API.Site) != "" {
		profilerOptions = append(profilerOptions, profiler.WithSite(string(e.cfg.API.Site)))
	}

	err := profiler.Start(
		profilerOptions...,
	)

	if err != nil {
		return err
	}
	
	return nil
}

func (e *ddExtension) Shutdown(ctx context.Context) error {
	profiler.Stop()
	return nil
}
