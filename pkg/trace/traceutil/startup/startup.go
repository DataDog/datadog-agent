// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package startup records framework-independent startup diagnostic traces.
package startup

import (
	"context"
	"maps"
	"math/rand"
	"sync"
	"time"
)

// Tracer starts opt-in startup diagnostics. Implementations own transport and
// enablement; a nil Tracer disables diagnostics.
type Tracer interface {
	StartStartupTrace(resource string) *Span
}

// PayloadSpan is the Datadog v0.3 JSON representation of a recorded span.
type PayloadSpan struct {
	Service  string             `json:"service"`
	Name     string             `json:"name"`
	Resource string             `json:"resource"`
	TraceID  uint64             `json:"trace_id"`
	SpanID   uint64             `json:"span_id"`
	ParentID uint64             `json:"parent_id"`
	Start    int64              `json:"start"`
	Duration int64              `json:"duration"`
	Error    int32              `json:"error"`
	Meta     map[string]string  `json:"meta,omitempty"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
	Type     string             `json:"type,omitempty"`
}

type startupSpanKey struct{}

type startupTrace struct {
	mu        sync.Mutex
	start     time.Time
	spans     []*PayloadSpan
	finished  bool
	send      func([]*PayloadSpan)
	phaseName string
}

// Span records opt-in startup diagnostics, including work that outlives
// Fx Started. A nil span is a no-op. Methods are safe for concurrent workers.
type Span struct {
	trace    *startupTrace
	span     *PayloadSpan
	start    time.Time
	finished bool
}

// NewTrace starts a diagnostic trace whose root owns delivery through send.
// Call Finish on the root only after joining all background children.
func NewTrace(service, resource string, send func([]*PayloadSpan)) *Span {
	t := &startupTrace{start: time.Now(), send: send, phaseName: resource + ".phase"}
	root := &Span{trace: t, start: t.start, span: &PayloadSpan{
		Service: service, Name: resource + ".startup", Resource: resource,
		TraceID: rand.Uint64(), SpanID: rand.Uint64(), Type: "custom",
	}}
	root.span.Start = root.start.UnixNano()
	t.spans = append(t.spans, root.span)
	return root
}

// Context attaches this span while preserving the caller's cancellation/deadline.
func (s *Span) Context(ctx context.Context) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, startupSpanKey{}, s)
}

// StartSpan starts a child of the diagnostic span in ctx. Without an
// active diagnostic trace it returns ctx unchanged and a nil, no-op span.
func StartSpan(ctx context.Context, resource string) (context.Context, *Span) {
	parent, _ := ctx.Value(startupSpanKey{}).(*Span)
	if parent == nil {
		return ctx, nil
	}
	t := parent.trace
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || parent.finished {
		return ctx, nil
	}
	s := &Span{trace: t, start: time.Now(), span: &PayloadSpan{
		Service: parent.span.Service, Name: t.phaseName, Resource: resource,
		TraceID: parent.span.TraceID, SpanID: rand.Uint64(), ParentID: parent.span.SpanID, Type: "custom",
		Meta: maps.Clone(parent.span.Meta),
	}}
	// Use one wall-clock anchor and monotonic offsets for the entire trace.
	// Independently reading wall time for each child can put its end beyond the
	// root on clocks with different wall/monotonic resolution or clock corrections.
	s.span.Start = t.start.UnixNano() + s.start.Sub(t.start).Nanoseconds()
	t.spans = append(t.spans, s.span)
	return s.Context(ctx), s
}

// Finish completes a span. Finishing the root sends the trace once and releases
// its buffer; all background children must have been joined before that point.
func (s *Span) Finish(err error) {
	if s == nil {
		return
	}
	t := s.trace
	t.mu.Lock()
	if s.finished || t.finished {
		t.mu.Unlock()
		return
	}
	s.finished = true
	s.span.Duration = time.Since(s.start).Nanoseconds()
	if err != nil {
		s.span.Error = 1
	}
	var spans []*PayloadSpan
	if s.span.ParentID == 0 {
		t.finished = true
		spans, t.spans = t.spans, nil
	}
	t.mu.Unlock()
	if spans != nil {
		t.send(spans)
	}
}

// SetTag records a bounded diagnostic dimension, never configuration contents.
func (s *Span) SetTag(key, value string) {
	if s == nil {
		return
	}
	s.trace.mu.Lock()
	defer s.trace.mu.Unlock()
	if s.finished || s.trace.finished {
		return
	}
	if s.span.Meta == nil {
		s.span.Meta = make(map[string]string)
	}
	s.span.Meta[key] = value
}

// AddMetric accumulates numeric diagnostics, including concurrent worker totals.
func (s *Span) AddMetric(key string, value float64) {
	if s == nil {
		return
	}
	s.trace.mu.Lock()
	defer s.trace.mu.Unlock()
	if s.finished || s.trace.finished {
		return
	}
	if s.span.Metrics == nil {
		s.span.Metrics = make(map[string]float64)
	}
	s.span.Metrics[key] += value
}

// Measure accumulates elapsed nanoseconds for a worker stage. Concurrent worker
// totals may exceed the scan's wall duration and must not be summed as latency.
func (s *Span) Measure(metric string) func() {
	if s == nil {
		return func() {}
	}
	start := time.Now()
	return func() { s.AddMetric(metric, float64(time.Since(start).Nanoseconds())) }
}
