// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package errortracking

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func runningPipeline(t *testing.T, opts Options) (*Pipeline, *recordingSender, func()) {
	t.Helper()
	sender := newRecordingSender()
	if opts.BatchSize == 0 {
		opts.BatchSize = 1
	}
	if opts.FlushInterval == 0 {
		opts.FlushInterval = time.Hour
	}
	p := NewPipeline(sender, opts)
	ctx, cancel := context.WithCancel(context.Background())
	go p.Run(ctx)
	cleanup := func() {
		drainCtx, drainCancel := context.WithTimeout(context.Background(), time.Second)
		defer drainCancel()
		_ = p.Drain(drainCtx)
		cancel()
	}
	return p, sender, cleanup
}

func TestHandler_Enabled(t *testing.T) {
	p, _, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	h := New(p)
	cases := []struct {
		level slog.Level
		want  bool
	}{
		{slog.LevelDebug, false},
		{slog.LevelInfo, false},
		{slog.LevelWarn, false},
		{slog.LevelError, true},
		{slog.LevelError + 4, true}, // Critical-ish levels must be enabled too.
	}
	for _, c := range cases {
		if got := h.Enabled(context.Background(), c.level); got != c.want {
			t.Errorf("Enabled(%v) = %v, want %v", c.level, got, c.want)
		}
	}
}

func TestHandler_FiltersByLevel(t *testing.T) {
	p, sender, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	h := New(p)

	// Lower levels: even if a caller bypasses Enabled and invokes Handle
	// directly, we drop the record.
	for _, lvl := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn} {
		r := slog.NewRecord(time.Now(), lvl, "skip", 0)
		if err := h.Handle(context.Background(), r); err != nil {
			t.Fatalf("Handle(%v): %v", lvl, err)
		}
	}

	// Error level: captured.
	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle(error): %v", err)
	}

	cleanup()

	batches := sender.snapshot()
	var msgs []string
	for _, b := range batches {
		for _, rec := range b {
			msgs = append(msgs, rec.Message)
		}
	}
	if len(msgs) != 1 || msgs[0] != "boom" {
		t.Fatalf("want only 'boom' captured, got %v", msgs)
	}
}

func TestHandler_NeverBlocks_UnderBackpressure(t *testing.T) {
	// Stuck Sender + tiny buffer; Handle must not block its caller.
	release := make(chan struct{})
	blocking := senderFunc(func(_ context.Context, _ []slog.Record) error {
		<-release
		return nil
	})
	p := NewPipeline(blocking, Options{
		BufferSize:    1,
		BatchSize:     1,
		FlushInterval: time.Hour,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	h := New(p)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = h.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelError, "x", 0))
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle blocked under back-pressure")
	}

	if dropped := p.Dropped(); dropped == 0 {
		t.Errorf("expected some records to be dropped under back-pressure, got dropped=%d", dropped)
	}
	close(release)
}

func TestHandler_WithAttrs_Preserved(t *testing.T) {
	p, sender, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	var sh slog.Handler = New(p)
	sh = sh.WithAttrs([]slog.Attr{slog.String("svc", "auth"), slog.Bool("retry", true)})

	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	r.AddAttrs(slog.Int("code", 500))
	if err := sh.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	cleanup()
	batches := sender.snapshot()
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("want 1 record, got %v", batches)
	}
	captured := batches[0][0]

	got := flatAttrs(captured)
	wantKeys := map[string]any{
		"svc":   "auth",
		"retry": true,
		"code":  int64(500),
	}
	for k, want := range wantKeys {
		v, ok := got[k]
		if !ok {
			t.Errorf("missing attr %q in captured record (got %v)", k, got)
			continue
		}
		if v != want {
			t.Errorf("attr %q = %v, want %v", k, v, want)
		}
	}
}

func TestHandler_WithAttrs_DoesNotMutateParent(t *testing.T) {
	p, sender, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	parent := New(p)
	child := parent.WithAttrs([]slog.Attr{slog.String("only_in_child", "1")})

	// Parent handle: must NOT include only_in_child.
	r := slog.NewRecord(time.Now(), slog.LevelError, "p", 0)
	if err := parent.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle parent: %v", err)
	}
	// Child handle: must include only_in_child.
	r2 := slog.NewRecord(time.Now(), slog.LevelError, "c", 0)
	if err := child.Handle(context.Background(), r2); err != nil {
		t.Fatalf("Handle child: %v", err)
	}

	cleanup()
	batches := sender.snapshot()
	var pAttrs, cAttrs map[string]any
	for _, b := range batches {
		for _, rec := range b {
			if rec.Message == "p" {
				pAttrs = flatAttrs(rec)
			}
			if rec.Message == "c" {
				cAttrs = flatAttrs(rec)
			}
		}
	}
	if _, ok := pAttrs["only_in_child"]; ok {
		t.Errorf("parent record contains child-only attr; got %v", pAttrs)
	}
	if v := cAttrs["only_in_child"]; v != "1" {
		t.Errorf("child record missing child attr; got %v", cAttrs)
	}
}

func TestHandler_WithGroup_NestsAttrs(t *testing.T) {
	p, sender, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	var sh slog.Handler = New(p)
	sh = sh.WithGroup("req")
	sh = sh.WithAttrs([]slog.Attr{slog.String("id", "abc")})

	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	r.AddAttrs(slog.Int("status", 500))
	if err := sh.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	cleanup()
	captured := sender.snapshot()[0][0]

	// Expect a top-level attr "req" of kind Group, containing id=abc and
	// status=500 inside.
	found := walkForGroup(captured, "req")
	if found == nil {
		t.Fatalf("group req not found in record attrs")
	}
	if v, ok := found["id"]; !ok || v != "abc" {
		t.Errorf("want req.id=abc, got %v", found)
	}
	if v, ok := found["status"]; !ok || v != int64(500) {
		t.Errorf("want req.status=500, got %v", found)
	}
}

func TestHandler_WithGroup_NestedGroups(t *testing.T) {
	p, sender, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	var sh slog.Handler = New(p)
	sh = sh.WithGroup("outer")
	sh = sh.WithAttrs([]slog.Attr{slog.String("o", "1")})
	sh = sh.WithGroup("inner")
	sh = sh.WithAttrs([]slog.Attr{slog.String("i", "2")})

	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	if err := sh.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	cleanup()
	captured := sender.snapshot()[0][0]

	outer := walkForGroup(captured, "outer")
	if outer == nil {
		t.Fatalf("outer group not found")
	}
	if v := outer["o"]; v != "1" {
		t.Errorf("want outer.o=1, got %v", outer)
	}
	innerVal, ok := outer["inner"]
	if !ok {
		t.Fatalf("inner group not found inside outer; got %v", outer)
	}
	innerMap, ok := innerVal.(map[string]any)
	if !ok {
		t.Fatalf("inner is not a group; got %T", innerVal)
	}
	if v := innerMap["i"]; v != "2" {
		t.Errorf("want outer.inner.i=2, got %v", innerMap)
	}
}

func TestHandler_WithGroup_Empty_NoOp(t *testing.T) {
	p, _, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	parent := New(p)
	child := parent.WithGroup("")
	if child != parent {
		t.Errorf("WithGroup(\"\") should return the same handler (no-op)")
	}
}

func TestHandler_WithAttrs_Empty_NoOp(t *testing.T) {
	p, _, cleanup := runningPipeline(t, Options{})
	defer cleanup()

	parent := New(p)
	child := parent.WithAttrs(nil)
	if child != parent {
		t.Errorf("WithAttrs(nil) should return the same handler (no-op)")
	}
}

// flatAttrs returns a flat map of top-level attrs (groups expanded as
// nested map[string]any).
func flatAttrs(r slog.Record) map[string]any {
	out := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		if a.Value.Kind() == slog.KindGroup {
			out[a.Key] = groupToMap(a.Value.Group())
		} else {
			out[a.Key] = a.Value.Any()
		}
		return true
	})
	return out
}

func groupToMap(attrs []slog.Attr) map[string]any {
	out := map[string]any{}
	for _, a := range attrs {
		if a.Value.Kind() == slog.KindGroup {
			out[a.Key] = groupToMap(a.Value.Group())
		} else {
			out[a.Key] = a.Value.Any()
		}
	}
	return out
}

// walkForGroup returns the contents of the first top-level group with the
// given name, flattened to map[string]any. Returns nil if not found.
func walkForGroup(r slog.Record, name string) map[string]any {
	var out map[string]any
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == name && a.Value.Kind() == slog.KindGroup {
			out = groupToMap(a.Value.Group())
			return false
		}
		return true
	})
	return out
}
