// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type progressTestSink struct{ ch chan *message.Payload }

func (s *progressTestSink) Channel() chan *message.Payload { return s.ch }

type progressTestRegistry struct {
	auditor.Registry
	offsets map[string]string
}

func (r *progressTestRegistry) SetOffset(id, offset string) { r.offsets[id] = offset }
func (r *progressTestRegistry) GetOffset(id string) string  { return r.offsets[id] }

func progressMessage(id, offset string) *message.Message {
	m := &message.Message{}
	m.Origin = message.NewOrigin(sources.NewLogSource("test", &config.LogsConfig{}))
	m.Origin.Identifier = id
	m.Origin.Offset = offset
	return m
}

func terminalPayload(m *message.Message) *message.Payload {
	meta := m.MessageMetadata
	return message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0)
}

func nextCheckpoint(t *testing.T, ch chan *message.Payload) string {
	t.Helper()
	select {
	case payload := <-ch:
		return payload.MessageMetas[0].Origin.Offset
	case <-time.After(time.Second):
		t.Fatal("checkpoint did not arrive")
		return ""
	}
}

func TestProgressWaitsForEveryGroupAndSafePrefix(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 8)}
	p, err := NewProgress(3, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first, second := progressMessage("file", "10"), progressMessage("file", "20")
	selection := Selection{Groups: []string{"a", "b"}, Generation: 1}
	for _, m := range []*message.Message{first, second} {
		if _, err := p.Register(context.Background(), m, selection, selection.Groups); err != nil {
			t.Fatal(err)
		}
	}
	p.CompleteGroup("a", terminalPayload(second))
	p.CompleteGroup("b", terminalPayload(second))
	p.CompleteGroup("a", terminalPayload(first))
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("checkpoint advanced early: %s", checkpoint.MessageMetas[0].Origin.Offset)
	default:
	}
	p.CompleteGroup("b", terminalPayload(first))
	if got := nextCheckpoint(t, sink.ch); got != "10" {
		t.Fatal(got)
	}
	if got := nextCheckpoint(t, sink.ch); got != "20" {
		t.Fatal(got)
	}
	p.CompleteGroup("b", terminalPayload(first))
	if p.Pending() != 0 || len(sink.ch) != 0 {
		t.Fatal("duplicate completion changed progress")
	}
}

func TestProgressGenerationAndBoundedAdmission(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 8)}
	p, err := NewProgress(1, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	old := progressMessage("file", "10")
	if _, err := p.Register(context.Background(), old, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := p.Register(ctx, progressMessage("other", "1"), Selection{Generation: 1}, []string{"a"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("full capacity should not take ownership: %v", err)
	}
	p.CompleteGroup("a", terminalPayload(old))
	if got := nextCheckpoint(t, sink.ch); got != "10" {
		t.Fatal(got)
	}
	if err := p.RetireSource("file", 1); err != nil {
		t.Fatal(err)
	}
	newer := progressMessage("file", "20")
	if _, err := p.Register(context.Background(), newer, Selection{Generation: 2}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(newer))
	if got := nextCheckpoint(t, sink.ch); got != "20" {
		t.Fatal(got)
	}
}

func TestInvalidSelectionHoldsLaterCheckpoint(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 4)}
	p, err := NewProgress(3, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	invalid := progressMessage("file", "10")
	if err := p.HoldInvalid(context.Background(), invalid); err != nil {
		t.Fatal(err)
	}
	later := progressMessage("file", "20")
	if _, err := p.Register(context.Background(), later, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(later))
	if p.Pending() != 2 {
		t.Fatalf("invalid record and later record must retain progress, got %d", p.Pending())
	}
	select {
	case <-sink.ch:
		t.Fatal("advanced past invalid selection")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestDirectBookmarkCannotBypassPendingGroup(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 2)}
	p, err := NewProgress(2, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	registry := &progressTestRegistry{offsets: make(map[string]string)}
	guarded := p.GuardRegistry(registry)
	guarded.SetOffset("eventlog", "initial")
	if got := guarded.GetOffset("eventlog"); got != "initial" {
		t.Fatalf("initial bookmark: %q", got)
	}
	m := progressMessage("eventlog", "event-1")
	if _, err := p.Register(context.Background(), m, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	guarded.SetOffset("eventlog", "unsafe-later")
	if got := guarded.GetOffset("eventlog"); got != "initial" {
		t.Fatalf("direct write bypassed pending group: %q", got)
	}
	p.CompleteGroup("a", terminalPayload(m))
	if got := nextCheckpoint(t, sink.ch); got != "event-1" {
		t.Fatal(got)
	}
	guarded.SetOffset("eventlog", "late-unsafe")
	if got := guarded.GetOffset("eventlog"); got != "initial" {
		t.Fatalf("direct write bypassed active generation: %q", got)
	}
	if err := p.RetireSource("eventlog", 1); err != nil {
		t.Fatal(err)
	}
	guarded.SetOffset("eventlog", "after-retirement")
	if got := guarded.GetOffset("eventlog"); got != "after-retirement" {
		t.Fatalf("retired source direct write: %q", got)
	}
}

func TestReplacementGenerationIgnoresOldCompletion(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 3)}
	p, err := NewProgress(3, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	old := progressMessage("file", "old-10")
	newer := progressMessage("file", "new-1")
	if _, err := p.Register(context.Background(), old, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Register(context.Background(), newer, Selection{Generation: 2}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(newer))
	if len(sink.ch) != 0 {
		t.Fatal("new source checkpoint advanced past old pending record")
	}
	p.CompleteGroup("a", terminalPayload(old))
	if got := nextCheckpoint(t, sink.ch); got != "new-1" {
		t.Fatalf("replacement checkpoint: %q", got)
	}
	if len(sink.ch) != 0 {
		t.Fatal("obsolete generation wrote a checkpoint")
	}
}

func TestObsoleteGenerationCannotBecomeCurrentAgain(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 3)}
	p, err := NewProgress(4, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	old := progressMessage("file", "old-10")
	newer := progressMessage("file", "new-1")
	for _, entry := range []struct {
		message    *message.Message
		generation uint64
	}{{old, 42}, {newer, 7}} {
		if _, err := p.Register(context.Background(), entry.message, Selection{Generation: entry.generation}, []string{"a"}); err != nil {
			t.Fatal(err)
		}
	}
	p.CompleteGroup("a", terminalPayload(newer))
	p.CompleteGroup("a", terminalPayload(old))
	if got := nextCheckpoint(t, sink.ch); got != "new-1" {
		t.Fatalf("replacement checkpoint: %q", got)
	}
	if p.Pending() != 0 {
		t.Fatal("source queue did not drain")
	}
	if err := p.RetireSource("file", 42); err != nil {
		t.Fatal(err)
	}
	stale := progressMessage("file", "old-11")
	if _, err := p.Register(context.Background(), stale, Selection{Generation: 42}, []string{"a"}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("obsolete generation reappeared after queue drained: %v", err)
	}
	if err := p.HoldInvalid(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	if len(sink.ch) != 0 || p.Pending() != 1 {
		t.Fatal("stale source record changed the replacement checkpoint")
	}
	if err := p.RetireSource("file", 7); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Register(context.Background(), progressMessage("file", "old-12"), Selection{Generation: 42}, []string{"a"}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("invalid barrier released obsolete generation: %v", err)
	}
}

func TestGenerationOwnershipIsBoundedUntilSourceRetires(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 3)}
	p, err := NewProgress(2, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, id := range []string{"a", "b"} {
		m := progressMessage(id, "1")
		if _, err := p.Register(context.Background(), m, Selection{Generation: 1}, []string{"g"}); err != nil {
			t.Fatal(err)
		}
		if err := p.RetireSource(id, 1); !errors.Is(err, ErrSourcePending) {
			t.Fatalf("retired pending source %q: %v", id, err)
		}
		p.CompleteGroup("g", terminalPayload(m))
		if got := nextCheckpoint(t, sink.ch); got != "1" {
			t.Fatalf("%s checkpoint: %q", id, got)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := p.Register(ctx, progressMessage("c", "1"), Selection{Generation: 1}, []string{"g"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ownership capacity did not backpressure new source: %v", err)
	}
	if err := p.RetireSource("a", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Register(context.Background(), progressMessage("c", "1"), Selection{Generation: 1}, []string{"g"}); err != nil {
		t.Fatalf("retirement did not release ownership capacity: %v", err)
	}
}

func TestGenerationHistoryIsBoundedUntilSourceRetires(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 3)}
	p, err := NewProgress(2, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for generation := uint64(1); generation <= 2; generation++ {
		m := progressMessage("file", "1")
		if _, err := p.Register(context.Background(), m, Selection{Generation: generation}, []string{"g"}); err != nil {
			t.Fatal(err)
		}
		p.CompleteGroup("g", terminalPayload(m))
		if got := nextCheckpoint(t, sink.ch); got != "1" {
			t.Fatalf("generation %d checkpoint: %q", generation, got)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := p.Register(ctx, progressMessage("file", "2"), Selection{Generation: 3}, []string{"g"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("generation history did not backpressure new generation: %v", err)
	}
	if err := p.RetireSource("file", 1); err != nil {
		t.Fatal(err)
	}
	ctxAgain, cancelAgain := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelAgain()
	if _, err := p.Register(ctxAgain, progressMessage("file", "2"), Selection{Generation: 3}, []string{"g"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retiring only old generation released current ownership: %v", err)
	}
	if err := p.RetireSource("file", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Register(context.Background(), progressMessage("file", "2"), Selection{Generation: 3}, []string{"g"}); err != nil {
		t.Fatalf("retirement did not release generation history: %v", err)
	}
}

func TestIndependentGroupKeepsPreviouslyPersistedPrefix(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 3)}
	p, err := NewProgress(3, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	// A prior persisted offset of 5 remains usable for replay. An unfinished B
	// record at 10 must hold the checkpoint even if a later A record completes.
	first := progressMessage("file", "10")
	later := progressMessage("file", "20")
	if _, err := p.Register(context.Background(), first, Selection{Generation: 1}, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Register(context.Background(), later, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(later))
	if p.Pending() != 2 || len(sink.ch) != 0 {
		t.Fatal("later A completion crossed pending B")
	}
	p.CompleteGroup("b", terminalPayload(first))
	if got := nextCheckpoint(t, sink.ch); got != "10" {
		t.Fatal(got)
	}
	if got := nextCheckpoint(t, sink.ch); got != "20" {
		t.Fatal(got)
	}
}

func TestProgressCloseReleasesBlockedSink(t *testing.T) {
	p, err := NewProgress(1, &progressTestSink{}) // No auditor consumer yet.
	if err != nil {
		t.Fatal(err)
	}
	m := progressMessage("file", "1")
	if _, err := p.Register(context.Background(), m, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(m))
	done := make(chan struct{})
	go func() { p.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close hung behind a nil auditor sink")
	}
}

func TestSourceBudgetWakesOnlyAfterSafePrefixRelease(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, SourcePendingLimit+2)}
	p, err := NewProgress(SourcePendingLimit+2, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	x := make([]*message.Message, SourcePendingLimit)
	for i := range x {
		x[i] = progressMessage("x", "1")
		if _, err := p.Register(context.Background(), x[i], Selection{Generation: 1}, []string{"a", "b"}); err != nil {
			t.Fatal(err)
		}
	}
	blocked := progressMessage("x", "65")
	registered := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		_, err := p.Register(ctx, blocked, Selection{Generation: 1}, []string{"a", "b"})
		registered <- err
	}()
	select {
	case err := <-registered:
		t.Fatalf("X passed its source budget: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	y := progressMessage("y", "1")
	if _, err := p.Register(context.Background(), y, Selection{Generation: 1}, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("b", terminalPayload(y))
	if got := nextCheckpoint(t, sink.ch); got != "1" {
		t.Fatal(got)
	}
	p.CompleteGroup("b", terminalPayload(x[0]))
	select {
	case err := <-registered:
		t.Fatalf("B-only completion freed X before A: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	p.CompleteGroup("a", terminalPayload(x[0]))
	if got := nextCheckpoint(t, sink.ch); got != "1" {
		t.Fatal(got)
	}
	select {
	case err := <-registered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("X did not resume after its safe prefix advanced")
	}
}

func TestAggregateBudgetBackpressuresOtherSources(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 3)}
	p, err := NewProgress(3, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	x, y, xLater := progressMessage("x", "1"), progressMessage("y", "1"), progressMessage("x", "2")
	for _, m := range []*message.Message{x, y, xLater} {
		if _, err := p.Register(context.Background(), m, Selection{Generation: 1}, []string{"b"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Register(ctx, progressMessage("z", "1"), Selection{Generation: 1}, []string{"b"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("aggregate exhaustion did not backpressure Z: %v", err)
	}
	p.CompleteGroup("b", terminalPayload(x))
	if got := nextCheckpoint(t, sink.ch); got != "1" {
		t.Fatal(got)
	}
	if _, err := p.Register(context.Background(), progressMessage("z", "1"), Selection{Generation: 1}, []string{"b"}); err != nil {
		t.Fatalf("aggregate capacity did not wake Z: %v", err)
	}
}
