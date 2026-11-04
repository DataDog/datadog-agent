// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package devtracing is the DEVELOPMENT-ONLY tracing setup of the Remote
// Queries POC. Delete this package and its call sites (the remote-queries
// bundle and the private-action-runner workflow task executor) with the POC.
//
// While enabled, the runner and the remote-queries bundle emit normal
// dd-trace-go spans to the POC's dedicated development trace-agent, which
// reports to ddstaging, so the Agent's part of a run is visible in the same
// trace as rq, ITS, Delancie, its-agent, and the integrations. This is tooling
// for the standing test harness, never a product feature: a customer's Agent
// reports to the customer's own org. No span ever relies on a default
// trace-agent address — TraceAgentURL is the only endpoint used (fail closed),
// because the workspace's own trace-agent on 127.0.0.1:8126 reports to a
// different org and must never receive these spans.
package devtracing

import (
	"os"
	"sync"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TraceAgentURL is the explicit endpoint of the POC's development trace-agent —
// a dedicated container publishing only 127.0.0.1:8127 and reporting to
// ddstaging. Setting it to the empty string disables development tracing
// entirely: the tracer never starts and no span is created.
const TraceAgentURL = "http://127.0.0.1:8127"

const (
	// Service is the dd-trace-go service name of the Agent's remote-queries spans.
	Service = "datadog-agent-remote-queries"
	// Env is the dd-trace-go env tag of the development spans.
	Env = "staging"
)

// enabled gates the development span paths at runtime. It derives from
// TraceAgentURL; it is a variable only so the unit tests can exercise the
// disabled path — production code never reassigns it.
var enabled = TraceAgentURL != ""

var (
	tracerOnce sync.Once

	// startTracerFn starts the tracer on first use. It is a variable so the unit
	// tests can substitute a no-op while the mocktracer owns the process's global
	// tracer; production code never reassigns it.
	startTracerFn = startTracer
)

// Enabled reports whether development tracing is enabled.
func Enabled() bool { return enabled }

// EnsureTracer lazily starts the development tracer at most once per process.
// It is fail-open: a start failure is logged and the span calls then run on the
// no-op tracer.
func EnsureTracer() {
	tracerOnce.Do(func() { startTracerFn() })
}

// SetEnabledForTest overrides the enabled gate and returns a restore function.
// Test seam only.
func SetEnabledForTest(value bool) (restore func()) {
	previous := enabled
	enabled = value
	return func() { enabled = previous }
}

// DisableTracerStartForTest makes EnsureTracer a no-op, keeping the real tracer
// out of unit tests that use the mocktracer, and returns a restore function.
// Test seam only.
func DisableTracerStartForTest() (restore func()) {
	previous := startTracerFn
	startTracerFn = func() {}
	return func() { startTracerFn = previous }
}

// startTracer starts the standard Datadog Go tracer against TraceAgentURL. The
// private-action-runner process owns no other dd-trace-go tracer, so the
// process-wide global tracer is exclusively the development one.
// Instrumentation telemetry, remote configuration, and runtime metrics are
// configured only through dd-trace-go environment variables, so they are turned
// off on the process before the first start: the development tracer must send
// nothing but the development spans to the development agent. The 128-bit
// trace-ID generation is disabled for the same reason — the runner's
// propagation contract carries 64-bit decimal IDs, and every span of a run must
// share that ID space.
func startTracer() {
	os.Setenv("DD_INSTRUMENTATION_TELEMETRY_ENABLED", "false")
	os.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "false")
	os.Setenv("DD_RUNTIME_METRICS_V2_ENABLED", "false")
	os.Setenv("DD_TRACE_128_BIT_TRACEID_GENERATION_ENABLED", "false")
	if err := tracer.Start(
		tracer.WithAgentURL(TraceAgentURL),
		tracer.WithService(Service),
		tracer.WithEnv(Env),
		tracer.WithLogStartup(false),
	); err != nil {
		log.Warnf("remote queries: development tracer failed to start, development spans disabled: %v", err)
	}
}
