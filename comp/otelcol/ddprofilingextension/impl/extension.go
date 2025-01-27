// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

import (
	"context"
	"errors"
	"net/http"
	"time"

	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	ddprofilingextensiondef "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var (
	_                extension.Extension = (*ddExtension)(nil)
	_                component.Config    = (*Config)(nil)
	defaultEndpoint                      = "7501"
	errApiKeyMissing error               = errors.New("API key is required for ddprofiling extension")
)

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg        *Config // Extension configuration.
	info       component.BuildInfo
	traceAgent traceagent.Component
	server     *http.Server
	log        corelog.Component
}

// NewExtension creates a new instance of the extension.
func NewExtension(cfg *Config, info component.BuildInfo, traceAgent traceagent.Component, log corelog.Component) (ddprofilingextensiondef.Component, error) {
	return &ddExtension{
		cfg:        cfg,
		info:       info,
		traceAgent: traceAgent,
		log:        log,
	}, nil
}

func (e *ddExtension) Start(_ context.Context, host component.Host) error {
	// OTEL AGENT
	if e.traceAgent != nil {
		return e.startForOTelAgent(host)
	}
	// OCB
	return e.startForOCB()
}

func (e *ddExtension) startForOTelAgent(host component.Host) error {
	// start server that handles profiles
	e.newServer()
	go e.startServer(host)

	profilerOptions := e.buildProfilerOptions()

	// agent
	if e.cfg.Endpoint != "" {
		profilerOptions = append(profilerOptions, profiler.WithAgentAddr("localhost:"+e.cfg.Endpoint))
	} else {
		profilerOptions = append(profilerOptions, profiler.WithAgentAddr("localhost:"+defaultEndpoint))
	}

	return profiler.Start(
		profilerOptions...,
	)
}

func (e *ddExtension) startForOCB() error {
	profilerOptions := e.buildProfilerOptions()

	if string(e.cfg.API.Key) == "" {
		return errApiKeyMissing
	}
	// agentless
	profilerOptions = append(profilerOptions,
		profiler.WithAgentlessUpload(),
		profiler.WithAPIKey(string(e.cfg.API.Key)),
	)

	// todo(mackjmr): add datadogexporter hostmetadata provider to retrieve hostname, and
	// pass it to profiler via profiler.WithHostname(). This requires a refactor in contrib,
	// as the logic lives within an internal package.
	if string(e.cfg.API.Site) != "" {
		profilerOptions = append(profilerOptions, profiler.WithSite(string(e.cfg.API.Site)))
	}

	return profiler.Start(
		profilerOptions...,
	)
}

func (e *ddExtension) buildProfilerOptions() []profiler.Option {
	profilerOptions := []profiler.Option{
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

	if e.cfg.ProfilerOptions.Service != "" {
		profilerOptions = append(profilerOptions, profiler.WithService(e.cfg.ProfilerOptions.Service))
	} else {
		profilerOptions = append(profilerOptions, profiler.WithService(e.info.Command))
	}

	if e.cfg.ProfilerOptions.Version != "" {
		profilerOptions = append(profilerOptions, profiler.WithVersion(e.cfg.ProfilerOptions.Version))
	} else {
		profilerOptions = append(profilerOptions, profiler.WithVersion(e.info.Version))
	}

	if e.cfg.ProfilerOptions.Env != "" {
		profilerOptions = append(profilerOptions, profiler.WithEnv(e.cfg.ProfilerOptions.Env))
	}

	if e.cfg.ProfilerOptions.Period > 0 {
		profilerOptions = append(profilerOptions, profiler.WithPeriod(time.Duration(e.cfg.ProfilerOptions.Period)*time.Second))
	}

	return profilerOptions
}

func (e *ddExtension) Shutdown(ctx context.Context) error {
	// stop profiler
	profiler.Stop()
	// stop server

	if e.traceAgent != nil {
		return e.server.Shutdown(ctx)
	}
	return nil
}
