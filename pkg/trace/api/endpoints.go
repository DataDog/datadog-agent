// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// Endpoint specifies an API endpoint definition.
type Endpoint struct {
	// Pattern specifies the API pattern, as registered by the HTTP handler.
	Pattern string

	// Handler specifies the http.Handler for this endpoint.
	Handler func(*HTTPReceiver) http.Handler

	// Hidden reports whether this endpoint should be hidden in the /info
	// discovery endpoint.
	Hidden bool

	// TimeoutOverride lets you specify a timeout for this endpoint that will be used
	// instead of the default one from conf.ReceiverTimeout
	TimeoutOverride func(conf *config.AgentConfig) time.Duration

	// IsEnabled specifies a function which reports whether this endpoint should be enabled
	// based on the given config conf.
	IsEnabled func(conf *config.AgentConfig) bool
}

// AttachEndpoint attaches an additional endpoint to the trace-agent. It is not thread-safe
// and should be called before (pkg/trace.*Agent).Run or (*HTTPReceiver).Start. In other
// words, endpoint setup must be final before the agent or HTTP receiver starts.
func AttachEndpoint(e Endpoint) { endpoints = append(endpoints, e) }

// endpoints specifies the list of endpoints registered for the trace-agent API.
var endpoints = []Endpoint{
	{
		Pattern: "/spans",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleTraces) },
		Hidden:  true,
	},
	{
		Pattern: "/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleServices) },
		Hidden:  true,
	},
	{
		Pattern: "/v0.1/spans",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleTraces) },
		Hidden:  true,
	},
	{
		Pattern: "/v0.1/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleServices) },
		Hidden:  true,
	},
	{
		Pattern: "/v0.2/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v02, r.handleTraces) },
		Hidden:  true,
	},
	{
		Pattern: "/v0.2/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v02, r.handleServices) },
		Hidden:  true,
	},
	{
		Pattern: "/v0.3/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v03, r.handleTraces) },
	},
	{
		Pattern: "/v0.3/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v03, r.handleServices) },
	},
	{
		Pattern: "/v0.4/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v04, r.handleTraces) },
	},
	{
		Pattern: "/v0.4/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v04, r.handleServices) },
	},
	{
		Pattern: "/v0.5/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v05, r.handleTraces) },
	},
	{
		Pattern: "/v0.7/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(V07, r.handleTraces) },
	},
	{
		Pattern:   "/v1.0/traces",
		Handler:   func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(V10, r.handleTraces) },
		IsEnabled: func(cfg *config.AgentConfig) bool { return cfg.EnableV1TraceEndpoint },
	},
	{
		Pattern:         "/profiling/v1/input",
		Handler:         func(r *HTTPReceiver) http.Handler { return r.profileProxyHandler() },
		TimeoutOverride: getConfiguredProfilingRequestTimeoutDuration,
	},
	{
		Pattern: "/telemetry/proxy/",
		Handler: func(r *HTTPReceiver) http.Handler {
			return http.StripPrefix("/telemetry/proxy", r.telemetryForwarderHandler())
		},
		IsEnabled: func(cfg *config.AgentConfig) bool { return cfg.TelemetryConfig.Enabled },
	},
	{
		Pattern: "/v0.6/stats",
		Handler: func(r *HTTPReceiver) http.Handler { return http.HandlerFunc(r.handleStats) },
	},
	{
		Pattern: "/v0.1/pipeline_stats",
		Handler: func(r *HTTPReceiver) http.Handler { return r.pipelineStatsProxyHandler() },
	},
	{
		Pattern: "/openlineage/api/v1/lineage",
		Handler: func(r *HTTPReceiver) http.Handler { return r.openLineageProxyHandler() },
	},
	{
		Pattern:         "/evp_proxy/v1/",
		Handler:         func(r *HTTPReceiver) http.Handler { return r.evpProxyHandler(1) },
		TimeoutOverride: getConfiguredEVPRequestTimeoutDuration,
	},
	{
		Pattern:         "/evp_proxy/v2/",
		Handler:         func(r *HTTPReceiver) http.Handler { return r.evpProxyHandler(2) },
		TimeoutOverride: getConfiguredEVPRequestTimeoutDuration,
	},
	{
		Pattern:         "/evp_proxy/v3/",
		Handler:         func(r *HTTPReceiver) http.Handler { return r.evpProxyHandler(3) },
		TimeoutOverride: getConfiguredEVPRequestTimeoutDuration,
	},
	{
		Pattern:         "/evp_proxy/v4/",
		Handler:         func(r *HTTPReceiver) http.Handler { return r.evpProxyHandler(4) },
		TimeoutOverride: getConfiguredEVPRequestTimeoutDuration,
	},
	{
		Pattern: "/debugger/v1/input",
		Handler: func(r *HTTPReceiver) http.Handler { return r.debuggerLogsProxyHandler() },
	},
	{
		Pattern: "/debugger/v1/diagnostics",
		Handler: func(r *HTTPReceiver) http.Handler { return r.debuggerDiagnosticsProxyHandler() },
	},
	{
		Pattern: "/debugger/v2/input",
		Handler: func(r *HTTPReceiver) http.Handler { return r.debuggerV2IntakeProxyHandler() },
	},
	{
		Pattern: "/symdb/v1/input",
		Handler: func(r *HTTPReceiver) http.Handler { return r.symDBProxyHandler() },
	},
	{
		Pattern: "/dogstatsd/v1/proxy", // deprecated
		Handler: func(r *HTTPReceiver) http.Handler { return r.dogstatsdProxyHandler() },
	},
	{
		Pattern: "/dogstatsd/v2/proxy",
		Handler: func(r *HTTPReceiver) http.Handler { return r.dogstatsdProxyHandler() },
	},
	{
		Pattern: "/tracer_flare/v1",
		Handler: func(r *HTTPReceiver) http.Handler { return r.tracerFlareHandler() },
	},
}
