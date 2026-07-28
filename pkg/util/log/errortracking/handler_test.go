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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func newRecordingHandler(t *testing.T) (*Handler, *recordingSubmitter) {
	t.Helper()
	rec := &recordingSubmitter{}
	var slot atomic.Pointer[Submitter]
	sub := Submitter(rec.submit)
	slot.Store(&sub)
	// A Bouncer is mandatory for submission. Use a long window so tests
	// see every record unless they explicitly need suppression behaviour.
	bouncer := NewBouncer(24*time.Hour, 0)
	return NewHandler(loaderFor(&slot)).WithBouncerLoader(func() *Bouncer { return bouncer }), rec
}

// TestHandler_FiltersByLevel asserts the Error threshold on both Enabled
// (used by the parent multi-handler to skip work) and Handle (defensive
// against direct callers that bypass Enabled).
func TestHandler_FiltersByLevel(t *testing.T) {
	h, rec := newRecordingHandler(t)

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
		assert.Equalf(t, c.wantEnabled, h.Enabled(context.Background(), c.level),
			"Enabled(%v)", c.level)
	}

	// Handle drops below-threshold records even if a caller bypasses Enabled.
	for _, lvl := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn} {
		r := slog.NewRecord(time.Now(), lvl, "skip", 0)
		require.NoError(t, h.Handle(context.Background(), r))
	}
	assert.Empty(t, rec.snapshot(), "below-threshold records must NOT leak to the Submitter")

	// Error record is captured.
	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	require.NoError(t, h.Handle(context.Background(), r))
	got := rec.snapshot()
	require.Len(t, got, 1)
}

// TestHandler_CallsSubmitter_BuildsErrorLog asserts the Handler builds a
// faithful ErrorLog from the incoming Record on the fields it still
// captures: Time and PC. Adding attrs to the slog chain MUST NOT break
// the call (other handlers in the multi-handler chain still see them).
func TestHandler_CallsSubmitter_BuildsErrorLog(t *testing.T) {
	h, rec := newRecordingHandler(t)

	var sh slog.Handler = h
	sh = sh.WithAttrs([]slog.Attr{slog.String("svc", "auth")})

	now := time.Date(2026, time.May, 11, 15, 0, 0, 0, time.UTC)
	r := slog.NewRecord(now, slog.LevelError, "boom", uintptr(0xC0FFEE))
	r.AddAttrs(slog.Int("code", 500))

	require.NoError(t, sh.Handle(context.Background(), r))

	got := rec.snapshot()
	require.Len(t, got, 1)
	e := got[0]
	assert.True(t, e.Time.Equal(now), "Time mismatch: got %v, want %v", e.Time, now)
	assert.Equal(t, uintptr(0xC0FFEE), e.PC)
}

// TestHandler_WithGroup_ReturnsNewInstance: WithGroup MUST return a new
// Handler instance with the same load closures and attrs (group name
// discarded). A no-op-receiver shape can subtly break parent
// multi-handlers that clone via WithGroup expecting each child to
// materialize a fresh instance per group context. Matches the canonical
// shape of pkg/util/log/slog/handlers/multi.go::WithGroup. Closes
// iglendd's "we probably need empty implementation here" thread on
// PR #50607.
func TestHandler_WithGroup_ReturnsNewInstance(t *testing.T) {
	h, _ := newRecordingHandler(t)
	withGroup := h.WithGroup("group-name")
	assert.NotSame(t, any(h), any(withGroup),
		"WithGroup must return a new instance, not the receiver")
}

// TestHandle_CallSiteIsFirstFrame verifies that PCs[0] points at the
// actual user call site (this test file), not at any slog or
// pkg/util/log wrapper frame. The handler anchors PCs at r.PC rather
// than using a fixed frame-skip constant, so this holds for both
// direct slog calls and calls routed via the pkg/util/log wrappers.
func TestHandle_CallSiteIsFirstFrame(t *testing.T) {
	h, rec := newRecordingHandler(t)

	logger := slog.New(h)
	logger.Error("calibration-marker") // <- user call site we expect at PCs[0]

	got := rec.snapshot()
	require.Len(t, got, 1)
	e := got[0]
	require.NotZero(t, e.PCsLen, "handler must populate PCs")

	frame, _ := runtime.CallersFrames(e.PCs[:1]).Next()
	require.NotEmpty(t, frame.File, "frame symbol resolution failed for PCs[0]")
	assert.Truef(t, strings.HasSuffix(frame.File, "handler_test.go"),
		"PCs[0] must point at the user call site, got %s",
		frame.File)
}

// TestHandler_BouncerSuppressesDuplicates: when a Bouncer is attached
// via WithBouncerLoader, the second sighting of a given PC inside the
// window must NOT reach the Submitter. The Submitter sees the first
// sighting with Count==1.
func TestHandler_BouncerSuppressesDuplicates(t *testing.T) {
	h, rec := newRecordingHandler(t)

	bouncer := NewBouncer(15*time.Minute, 0)
	h = h.WithBouncerLoader(func() *Bouncer { return bouncer })

	pc := uintptr(0xABCDEF)
	for i := 0; i < 4; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelError, "same site", pc)
		require.NoError(t, h.Handle(context.Background(), r))
	}

	got := rec.snapshot()
	require.Len(t, got, 1, "Bouncer must suppress duplicates")
	assert.Equal(t, uint32(1), got[0].Count, "first sighting Count")
}

// TestHandle_BouncerRegisteredButNilDropsRecord: when WithBouncerLoader
// is called (bouncer is in the design) but the closure returns nil
// (e.g. the brief Fx startup/shutdown window), the record must be
// DROPPED, not forwarded without rate-limiting. Sending without dedup
// during a fleet-wide restart is the higher risk.
func TestHandle_BouncerRegisteredButNilDropsRecord(t *testing.T) {
	h, rec := newRecordingHandler(t)
	h = h.WithBouncerLoader(func() *Bouncer { return nil })

	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	require.NoError(t, h.Handle(context.Background(), r))

	assert.Empty(t, rec.snapshot(), "registered-but-nil bouncer must DROP the record, not pass through")
}

// TestHandler_BouncerNilLoaderDropsRecord: passing nil to WithBouncerLoader
// clears the late-binder. Since a Bouncer is mandatory for submission, a nil
// loader must DROP all records — the same behaviour as a registered loader
// that returns nil.
func TestHandler_BouncerNilLoaderDropsRecord(t *testing.T) {
	h, rec := newRecordingHandler(t)
	h = h.WithBouncerLoader(nil)

	pc := uintptr(0xABCDEF)
	for i := 0; i < 3; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelError, "same site", pc)
		require.NoError(t, h.Handle(context.Background(), r))
	}

	assert.Empty(t, rec.snapshot(), "nil bouncer loader must drop all records")
}

// TestHandler_StackHashKeyDistinguishesStacks: two different stacks
// reaching the same terminal slog.Error call site must produce
// DIFFERENT bouncer keys (because the bouncer key is the FNV-1a hash
// of the full captured PCs slice) and BOTH must ship to the
// Submitter. The old r.PC-keyed bouncer would have collapsed these
// two stacks into one, hiding the distinct call paths from the wire.
func TestHandler_StackHashKeyDistinguishesStacks(t *testing.T) {
	h, rec := newRecordingHandler(t)
	bouncer := NewBouncer(15*time.Minute, 0)
	h = h.WithBouncerLoader(func() *Bouncer { return bouncer })

	logger := slog.New(h)

	// emit is a shared closure whose Error call site is the same for
	// both callers; the parent frames (fromA vs fromB) diverge above.
	emit := func() { logger.Error("e") }

	fromA := func() { emit() }
	fromB := func() { emit() }

	fromA()
	fromB()

	got := rec.snapshot()
	require.Len(t, got, 2, "two distinct stacks must each ship a record (no cross-stack dedup)")
}

// TestHandler_BouncerSuppressionAndCountDelivery: 5 emits from the
// same call site ship 1 record with Count=1 (first sighting); the
// remaining 4 are suppressed inside the window. The
// suppressed-duplicate total is carried on the NEXT delivered record
// once the window elapses (see TestBouncer_WindowElapseCarriesPriorCount
// for that path) — within a single window the first delivery's Count
// is 1.
func TestHandler_BouncerSuppressionAndCountDelivery(t *testing.T) {
	h, rec := newRecordingHandler(t)
	bouncer := NewBouncer(15*time.Minute, 0)
	h = h.WithBouncerLoader(func() *Bouncer { return bouncer })

	logger := slog.New(h)

	for i := 0; i < 5; i++ {
		logger.Error("e")
	}

	got := rec.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, uint32(1), got[0].Count)
}

// TestHandler_NeverBlocks_WhenNoSubmitter asserts that the steady-state
// "no submitter registered" path is a fast no-op. The atomic-load fast
// path must not allocate or contend even at high call rates - this is
// the guarantee the logger hot path depends on before Fx wires the
// consumer.
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

// BenchmarkErrortrackingHandle_NoSubmitter measures the steady-state
// off-path cost when no Submitter is registered — the default state
// for agents that have not opted in to errortracking. The Enabled
// gate / load-closure short-circuit must keep this path negligibly
// cheap so the foundational logger hot path is not taxed.
func BenchmarkErrortrackingHandle_NoSubmitter(b *testing.B) {
	h := NewHandler(func() Submitter { return nil })
	logger := slog.New(h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Error("e")
	}
}

// BenchmarkErrortrackingHandle_FullPath measures the on-path cost when
// both a Submitter and a Bouncer are wired. The benchmark reuses the
// same call site for every iteration, so the Bouncer suppresses every
// emit past the first — what we measure is the steady-state cost of
// the FNV stack-hash + Bouncer.Observe lookup + suppression decision
// (the common case once a stack has been seen at least once in the
// current window).
func BenchmarkErrortrackingHandle_FullPath(b *testing.B) {
	submit := func(_ ErrorLog) {}
	bouncer := NewBouncer(15*time.Minute, 0)
	h := NewHandler(func() Submitter { return submit }).
		WithBouncerLoader(func() *Bouncer { return bouncer })
	logger := slog.New(h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Error("e")
	}
}
