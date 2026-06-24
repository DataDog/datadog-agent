// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	ddprofilingextensiondef "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"

	"github.com/DataDog/dd-trace-go/v2/profiler"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

var (
	_               extension.Extension = (*ddExtension)(nil)
	_               component.Config    = (*Config)(nil)
	defaultEndpoint                     = "7501"
)

const (
	ddServiceEnvVar = "DD_SERVICE"
	ddEnvEnvVar     = "DD_ENV"
	ddVersionEnvVar = "DD_VERSION"
)

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg        *Config // Extension configuration.
	info       component.BuildInfo
	traceAgent traceagent.Component
	server     *http.Server
	log        corelog.Component
	agentMode  bool
}

// NewComponent creates a new instance of the extension.
func NewComponent(cfg *Config, info component.BuildInfo, traceAgent traceagent.Component, log corelog.Component) (ddprofilingextensiondef.Component, error) {
	return &ddExtension{
		cfg:        cfg,
		info:       info,
		traceAgent: traceAgent,
		log:        log,
		agentMode:  traceAgent != nil,
	}, nil
}

func (e *ddExtension) Start(_ context.Context, host component.Host) error {
	if e.agentMode {
		return e.startForAgent(host)
	}
	return e.startForStandalone()
}

func (e *ddExtension) startForAgent(host component.Host) error {
	// start server that handles profiles
	err := e.newServer()
	if err != nil {
		return err
	}
	go e.startServer(host)

	profilerOptions := e.buildProfilerOptions()

	// agent
	profilerOptions = append(profilerOptions, profiler.WithAgentAddr("localhost:"+e.endpoint()))

	return profiler.Start(
		profilerOptions...,
	)
}

func (e *ddExtension) startForStandalone() error {
	profilerOptions := e.buildProfilerOptions()
	if e.cfg.AgentAddr != "" {
		profilerOptions = append(profilerOptions, profiler.WithAgentAddr(e.cfg.AgentAddr))
	}
	return profiler.Start(profilerOptions...)
}

func (e *ddExtension) buildProfilerOptions() []profiler.Option {
	defaultProfileTypes := []profiler.ProfileType{
		profiler.CPUProfile,
		profiler.HeapProfile,
	}

	profilerOptions := []profiler.Option{}

	for _, profileType := range e.cfg.ProfilerOptions.ProfileTypes {
		if profileType == "blockprofile" {
			defaultProfileTypes = append(defaultProfileTypes, profiler.BlockProfile)
		}
		if profileType == "mutexprofile" {
			defaultProfileTypes = append(defaultProfileTypes, profiler.MutexProfile)
		}
		if profileType == "goroutineprofile" {
			defaultProfileTypes = append(defaultProfileTypes, profiler.GoroutineProfile)
		}
	}
	profilerOptions = append(profilerOptions, profiler.WithProfileTypes(defaultProfileTypes...))

	if e.cfg.ProfilerOptions.Service != "" {
		profilerOptions = append(profilerOptions, profiler.WithService(e.cfg.ProfilerOptions.Service))
	} else if service, ok := nonBlankEnv(ddServiceEnvVar); ok {
		profilerOptions = append(profilerOptions, profiler.WithService(service))
	} else {
		profilerOptions = append(profilerOptions, profiler.WithService(e.info.Command))
	}

	if e.cfg.ProfilerOptions.Version != "" {
		profilerOptions = append(profilerOptions, profiler.WithVersion(e.cfg.ProfilerOptions.Version))
	} else if version, ok := nonBlankEnv(ddVersionEnvVar); ok {
		profilerOptions = append(profilerOptions, profiler.WithVersion(version))
	} else {
		profilerOptions = append(profilerOptions, profiler.WithVersion(e.info.Version))
	}

	if e.cfg.ProfilerOptions.Env != "" {
		profilerOptions = append(profilerOptions, profiler.WithEnv(e.cfg.ProfilerOptions.Env))
	} else if env, ok := nonBlankEnv(ddEnvEnvVar); ok {
		profilerOptions = append(profilerOptions, profiler.WithEnv(env))
	}

	if e.cfg.ProfilerOptions.Period > 0 {
		profilerOptions = append(profilerOptions, profiler.WithPeriod(time.Duration(e.cfg.ProfilerOptions.Period)*time.Second))
	}

	return profilerOptions
}

func nonBlankEnv(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func (e *ddExtension) Shutdown(ctx context.Context) error {
	profiler.Stop()
	if e.server == nil {
		// Standalone mode sends directly to an external trace-agent and does not
		// start the local forwarding server used in bundled mode.
		return nil
	}
	return e.server.Shutdown(ctx)
}
