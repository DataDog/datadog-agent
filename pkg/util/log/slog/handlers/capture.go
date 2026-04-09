// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CapturedLog holds a single intercepted log entry.
type CapturedLog struct {
	Level     string
	Message   string
	Timestamp time.Time
	// Attrs contains attributes attached to the log record by the emitting code,
	// plus any attributes accumulated via WithAttrs on this handler.
	Attrs map[string]string
}

// captureSharedState is the shared mutable buffer across all Capture handlers
// derived from the same root via WithAttrs/WithGroup. This ensures Drain() sees
// all captured records regardless of which derived handler emitted them.
type captureSharedState struct {
	mu      sync.Mutex
	buf     []CapturedLog
	maxSize int
}

func (s *captureSharedState) add(entry CapturedLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) >= s.maxSize {
		s.buf = s.buf[1:] // drop oldest when full
	}
	s.buf = append(s.buf, entry)
}

func (s *captureSharedState) drain() []CapturedLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.buf
	s.buf = nil
	return out
}

// Capture is a slog.Handler that intercepts Error and Critical log records into a
// bounded ring buffer without affecting normal log output. It wraps an inner
// handler to which all records are still forwarded normally.
//
// Multiple Capture handlers derived via WithAttrs/WithGroup share the same buffer,
// so a single Drain() call from agenttelemetry sees all captured records.
type Capture struct {
	*captureSharedState
	inner    slog.Handler
	preAttrs map[string]string // attrs accumulated via WithAttrs, injected into each captured entry
}

// NewCapture creates a Capture handler wrapping inner. maxSize is the ring buffer
// capacity; when full the oldest entry is dropped.
func NewCapture(inner slog.Handler, maxSize int) *Capture {
	return &Capture{
		captureSharedState: &captureSharedState{maxSize: maxSize},
		inner:              inner,
		preAttrs:           map[string]string{},
	}
}

// Enabled always returns true so that Error/Critical records reach Handle regardless
// of the configured output level. The inner handler applies its own level gating.
func (h *Capture) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle intercepts records at Error level or above into the ring buffer, then
// forwards the record to the inner handler.
func (h *Capture) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		entry := CapturedLog{
			Level:     r.Level.String(),
			Message:   r.Message,
			Timestamp: r.Time,
			Attrs:     make(map[string]string, len(h.preAttrs)),
		}
		for k, v := range h.preAttrs {
			entry.Attrs[k] = v
		}
		r.Attrs(func(a slog.Attr) bool {
			entry.Attrs[a.Key] = a.Value.String()
			return true
		})
		h.add(entry)
	}
	if h.inner.Enabled(ctx, r.Level) {
		return h.inner.Handle(ctx, r)
	}
	return nil
}

// WithAttrs returns a derived Capture handler that shares the same ring buffer
// and enriches every captured entry with the given attributes.
func (h *Capture) WithAttrs(attrs []slog.Attr) slog.Handler {
	newPreAttrs := make(map[string]string, len(h.preAttrs)+len(attrs))
	for k, v := range h.preAttrs {
		newPreAttrs[k] = v
	}
	for _, a := range attrs {
		newPreAttrs[a.Key] = a.Value.String()
	}
	return &Capture{
		captureSharedState: h.captureSharedState,
		inner:              h.inner.WithAttrs(attrs),
		preAttrs:           newPreAttrs,
	}
}

// WithGroup returns a derived Capture handler that shares the same ring buffer.
// Group namespacing is delegated to the inner handler; the capture side does not
// prefix attrs with the group name (POC simplification).
func (h *Capture) WithGroup(name string) slog.Handler {
	return &Capture{
		captureSharedState: h.captureSharedState,
		inner:              h.inner.WithGroup(name),
		preAttrs:           h.preAttrs,
	}
}

// Drain removes and returns all buffered log entries since the last call.
// It is safe to call concurrently with Handle.
func (h *Capture) Drain() []CapturedLog {
	return h.drain()
}
