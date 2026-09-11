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
	"runtime"
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
	// ddUnixSocketEnvVar mirrors the <component>.internal_profiling.unix_socket
	// setting every other agent process exposes. It is read as an env var, not
	// only as a config key, so that a collector config carrying it still parses
	// on agent builds that predate unix_socket -- the collector rejects unknown
	// config keys outright, which would otherwise make this un-A/B-able.
	ddUnixSocketEnvVar = "DD_OTELCOLLECTOR_INTERNAL_PROFILING_UNIX_SOCKET"
)

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg        *Config // Extension configuration.
	info       component.BuildInfo
	traceAgent traceagent.Component
	server     *http.Server
	// log is nil when the extension is built by NewFactory (standalone mode),
	// which is given no agent components. Every use must be nil-checked.
	log       corelog.Component
	agentMode bool
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
	profilerOptions := e.buildProfilerOptions()

	// A unix socket is a complete substitute for the local forwarding server:
	// profiles go straight to the trace agent listening on the socket, so there
	// is nothing left for the server to forward. Skip starting it rather than
	// leaving an idle listener on port 7501.
	if socket := e.unixSocket(); socket != "" {
		if e.log != nil {
			e.log.Info("DD Profiling Extension sending profiles over unix socket: " + socket)
		}
		return profiler.Start(append(profilerOptions, profiler.WithUDS(socket))...)
	}

	// start server that handles profiles
	err := e.newServer()
	if err != nil {
		return err
	}
	go e.startServer(host)

	// agent
	profilerOptions = append(profilerOptions, profiler.WithAgentAddr("localhost:"+e.endpoint()))

	return profiler.Start(
		profilerOptions...,
	)
}

func (e *ddExtension) startForStandalone() error {
	profilerOptions := e.buildProfilerOptions()
	// A socket and a TCP address are two ways of naming the same trace agent, so
	// only one can apply. The socket wins: it is the more specific of the two.
	if socket := e.unixSocket(); socket != "" {
		profilerOptions = append(profilerOptions, profiler.WithUDS(socket))
	} else if e.cfg.AgentAddr != "" {
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

// unixSocket returns the unix socket profiles should be sent to, preferring the
// config key over the environment.
//
// It also returns "" when a socket is configured on a platform that has no use
// for one. Callers then take the address-based path they would have taken had
// the setting been absent, so a socket carried in a config or an environment
// shared across a mixed fleet degrades to HTTP on Windows rather than pointing
// the profiler at a path nothing serves.
func (e *ddExtension) unixSocket() string {
	socket, source := e.cfg.UnixSocket, "the unix_socket setting"
	if socket == "" {
		if fromEnv, ok := nonBlankEnv(ddUnixSocketEnvVar); ok {
			socket, source = fromEnv, ddUnixSocketEnvVar
		}
	}
	if socket == "" {
		return ""
	}
	if !hasUnixSocketSupport() {
		if e.log != nil {
			e.log.Warn("DD Profiling Extension ignoring " + source + ": no trace agent profiling socket is " +
				"available on " + runtime.GOOS + ", falling back to sending profiles over HTTP")
		}
		return ""
	}
	return socket
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
