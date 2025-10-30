// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"math"
	"strings"
	"sync"
)

const (
	dropTraceID      = 1
	maxSpansInFlight = 1000
)

var (
	globalTracer  *tracer
	samplingRates = map[string]float64{
		"cdn":                       0.1,
		"installer.garbage_collect": 0.05,
		"garbage_collect":           0.05,
		"HTTPClient":                0.05,
		"agent.startup":             0.0,
		"get_states":                0.01,
		"installer.get_states":      0.01,
		"installer.get-states":      0.01,
	}
)

func init() {
	globalTracer = &tracer{
		spans: make(map[uint64]*Span),
	}
}

type tracer struct {
	mu             sync.Mutex
	spans          map[uint64]*Span
	completedSpans []*Span
}

func (t *tracer) registerSpan(span *Span) {
	if span.span.TraceID == dropTraceID {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	// naive maxSpansInFlight check as this is just telemetry
	// next iteration if needed would be to flush long running spans to troubleshoot
	if len(t.spans) >= maxSpansInFlight {
		return
	}
	t.spans[span.span.SpanID] = span
}

func (t *tracer) getSpan(spanID uint64) (*Span, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	span, ok := t.spans[spanID]
	return span, ok
}

func (t *tracer) finishSpan(span *Span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.spans[span.span.SpanID]; exists {
		delete(t.spans, span.span.SpanID)
		t.completedSpans = append(t.completedSpans, span)
	}
}

func (t *tracer) flushCompletedSpans() []*Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	newSpanArray := make([]*Span, 0)
	completedSpans := t.completedSpans
	t.completedSpans = newSpanArray
	return completedSpans
}

func headSamplingKeep(spanName string, traceID uint64) bool {
	for k, r := range samplingRates {
		if strings.Contains(spanName, k) {
			return sampledByRate(traceID, r)
		}
	}
	return true
}

func sampledByRate(n uint64, rate float64) bool {
	if rate < 1 {
		return n*uint64(1111111111111111111) < uint64(rate*math.MaxUint64)
	}
	return true
}

// SetSamplingRate sets the sampling rate for a given span name.
// The rate must be between 0 and 1.
func SetSamplingRate(name string, rate float64) {
	if rate < 0 || rate > 1 {
		return
	}
	samplingRates[name] = rate
}
