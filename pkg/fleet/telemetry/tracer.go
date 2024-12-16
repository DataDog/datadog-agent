// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import "sync"

var (
	globalTracer *tracer
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
	t.mu.Lock()
	defer t.mu.Unlock()
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
	delete(t.spans, span.span.SpanID)
	t.completedSpans = append(t.completedSpans, span)
}

func (t *tracer) flushCompletedSpans() []*Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	newSpanArray := make([]*Span, 0)
	completedSpans := t.completedSpans
	t.completedSpans = newSpanArray
	return completedSpans
}
