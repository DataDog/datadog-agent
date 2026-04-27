// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package errortracking

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// senderFunc lets a plain function satisfy the Sender interface.
type senderFunc func(ctx context.Context, batch []slog.Record) error

func (f senderFunc) Send(ctx context.Context, batch []slog.Record) error {
	return f(ctx, batch)
}

// procFunc lets a plain function satisfy the Processor interface.
type procFunc func(r *slog.Record) *slog.Record

func (f procFunc) Process(r *slog.Record) *slog.Record { return f(r) }

// recordingSender captures every batch passed to Send for later inspection.
type recordingSender struct {
	mu      sync.Mutex
	batches [][]slog.Record
	sentCh  chan struct{}
}

func newRecordingSender() *recordingSender {
	return &recordingSender{sentCh: make(chan struct{}, 64)}
}

func (s *recordingSender) Send(_ context.Context, batch []slog.Record) error {
	cp := make([]slog.Record, len(batch))
	copy(cp, batch)
	s.mu.Lock()
	s.batches = append(s.batches, cp)
	s.mu.Unlock()
	select {
	case s.sentCh <- struct{}{}:
	default:
	}
	return nil
}

func (s *recordingSender) snapshot() [][]slog.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]slog.Record, len(s.batches))
	for i, b := range s.batches {
		cp := make([]slog.Record, len(b))
		copy(cp, b)
		out[i] = cp
	}
	return out
}

func (s *recordingSender) waitForSend(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-s.sentCh:
	case <-time.After(timeout):
		t.Fatalf("Sender.Send was not called within %s", timeout)
	}
}

func errorRecord(msg string) slog.Record {
	return slog.NewRecord(time.Now(), slog.LevelError, msg, 0)
}

func TestPipeline_BatchSizeFlush(t *testing.T) {
	sender := newRecordingSender()
	p := NewPipeline(sender, Options{
		BatchSize:     5,
		FlushInterval: time.Hour, // Long enough that only batch-size triggers flush.
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	for i := 0; i < 5; i++ {
		p.Submit(errorRecord(fmt.Sprintf("m%d", i)))
	}

	sender.waitForSend(t, 2*time.Second)

	batches := sender.snapshot()
	if len(batches) != 1 {
		t.Fatalf("want 1 batch, got %d (%v)", len(batches), batches)
	}
	if len(batches[0]) != 5 {
		t.Fatalf("want batch of 5, got %d", len(batches[0]))
	}
}

func TestPipeline_TimeBasedFlush(t *testing.T) {
	sender := newRecordingSender()
	p := NewPipeline(sender, Options{
		BatchSize:     100,
		FlushInterval: 50 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	p.Submit(errorRecord("only"))

	sender.waitForSend(t, 2*time.Second)

	batches := sender.snapshot()
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("want 1 batch of 1; got %d batches: %v", len(batches), batches)
	}
	if batches[0][0].Message != "only" {
		t.Fatalf("want message=only, got %q", batches[0][0].Message)
	}
}

func TestPipeline_ProcessorChain(t *testing.T) {
	dropDrop := procFunc(func(r *slog.Record) *slog.Record {
		if r.Message == "drop" {
			return nil
		}
		return r
	})
	prefix := procFunc(func(r *slog.Record) *slog.Record {
		nr := slog.NewRecord(r.Time, r.Level, "p2:"+r.Message, r.PC)
		return &nr
	})

	sender := newRecordingSender()
	p := NewPipeline(sender, Options{
		BatchSize:     2, // We expect exactly 2 records to make it through.
		FlushInterval: time.Hour,
		Processors:    []Processor{dropDrop, prefix},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	p.Submit(errorRecord("keep1"))
	p.Submit(errorRecord("drop"))
	p.Submit(errorRecord("keep2"))

	sender.waitForSend(t, 2*time.Second)

	var msgs []string
	for _, b := range sender.snapshot() {
		for _, r := range b {
			msgs = append(msgs, r.Message)
		}
	}
	if len(msgs) != 2 || msgs[0] != "p2:keep1" || msgs[1] != "p2:keep2" {
		t.Fatalf("want [p2:keep1 p2:keep2], got %v", msgs)
	}
}

func TestPipeline_ProcessorChain_OrderMatters(t *testing.T) {
	// Two processors; first appends "A", second appends "B".
	// We expect output to be "msgAB".
	appendA := procFunc(func(r *slog.Record) *slog.Record {
		nr := slog.NewRecord(r.Time, r.Level, r.Message+"A", r.PC)
		return &nr
	})
	appendB := procFunc(func(r *slog.Record) *slog.Record {
		nr := slog.NewRecord(r.Time, r.Level, r.Message+"B", r.PC)
		return &nr
	})

	sender := newRecordingSender()
	p := NewPipeline(sender, Options{
		BatchSize:     1,
		FlushInterval: time.Hour,
		Processors:    []Processor{appendA, appendB},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	p.Submit(errorRecord("msg"))
	sender.waitForSend(t, 2*time.Second)

	batches := sender.snapshot()
	if batches[0][0].Message != "msgAB" {
		t.Fatalf("want msgAB, got %q", batches[0][0].Message)
	}
}

func TestPipeline_OverflowDropsOldest(t *testing.T) {
	// Sender blocks on the first Send until release is closed. While Run is
	// stuck in Send, subsequent Submit calls overflow the buffer.
	release := make(chan struct{})
	sendStarted := make(chan struct{}, 1)
	var blockedOnce sync.Once

	delivered := newRecordingSender()
	blocking := senderFunc(func(ctx context.Context, batch []slog.Record) error {
		blockedOnce.Do(func() {
			sendStarted <- struct{}{}
			<-release
		})
		return delivered.Send(ctx, batch)
	})

	p := NewPipeline(blocking, Options{
		BufferSize:    2,
		BatchSize:     1, // Each record triggers immediate flush attempt.
		FlushInterval: time.Hour,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	// First record: drains into a batch and stalls inside Send.
	p.Submit(errorRecord("first"))
	<-sendStarted

	// Run is now blocked. Subsequent Submits fill the buffer (cap=2)
	// and then drop oldest.
	p.Submit(errorRecord("m1"))
	p.Submit(errorRecord("m2")) // buffer: [m1, m2]
	p.Submit(errorRecord("m3")) // drop m1 → buffer: [m2, m3]
	p.Submit(errorRecord("m4")) // drop m2 → buffer: [m3, m4]
	p.Submit(errorRecord("m5")) // drop m3 → buffer: [m4, m5]

	if got := p.Dropped(); got != 3 {
		t.Fatalf("want 3 dropped, got %d", got)
	}

	close(release)

	drainCtx, drainCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer drainCancel()
	if err := p.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var msgs []string
	for _, b := range delivered.snapshot() {
		for _, r := range b {
			msgs = append(msgs, r.Message)
		}
	}
	want := []string{"first", "m4", "m5"}
	if !equalStrings(msgs, want) {
		t.Fatalf("delivered = %v, want %v (oldest m1/m2/m3 should have been dropped)", msgs, want)
	}
}

func TestPipeline_SenderError_NoBackpressure(t *testing.T) {
	var attempts atomic.Int32
	failing := senderFunc(func(_ context.Context, _ []slog.Record) error {
		attempts.Add(1)
		return errors.New("boom")
	})

	p := NewPipeline(failing, Options{
		BatchSize:     1,
		FlushInterval: time.Hour,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	p.Submit(errorRecord("first"))

	if !waitForCount(&attempts, 2, 2*time.Second) {
		t.Fatalf("want 2 attempts (1 initial + 1 retry) for first batch, got %d", attempts.Load())
	}
	// Batch should have been dropped after retry failure.
	if got := p.Dropped(); got != 1 {
		t.Fatalf("want 1 record counted as dropped after retry failure, got %d", got)
	}

	// Subsequent submit must still flow (no back-pressure).
	p.Submit(errorRecord("second"))
	if !waitForCount(&attempts, 4, 2*time.Second) {
		t.Fatalf("want 4 attempts after second submit, got %d", attempts.Load())
	}
}

func TestPipeline_Drain(t *testing.T) {
	sender := newRecordingSender()
	p := NewPipeline(sender, Options{
		BatchSize:     100,
		FlushInterval: time.Hour, // Without Drain, this batch would never flush.
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	p.Submit(errorRecord("a"))
	p.Submit(errorRecord("b"))

	// Allow Run to pull both records from the channel into the batch.
	// We can't observe this directly, so we poll for the channel to drain.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		// Indirectly: nothing in channel means Run picked them up.
		time.Sleep(5 * time.Millisecond)
		if len(p.in) == 0 {
			break
		}
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer drainCancel()
	if err := p.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var total int
	for _, b := range sender.snapshot() {
		total += len(b)
	}
	if total != 2 {
		t.Fatalf("want 2 records flushed by Drain, got %d", total)
	}
}

func TestPipeline_Drain_BeforeRun_TimesOut(t *testing.T) {
	// Drain blocks until Run flushes; if Run was never started it must
	// honor the caller's context deadline.
	sender := senderFunc(func(_ context.Context, _ []slog.Record) error { return nil })
	p := NewPipeline(sender, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := p.Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain before Run should time out; got err=%v", err)
	}
}

func TestPipeline_Submit_AfterDrain_Drops(t *testing.T) {
	sender := newRecordingSender()
	p := NewPipeline(sender, Options{
		BatchSize:     10,
		FlushInterval: time.Hour,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	drainCtx, drainCancel := context.WithTimeout(context.Background(), time.Second)
	defer drainCancel()
	if err := p.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	p.Submit(errorRecord("after-drain-1"))
	p.Submit(errorRecord("after-drain-2"))

	if got := p.Dropped(); got != 2 {
		t.Fatalf("want 2 dropped after Drain, got %d", got)
	}
}

// waitForCount polls v until it reaches >= want or timeout elapses.
func waitForCount(v *atomic.Int32, want int32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if v.Load() >= want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return v.Load() >= want
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
