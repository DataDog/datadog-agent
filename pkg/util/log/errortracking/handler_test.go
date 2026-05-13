// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package errortracking

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
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
// faithful ErrorLog from the incoming Record on the fields it still
// captures after the PR #50607 PII pivot: Time, Level, Message, PC.
// Attrs (WithAttrs-accumulated and record-level) are intentionally not
// copied into the DTO any more — they would be dropped at the sender
// boundary anyway, so the handler does not allocate them. Adding
// attrs to the slog chain MUST NOT break the call (other handlers in
// the multi-handler chain still see them).
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
	if len(e.Attrs) != 0 {
		t.Errorf("Attrs must be empty post-PII-pivot, got %v", e.Attrs)
	}
}

// TestHandle_StackSkipBase locks the stackSkipBase constant: a real
// logger.Error(...) call routed through the slog chain must produce a
// captured stack whose first frame is in the test source file (the user
// call site), not in slog plumbing. If a future slog version changes
// the number of internal frames between user code and Handler.Handle,
// this test fails and the constant must be re-calibrated.
func TestHandle_StackSkipBase(t *testing.T) {
	rec := &recordingSubmitter{}
	var slot atomic.Pointer[Submitter]
	sub := Submitter(rec.submit)
	slot.Store(&sub)

	h := NewHandler(loaderFor(&slot))
	logger := slog.New(h)
	logger.Error("calibration-marker") // <- user call site we expect at PCs[0]

	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("want 1 ErrorLog, got %d", len(got))
	}
	e := got[0]
	if e.PCsLen == 0 {
		t.Fatalf("handler must populate PCs")
	}
	frame, _ := runtime.CallersFrames(e.PCs[:1]).Next()
	if frame.File == "" {
		t.Fatalf("frame symbol resolution failed for PCs[0]")
	}
	if !strings.HasSuffix(frame.File, "handler_test.go") {
		t.Fatalf("PCs[0] must point at the user call site, got %s (skipBase=%d miscalibrated)",
			frame.File, stackSkipBase)
	}
}

// TestHandler_BouncerSuppressesDuplicates: when a Bouncer is attached
// via WithBouncerLoader, the second sighting of a given PC inside the
// window must NOT reach the Submitter. The Submitter sees the first
// sighting with Count==1, and the count accumulates on subsequent
// non-suppressed sightings.
func TestHandler_BouncerSuppressesDuplicates(t *testing.T) {
	rec := &recordingSubmitter{}
	var slot atomic.Pointer[Submitter]
	sub := Submitter(rec.submit)
	slot.Store(&sub)

	bouncer := NewBouncer(15*time.Minute, 0)
	h := NewHandler(loaderFor(&slot)).WithBouncerLoader(func() *Bouncer { return bouncer })

	// Same PC repeated four times — slog.NewRecord with explicit PC.
	pc := uintptr(0xABCDEF)
	for i := 0; i < 4; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelError, "same site", pc)
		if err := h.Handle(context.Background(), r); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}

	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("Bouncer must suppress duplicates — got %d records, want 1", len(got))
	}
	if got[0].Count != 1 {
		t.Fatalf("first sighting Count = %d, want 1", got[0].Count)
	}
}

// TestHandler_BouncerNilLoaderIsPassThrough: a nil bouncer loader (or
// a loader returning nil) MUST disable dedup. Every record reaches the
// Submitter with Count=1.
func TestHandler_BouncerNilLoaderIsPassThrough(t *testing.T) {
	rec := &recordingSubmitter{}
	var slot atomic.Pointer[Submitter]
	sub := Submitter(rec.submit)
	slot.Store(&sub)

	h := NewHandler(loaderFor(&slot)).WithBouncerLoader(func() *Bouncer { return nil })

	pc := uintptr(0xABCDEF)
	for i := 0; i < 3; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelError, "same site", pc)
		_ = h.Handle(context.Background(), r)
	}

	got := rec.snapshot()
	if len(got) != 3 {
		t.Fatalf("nil-bouncer loader must NOT dedup — got %d records, want 3", len(got))
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
