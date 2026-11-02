// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logging

import (
	"context"
	"maps"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

type startupSpanKey struct{}

type startupTrace struct {
	mu        sync.Mutex
	start     time.Time
	spans     []*Span
	finished  bool
	send      func([]*Span)
	phaseName string
}

// StartupSpan records opt-in startup diagnostics, including work that outlives
// Fx Started. A nil span is a no-op. Methods are safe for concurrent workers.
type StartupSpan struct {
	trace    *startupTrace
	span     *Span
	start    time.Time
	finished bool
}

// StartStartupTrace starts a separate diagnostic trace. It does not extend the
// Fx startup root: that root still ends at Fx Started. The fx.trace_id tag links
// the traces, and the caller must finish this root after joining background work.
// A nil or disabled Fx tracer returns a nil, no-op span.
func (l *FxTracingLogger) StartStartupTrace(resource string) *StartupSpan {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	port, writer, flavor, fxTraceID := l.traceAgentPort, l.debugLogger, l.flavor, l.traceID
	l.mu.Unlock()
	if port == "" {
		return nil
	}
	root := newStartupTrace(resource, func(spans []*Span) {
		go sendSpansToDatadog(writer, spans, port)
	})
	root.SetTag("process_name", flavor)
	root.SetTag("fx.trace_id", strconv.FormatUint(fxTraceID, 10))
	return root
}

func newStartupTrace(resource string, send func([]*Span)) *StartupSpan {
	t := &startupTrace{start: time.Now(), send: send, phaseName: resource + ".phase"}
	root := &StartupSpan{trace: t, start: t.start, span: &Span{
		Service: serviceName, Name: resource + ".startup", Resource: resource,
		TraceID: rand.Uint64(), SpanID: rand.Uint64(), Type: "custom",
	}}
	root.span.Start = root.start.UnixNano()
	t.spans = append(t.spans, root.span)
	return root
}

// Context attaches this span while preserving the caller's cancellation/deadline.
func (s *StartupSpan) Context(ctx context.Context) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, startupSpanKey{}, s)
}

// StartStartupSpan starts a child of the diagnostic span in ctx. Without an
// active diagnostic trace it returns ctx unchanged and a nil, no-op span.
func StartStartupSpan(ctx context.Context, resource string) (context.Context, *StartupSpan) {
	parent, _ := ctx.Value(startupSpanKey{}).(*StartupSpan)
	if parent == nil {
		return ctx, nil
	}
	t := parent.trace
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || parent.finished {
		return ctx, nil
	}
	s := &StartupSpan{trace: t, start: time.Now(), span: &Span{
		Service: serviceName, Name: t.phaseName, Resource: resource,
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
func (s *StartupSpan) Finish(err error) {
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
	s.span.Error = errToCode(err)
	var spans []*Span
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
func (s *StartupSpan) SetTag(key, value string) {
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
func (s *StartupSpan) AddMetric(key string, value float64) {
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
func (s *StartupSpan) Measure(metric string) func() {
	if s == nil {
		return func() {}
	}
	start := time.Now()
	return func() { s.AddMetric(metric, float64(time.Since(start).Nanoseconds())) }
}
