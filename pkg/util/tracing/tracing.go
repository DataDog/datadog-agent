// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tracing provides helpers for creating APM spans in agent components.
// When the global dd-trace-go tracer has not been started (e.g. on the node
// agent), all helpers return no-op spans so callers never need nil checks.
package tracing

import (
	"context"
	"sync/atomic"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// running tracks whether the global tracer has been started.
var running atomic.Bool

// SetRunning marks the global tracer as running (true) or stopped (false).
// Call this immediately after tracer.Start() and before tracer.Stop().
func SetRunning(v bool) {
	running.Store(v)
}

// IsRunning reports whether the global dd-trace-go tracer has been started.
// It is safe to call from any goroutine.
func IsRunning() bool {
	return running.Load()
}

// StartSpanFromContext creates a new span from the given context.
// If the global tracer is not running it returns a no-op span and the
// original context, avoiding any overhead.
func StartSpanFromContext(ctx context.Context, operationName string, opts ...ddtrace.StartSpanOption) (ddtrace.Span, context.Context) {
	if !IsRunning() {
		return noopSpan{}, ctx
	}
	return tracer.StartSpanFromContext(ctx, operationName, opts...)
}

// noopSpan implements ddtrace.Span as a no-op.
type noopSpan struct{}

func (noopSpan) SetTag(_ string, _ interface{})   {}
func (noopSpan) SetOperationName(_ string)        {}
func (noopSpan) BaggageItem(_ string) string      { return "" }
func (noopSpan) SetBaggageItem(_, _ string)       {}
func (noopSpan) Finish(_ ...ddtrace.FinishOption) {}
func (noopSpan) Context() ddtrace.SpanContext     { return noopSpanContext{} }

// noopSpanContext implements ddtrace.SpanContext as a no-op.
type noopSpanContext struct{}

func (noopSpanContext) SpanID() uint64                              { return 0 }
func (noopSpanContext) TraceID() uint64                             { return 0 }
func (noopSpanContext) ForeachBaggageItem(_ func(k, v string) bool) {}
