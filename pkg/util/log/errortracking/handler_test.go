// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package errortracking

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingSubmitter captures every ErrorLog it receives. Safe for
// concurrent use; tests can read back via snapshot().
type recordingSubmitter struct {
	mu   sync.Mutex
	logs []ErrorLog
}

func (r *recordingSubmitter) submit(e ErrorLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, e)
}

func (r *recordingSubmitter) snapshot() []ErrorLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ErrorLog, len(r.logs))
	copy(out, r.logs)
	return out
}

// loaderFor returns a load closure that reads s atomically. Mirrors the
// atomic.Pointer[Submitter] indirection used by the production wiring in
// pkg/util/log/setup so the test covers the same code path.
func loaderFor(s *atomic.Pointer[Submitter]) func() Submitter {
	return func() Submitter {
		p := s.Load()
		if p == nil {
			return nil
		}
		return *p
	}
}

// TestHandler_FiltersByLevel asserts the Error threshold on both Enabled
// (used by the parent multi-handler to skip work) and Handle (defensive
// against direct callers that bypass Enabled).
func TestHandler_FiltersByLevel(t *testing.T) {
	rec := &recordingSubmitter{}
	var slot atomic.Pointer[Submitter]
	sub := Submitter(rec.submit)
	slot.Store(&sub)
	h := NewHandler(loaderFor(&slot))

	cases := []struct {
		level       slog.Level
		wantEnabled bool
	}{
		{slog.LevelDebug, false},
		{slog.LevelInfo, false},
		{slog.LevelWarn, false},
		{slog.LevelError, true},
		{slog.LevelError + 4, true}, // Critical-ish levels remain enabled.
	}
	for _, c := range cases {
		if got := h.Enabled(context.Background(), c.level); got != c.wantEnabled {
			t.Errorf("Enabled(%v) = %v, want %v", c.level, got, c.wantEnabled)
		}
	}

	// Handle drops below-threshold records even if a caller bypasses Enabled.
	for _, lvl := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn} {
		r := slog.NewRecord(time.Now(), lvl, "skip", 0)
		if err := h.Handle(context.Background(), r); err != nil {
			t.Fatalf("Handle(%v): %v", lvl, err)
		}
	}
	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("below-threshold records leaked: %v", got)
	}

	// Error record is captured.
	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle(error): %v", err)
	}
	got := rec.snapshot()
	if len(got) != 1 || got[0].Message != "boom" {
		t.Fatalf("want only 'boom' captured, got %v", got)
	}
}

// TestHandler_CallsSubmitter_BuildsErrorLog asserts the Handler builds a
// faithful ErrorLog from the incoming Record - same Time/Level/Message/PC
// and the union of WithAttrs-accumulated attrs plus the record's own.
func TestHandler_CallsSubmitter_BuildsErrorLog(t *testing.T) {
	rec := &recordingSubmitter{}
	var slot atomic.Pointer[Submitter]
	sub := Submitter(rec.submit)
	slot.Store(&sub)

	var sh slog.Handler = NewHandler(loaderFor(&slot))
	sh = sh.WithAttrs([]slog.Attr{slog.String("svc", "auth")})

	now := time.Date(2026, time.May, 11, 15, 0, 0, 0, time.UTC)
	r := slog.NewRecord(now, slog.LevelError, "boom", uintptr(0xC0FFEE))
	r.AddAttrs(slog.Int("code", 500))

	if err := sh.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("want 1 ErrorLog, got %d", len(got))
	}
	e := got[0]
	if !e.Time.Equal(now) {
		t.Errorf("Time = %v, want %v", e.Time, now)
	}
	if e.Level != slog.LevelError {
		t.Errorf("Level = %v, want Error", e.Level)
	}
	if e.Message != "boom" {
		t.Errorf("Message = %q, want %q", e.Message, "boom")
	}
	if e.PC != uintptr(0xC0FFEE) {
		t.Errorf("PC = %v, want 0xC0FFEE", e.PC)
	}
	// WithAttrs attrs come first, then record's own attrs.
	if len(e.Attrs) != 2 || e.Attrs[0].Key != "svc" || e.Attrs[1].Key != "code" {
		t.Errorf("Attrs = %v, want [svc=auth, code=500]", e.Attrs)
	}
}

// TestHandler_NeverBlocks_WhenNoSubmitter asserts that the steady-state
// "no submitter registered" path is a fast no-op. The atomic-load fast
// path must not allocate or contend even at high call rates - this is the
// guarantee the logger hot path depends on before Fx wires the consumer.
func TestHandler_NeverBlocks_WhenNoSubmitter(t *testing.T) {
	var slot atomic.Pointer[Submitter]
	h := NewHandler(loaderFor(&slot))

	const total = 10000
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < total; i++ {
			_ = h.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelError, "x", 0))
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Handle blocked across %d calls with no Submitter registered", total)
	}
}
