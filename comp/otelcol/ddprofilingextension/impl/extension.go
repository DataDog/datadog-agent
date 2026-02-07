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
	"os"
	"runtime/debug"
	"strings"
	"time"

	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	ddprofilingextensiondef "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"

	"github.com/DataDog/dd-trace-go/v2/profiler"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

var (
	_                    extension.Extension = (*ddExtension)(nil)
	_                    component.Config    = (*Config)(nil)
	defaultEndpoint                          = "7501"
	errAPIKeyMissing                         = errors.New("API key is required for ddprofiling extension")
	additionalTagsHeader                     = "X-Datadog-Additional-Tags"
)

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg            *Config // Extension configuration.
	info           component.BuildInfo
	traceAgent     traceagent.Component
	server         *http.Server
	log            corelog.Component
	sourceProvider source.Provider
}

// NewExtension creates a new instance of the extension.
func NewExtension(cfg *Config, info component.BuildInfo, traceAgent traceagent.Component, log corelog.Component, sourceProvider source.Provider) (ddprofilingextensiondef.Component, error) {
	return &ddExtension{
		cfg:            cfg,
		info:           info,
		traceAgent:     traceAgent,
		log:            log,
		sourceProvider: sourceProvider,
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

type headerTransport struct {
	wrapped http.RoundTripper
	headers map[string]string
}

func (m *headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	for k, v := range m.headers {
		r.Header.Add(k, v)
	}
	return m.wrapped.RoundTrip(r)
}

func (e *ddExtension) startForOCB() error {
	profilerOptions := e.buildProfilerOptions()

	if string(e.cfg.API.Key) == "" {
		return errAPIKeyMissing
	}
	// In dd-trace-go v2, agentless mode is configured via environment variables.
	os.Setenv("DD_PROFILING_AGENTLESS", "true")
	os.Setenv("DD_API_KEY", string(e.cfg.API.Key))
	if string(e.cfg.API.Site) != "" {
		os.Setenv("DD_SITE", string(e.cfg.API.Site))
	}

	source, err := e.sourceProvider.Source(context.Background())
	if err != nil {
		return err
	}

	var tags strings.Builder
	// agent_version is required by profiling backend. Use version of comp/trace/agent/def, and fallback to 7.64.0.
	agentVersion := "7.64.0"
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		for _, module := range buildInfo.Deps {
			if module.Path == "github.com/DataDog/datadog-agent/comp/trace/agent/def" {
				agentVersion = module.Version
			}
		}
	}
	tags.WriteString("agent_version:" + agentVersion)
	tags.WriteString(",source:oss-ddprofilingextension")
	if e.cfg.ProfilerOptions.Env != "" {
		tags.WriteString(",default_env:" + e.cfg.ProfilerOptions.Env)
	}

	if source.Kind == "host" {
		profilerOptions = append(profilerOptions, profiler.WithHostname(source.Identifier))
		tags.WriteString(",host:" + source.Identifier)
	}

	if source.Kind == "task_arn" {
		tags.WriteString(",orchestrator:fargate_ecs,task_arn:" + source.Identifier)
	}

	cl := new(http.Client)
	cl.Transport = &headerTransport{
		wrapped: http.DefaultTransport,
		headers: map[string]string{
			additionalTagsHeader: tags.String(),
		},
	}
	profilerOptions = append(profilerOptions, profiler.WithHTTPClient(cl))

	return profiler.Start(
		profilerOptions...,
	)
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

	if e.traceAgent != nil {
		// stop server
		return e.server.Shutdown(ctx)
	}
	return nil
}
