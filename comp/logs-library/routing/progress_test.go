// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type progressTestSink struct{ ch chan *message.Payload }

func (s *progressTestSink) Channel() chan *message.Payload { return s.ch }

type progressTestRegistry struct {
	Registry
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
	if p.Pending() != 1 {
		t.Fatalf("only the invalid barrier should retain progress, got %d", p.Pending())
	}
	select {
	case <-sink.ch:
		t.Fatal("advanced past invalid selection")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestInvalidBarriersDoNotRetainDeliveredWindows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &progressTestSink{ch: make(chan *message.Payload, 8)}
		p, err := NewProgress(8, sink)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		for _, id := range []string{"bad-a", "bad-b"} {
			if err := p.RegisterSource(id, 1); err != nil {
				t.Fatal(err)
			}
			if err := p.HoldInvalid(ctx, progressMessage(id, "invalid")); err != nil {
				t.Fatal(err)
			}
			// Exceed both the per-source and aggregate record windows.
			for range 16 {
				m := progressMessage(id, "later")
				if _, err := p.Register(ctx, m, Selection{Generation: 1}, []string{"a", "b"}); err != nil {
					t.Fatal(err)
				}
				before := p.Pending()
				p.CompleteGroup("a", terminalPayload(m))
				if p.Pending() != before {
					t.Fatal("released a record with a pending group")
				}
				p.CompleteGroup("b", terminalPayload(m))
				p.CompleteGroup("b", terminalPayload(m))
			}
			if err := p.RetireSource(id, 1); err != nil {
				t.Fatal(err)
			}
			if err := p.RegisterSource(id, 2); err != nil {
				t.Fatal(err)
			}
			m := progressMessage(id, "replacement")
			if _, err := p.Reserve(ctx, m, 2); err != nil {
				t.Fatal(err)
			}
			p.CompleteLocal(m.RoutingToken())
			if err := p.RetireSource(id, 2); err != nil || !p.OwnsSource(id) {
				t.Fatalf("barrier ownership after replacement retirement: %v", err)
			}
		}
		if p.Pending() != 2 {
			t.Fatalf("only two barriers should remain: %d", p.Pending())
		}
		synctest.Wait()
		if len(sink.ch) != 0 {
			t.Fatal("published past an invalid barrier")
		}
		healthy := progressMessage("healthy", "1")
		if _, err := p.Register(ctx, healthy, Selection{Generation: 1}, []string{"a"}); err != nil {
			t.Fatal(err)
		}
		p.CompleteGroup("a", terminalPayload(healthy))
		if got := nextCheckpoint(t, sink.ch); got != "1" {
			t.Fatal(got)
		}
	})
}

func TestInvalidationReleasesOnlyCompletedSuffix(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &progressTestSink{ch: make(chan *message.Payload, 4)}
		p, err := NewProgress(4, sink)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		var records []*message.Message
		for _, offset := range []string{"1", "2", "3", "4"} {
			m := progressMessage("file", offset)
			if _, err := p.Reserve(context.Background(), m, 1); err != nil {
				t.Fatal(err)
			}
			records = append(records, m)
		}
		p.CompleteLocal(records[1].RoutingToken())
		p.CompleteLocal(records[3].RoutingToken())
		admitted := make(chan error, 1)
		go func() {
			_, err := p.Reserve(context.Background(), progressMessage("other", "1"), 1)
			admitted <- err
		}()
		synctest.Wait()
		select {
		case err := <-admitted:
			t.Fatalf("full capacity admitted a record: %v", err)
		default:
		}
		p.Invalidate(records[2].RoutingToken())
		if err := <-admitted; err != nil {
			t.Fatal(err)
		}
		if p.Pending() != 4 {
			t.Fatalf("suffix cleanup removed unfinished or pre-barrier records: %d", p.Pending())
		}
		p.CompleteLocal(records[0].RoutingToken())
		if got := nextCheckpoint(t, sink.ch); got != "1" {
			t.Fatal(got)
		}
		if got := nextCheckpoint(t, sink.ch); got != "2" {
			t.Fatal(got)
		}
		synctest.Wait()
		if p.Pending() != 2 || len(sink.ch) != 0 {
			t.Fatal("invalid barrier failed to retain the safe checkpoint")
		}
	})
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
	// SourceInput binds the producer before the subscription saves its initial bookmark.
	if err := p.RegisterSource("eventlog", 1); err != nil {
		t.Fatal(err)
	}
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

func TestDirectBookmarkCannotBypassReplacement(t *testing.T) {
	p, err := NewProgress(2, &progressTestSink{ch: make(chan *message.Payload, 2)})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, generation := range []uint64{1, 2} {
		if err := p.RegisterSource("eventlog", generation); err != nil {
			t.Fatal(err)
		}
	}
	registry := &progressTestRegistry{offsets: map[string]string{"eventlog": "safe"}}
	guarded := p.GuardRegistry(registry)
	guarded.SetOffset("eventlog", "obsolete-initial")
	if got := guarded.GetOffset("eventlog"); got != "safe" {
		t.Fatalf("direct write bypassed replacement ownership: %q", got)
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
	old, newer := progressMessage("file", "old-10"), progressMessage("file", "new-1")
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
	// The older producer can keep draining after replacement and queue drain.
	late := progressMessage("file", "old-11")
	if _, err := p.Register(context.Background(), late, Selection{Generation: 42}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	latest := progressMessage("file", "new-2")
	if _, err := p.Register(context.Background(), latest, Selection{Generation: 7}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(latest))
	select {
	case <-sink.ch:
		t.Fatal("checkpoint crossed pending old-producer record")
	default:
	}
	p.CompleteGroup("a", terminalPayload(late))
	if got := nextCheckpoint(t, sink.ch); got != "new-2" {
		t.Fatalf("older producer reclaimed checkpoint ownership: %q", got)
	}
	if err := p.RetireSource("file", 42); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Register(context.Background(), progressMessage("file", "old-12"), Selection{Generation: 42}, []string{"a"}); !errors.Is(err, ErrObsoleteGeneration) {
		t.Fatalf("retired generation accepted: %v", err)
	}
	if len(sink.ch) != 0 || p.Pending() != 0 {
		t.Fatal("retired producer changed replacement progress")
	}
}

func TestInvalidBarrierDoesNotPreventProducerRetirement(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 1)}
	p, err := NewProgress(4, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	m := progressMessage("file", "1")
	if _, err := p.Register(context.Background(), m, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(m))
	if got := nextCheckpoint(t, sink.ch); got != "1" {
		t.Fatal(got)
	}
	if err := p.HoldInvalid(context.Background(), progressMessage("file", "2")); err != nil {
		t.Fatal(err)
	}
	if err := p.RetireSource("file", 1); err != nil {
		t.Fatalf("barrier prevented producer retirement: %v", err)
	}
	if !p.OwnsSource("file") || p.Pending() != 1 {
		t.Fatal("retirement released unresolved source barrier")
	}
	if _, err := p.Register(context.Background(), progressMessage("file", "3"), Selection{Generation: 1}, []string{"a"}); !errors.Is(err, ErrObsoleteGeneration) {
		t.Fatalf("retired producer accepted: %v", err)
	}
	if len(sink.ch) != 0 {
		t.Fatal("barrier advanced checkpoint")
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
	p, err := NewProgress(1, &progressTestSink{ch: make(chan *message.Payload)}) // No auditor consumer yet.
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

// A stalled auditor can fill the publication queue even as completed records
// free admission capacity. Cancellation must release the completion worker.
func TestProgressCancellationUnblocksFullCheckpointQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &progressTestSink{ch: make(chan *message.Payload)}
		p, err := NewProgress(1, sink)
		if err != nil {
			t.Fatal(err)
		}
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			for i := 0; i < 3; i++ {
				m := progressMessage("file", "10")
				if _, err := p.Register(context.Background(), m, Selection{Generation: 1}, []string{"a"}); err != nil {
					t.Error(err)
					return
				}
				p.CompleteGroup("a", terminalPayload(m))
			}
		}()
		synctest.Wait()
		select {
		case <-finished:
			t.Fatal("expected publication backpressure")
		default:
		}
		p.StopForwarding()
		<-finished
		p.Close()
		select {
		case <-sink.ch:
			t.Fatal("blocked auditor received a checkpoint")
		default:
		}
	})
}

func TestRetireProducerWhoseFirstRecordIsInvalid(t *testing.T) {
	p, err := NewProgress(4, &progressTestSink{ch: make(chan *message.Payload, 4)})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.RegisterSource("file", 1); err != nil {
		t.Fatal(err)
	}
	if err := p.HoldInvalid(context.Background(), progressMessage("file", "bad-1")); err != nil {
		t.Fatal(err)
	}
	if err := p.RetireSource("file", 1); err != nil {
		t.Fatalf("producer with no pending delivery cannot retire: %v", err)
	}
	if !p.OwnsSource("file") {
		t.Fatal("source barrier was released")
	}
}

func TestRetireAfterCompletedRecordBehindBarrier(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 4)}
	p, err := NewProgress(4, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first := progressMessage("file", "1")
	if _, err := p.Register(context.Background(), first, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(first))
	_ = nextCheckpoint(t, sink.ch)
	if err := p.HoldInvalid(context.Background(), progressMessage("file", "bad-2")); err != nil {
		t.Fatal(err)
	}
	third := progressMessage("file", "3")
	if _, err := p.Register(context.Background(), third, Selection{Generation: 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.CompleteGroup("a", terminalPayload(third))
	if err := p.RetireSource("file", 1); err != nil {
		t.Fatalf("producer whose delivery completed cannot retire behind source barrier: %v", err)
	}
	if !p.OwnsSource("file") {
		t.Fatal("source barrier was released")
	}
}

func TestRegisterSourceReservesOwnershipAndHistoryBeforeRecords(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 2)}
	p, err := NewProgress(2, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, gen := range []uint64{42, 7} {
		if err := p.RegisterSource("file", gen); err != nil {
			t.Fatal(err)
		}
	}
	old := progressMessage("file", "old")
	if _, err := p.Register(context.Background(), old, Selection{Generation: 42, Excluded: true}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.ch:
		t.Fatal("older producer reclaimed current ownership on its first record")
	default:
	}
	current := progressMessage("file", "new")
	if _, err := p.Register(context.Background(), current, Selection{Generation: 7, Excluded: true}, nil); err != nil {
		t.Fatal(err)
	}
	if got := nextCheckpoint(t, sink.ch); got != "new" {
		t.Fatal(got)
	}
	if err := p.RegisterSource("other", 1); !errors.Is(err, ErrSourceCapacity) {
		t.Fatalf("ownership reservation exceeded history limit: %v", err)
	}
	if err := p.RetireSource("file", 42); err != nil {
		t.Fatal(err)
	}
	if err := p.RetireSource("file", 7); err != nil {
		t.Fatal(err)
	}
	if err := p.RegisterSource("other", 1); err != nil {
		t.Fatalf("retirement did not release capacity: %v", err)
	}
}

func TestProgressReservedSelectionOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		groups   []string
		excluded bool
	}{
		{"a", []string{"a"}, false}, {"b", []string{"b"}, false},
		{"both", []string{"a", "b"}, false}, {"repeated", []string{"a", "b", "a"}, false},
		{"excluded", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &progressTestSink{ch: make(chan *message.Payload, 2)}
			p, err := NewProgress(2, sink)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			first, last := progressMessage("file:test", "1"), progressMessage("file:test", "2")
			token, err := p.Reserve(context.Background(), first, 1)
			if err != nil {
				t.Fatal(err)
			}
			// A later local outcome cannot pass the pending selection.
			if _, err := p.Register(context.Background(), last, Selection{Generation: 1, Excluded: true}, nil); err != nil {
				t.Fatal(err)
			}
			if p.Pending() != 2 || len(sink.ch) != 0 {
				t.Fatal("pending selection released source order")
			}
			// Delivery notifications cannot complete an unselected reservation.
			p.CompleteGroup("a", terminalPayload(first))
			if p.Pending() != 2 {
				t.Fatal("unselected record completed")
			}
			if err := p.Attach(token, Selection{Excluded: tc.excluded}, tc.groups); err != nil {
				t.Fatal(err)
			}
			if err := p.Attach(token, Selection{Groups: []string{"other"}}, []string{"other"}); !errors.Is(err, ErrInvalidSelection) {
				t.Fatalf("second attachment = %v", err)
			}
			seen := map[string]bool{}
			for _, group := range tc.groups {
				if seen[group] {
					continue
				}
				seen[group] = true
				p.CompleteGroup(group, terminalPayload(first))
				if tc.name == "both" && group == "a" && p.Pending() != 2 {
					t.Fatal("one group completed both obligations")
				}
			}
			if got := nextCheckpoint(t, sink.ch); got != "1" {
				t.Fatalf("first checkpoint = %s", got)
			}
			if got := nextCheckpoint(t, sink.ch); got != "2" {
				t.Fatalf("second checkpoint = %s", got)
			}
			if p.Pending() != 0 {
				t.Fatal("completed reservations retained")
			}
		})
	}
}

func TestProgressInvalidatesSameReservationAndReleasesProducer(t *testing.T) {
	sink := &progressTestSink{ch: make(chan *message.Payload, 2)}
	p, err := NewProgress(2, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first := progressMessage("file:test", "1")
	token, err := p.Reserve(context.Background(), first, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.RetireSource("file:test", 1); !errors.Is(err, ErrSourcePending) {
		t.Fatalf("retiring pending selection = %v", err)
	}
	if err := p.Attach(token, Selection{Generation: 2, Excluded: true}, nil); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("identity mismatch = %v", err)
	}
	p.Invalidate(token)
	p.Invalidate(token)
	if p.Pending() != 1 || p.records[token].generation != 0 || !p.OwnsSource("file:test") {
		t.Fatal("invalid selection did not reuse the source reservation")
	}
	if err := p.Attach(token, Selection{Excluded: true}, nil); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("attached invalid barrier = %v", err)
	}
	if err := p.RetireSource("file:test", 1); err != nil {
		t.Fatal(err)
	}
	if !p.OwnsSource("file:test") {
		t.Fatal("retirement removed invalid barrier")
	}
	later := progressMessage("file:test", "2")
	if _, err := p.Reserve(context.Background(), later, 2); err != nil {
		t.Fatal(err)
	}
	p.CompleteLocal(later.RoutingToken())
	if p.Pending() != 1 || len(sink.ch) != 0 {
		t.Fatal("later completion bypassed invalid selection")
	}
}

func TestProgressPendingSelectionBoundsAdmissionAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &progressTestSink{ch: make(chan *message.Payload, 2)}
		p, err := NewProgress(1, sink)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		first := progressMessage("file:test", "1")
		token, err := p.Reserve(context.Background(), first, 1)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		reserved := make(chan error, 1)
		go func() { _, err := p.Reserve(ctx, progressMessage("file:other", "2"), 1); reserved <- err }()
		synctest.Wait()
		select {
		case err := <-reserved:
			t.Fatalf("pending selection did not consume capacity: %v", err)
		default:
		}
		cancel()
		if err := <-reserved; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		p.Invalidate(token)
		if p.Pending() != 1 {
			t.Fatal("invalid selection changed capacity usage")
		}
	})
}

func TestProgressAnonymousSelectionFailureReleasesCapacity(t *testing.T) {
	p, err := NewProgress(1, &progressTestSink{ch: make(chan *message.Payload, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	token, err := p.Reserve(context.Background(), progressMessage("", ""), 0)
	if err != nil {
		t.Fatal(err)
	}
	p.Invalidate(token)
	if p.Pending() != 0 {
		t.Fatal("anonymous failure retained a checkpoint barrier")
	}
}

func TestProgressSelectionCompletionUnblocksCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &progressTestSink{ch: make(chan *message.Payload, 1)}
		p, err := NewProgress(1, sink)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		first := progressMessage("file:test", "1")
		token, err := p.Reserve(context.Background(), first, 1)
		if err != nil {
			t.Fatal(err)
		}
		reserved := make(chan error, 1)
		later := progressMessage("file:test", "2")
		go func() { _, err := p.Reserve(context.Background(), later, 1); reserved <- err }()
		synctest.Wait()
		select {
		case err := <-reserved:
			t.Fatalf("pending selection did not block: %v", err)
		default:
		}
		if err := p.Attach(token, Selection{Excluded: true}, nil); err != nil {
			t.Fatal(err)
		}
		if err := <-reserved; err != nil {
			t.Fatal(err)
		}
		if p.Pending() != 1 {
			t.Fatal("resumed admission did not own capacity")
		}
		p.CompleteLocal(later.RoutingToken())
		if got := nextCheckpoint(t, sink.ch); got != "1" {
			t.Fatalf("first checkpoint = %s", got)
		}
		if got := nextCheckpoint(t, sink.ch); got != "2" {
			t.Fatalf("second checkpoint = %s", got)
		}
	})
}

func TestProgressNilSinkDoesNotBlockCompletion(t *testing.T) {
	for _, tc := range []struct {
		name string
		sink sender.Sink
	}{
		{"noop sink", &sender.NoopSink{}},
		{"custom nil channel", &progressTestSink{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p, err := NewProgress(1, tc.sink)
				if err != nil {
					t.Fatal(err)
				}
				defer p.Close()
				finished := make(chan struct{})
				go func() {
					defer close(finished)
					// Exceed both the publication queue and the single in-flight forward.
					for i := 0; i < 4; i++ {
						m := progressMessage("file:test", "1")
						token, err := p.Reserve(context.Background(), m, 1)
						if err != nil {
							t.Error(err)
							return
						}
						if err := p.Attach(token, Selection{}, []string{"a", "b"}); err != nil {
							t.Error(err)
							return
						}
						p.CompleteGroup("a", terminalPayload(m))
						if p.Pending() != 1 {
							t.Error("no-op sink bypassed the pending group")
							return
						}
						p.CompleteGroup("b", terminalPayload(m))
					}
				}()
				synctest.Wait()
				select {
				case <-finished:
				default:
					t.Error("nil sink blocked completed records after publication queue filled")
				}
				// Release a failing implementation so test cleanup cannot deadlock.
				p.StopForwarding()
				<-finished
				if p.Pending() != 0 {
					t.Fatal("completed records retained progress capacity")
				}
				if err := p.RetireSource("file:test", 1); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func BenchmarkProgressReserveAttachComplete(b *testing.B) {
	for _, excluded := range []bool{false, true} {
		name := "two groups"
		if excluded {
			name = "excluded"
		}
		b.Run(name, func(b *testing.B) {
			p, err := NewProgress(128, &sender.NoopSink{})
			if err != nil {
				b.Fatal(err)
			}
			defer p.Close()
			m := progressMessage("file:benchmark", "1")
			payload := message.NewPayload([]*message.MessageMetadata{&m.MessageMetadata}, nil, "", 0)
			groups := []string{"a", "b"}
			selection := Selection{Excluded: excluded}
			if excluded {
				groups = nil
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				token, err := p.Reserve(context.Background(), m, 1)
				if err != nil {
					b.Fatal(err)
				}
				if err := p.Attach(token, selection, groups); err != nil {
					b.Fatal(err)
				}
				if !excluded {
					p.CompleteGroup("a", payload)
					p.CompleteGroup("b", payload)
				}
			}
			b.StopTimer()
		})
	}
}

func TestProgressWakesAllWaitingAdmissions(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		name := "capacity released"
		if stopped {
			name = "stopped"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p, err := NewProgress(1, &sender.NoopSink{})
				if err != nil {
					t.Fatal(err)
				}
				defer p.Close()
				first := progressMessage("file:test", "1")
				token, err := p.Reserve(context.Background(), first, 1)
				if err != nil {
					t.Fatal(err)
				}
				finished := make(chan error, 2)
				for range 2 {
					go func() {
						token, err := p.Reserve(context.Background(), progressMessage("file:test", "later"), 1)
						if err == nil {
							p.CompleteLocal(token)
						}
						finished <- err
					}()
				}
				synctest.Wait()
				select {
				case err := <-finished:
					t.Fatalf("admission passed full capacity: %v", err)
				default:
				}
				if stopped {
					p.Close()
				} else {
					p.CompleteLocal(token)
				}
				for range 2 {
					err := <-finished
					if stopped {
						if !errors.Is(err, ErrStopped) {
							t.Fatalf("stopped admission = %v", err)
						}
					} else if err != nil {
						t.Fatal(err)
					}
				}
				if p.Pending() != 0 {
					t.Fatal("waiting admissions retained progress capacity")
				}
			})
		})
	}
}

func TestTryRetireSourcePreservesOwnershipWhenBusy(t *testing.T) {
	p, err := NewProgress(2, &sender.NoopSink{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.RegisterSource("file", 1); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	_, err = p.TryRetireSource("file", 1)
	p.mu.Unlock()
	if !errors.Is(err, ErrSourcePending) {
		t.Fatalf("busy retirement = %v", err)
	}
	if !p.OwnsSource("file") {
		t.Fatal("busy retirement released ownership")
	}
	// Registering a replacement must not let retirement release its ownership.
	if err := p.RegisterSource("file", 2); err != nil {
		t.Fatal(err)
	}
	if owned, err := p.TryRetireSource("file", 1); err != nil || !owned {
		t.Fatalf("old producer retirement: owned=%v err=%v", owned, err)
	}
	if owned, err := p.TryRetireSource("file", 2); err != nil || owned {
		t.Fatalf("last producer retirement: owned=%v err=%v", owned, err)
	}
}

func TestPendingGroupsOnlyIncludesUnresolvedSourceRecipients(t *testing.T) {
	p, err := NewProgress(8, &progressTestSink{ch: make(chan *message.Payload, 8)})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	register := func(identifier, offset string, groups []string) *message.Message {
		m := progressMessage(identifier, offset)
		if _, err := p.Register(context.Background(), m, Selection{Generation: 1}, groups); err != nil {
			t.Fatal(err)
		}
		return m
	}
	first := register("file:test", "1", []string{"a", "b"})
	if _, err := p.Reserve(context.Background(), progressMessage("file:test", "2"), 1); err != nil {
		t.Fatal(err)
	}
	if err := p.HoldInvalid(context.Background(), progressMessage("file:test", "3")); err != nil {
		t.Fatal(err)
	}
	completed := register("file:test", "4", []string{"d"})
	p.CompleteGroup("d", terminalPayload(completed)) // Released behind the invalid barrier.
	register("file:other", "1", []string{"c"})
	register("", "", []string{"anonymous"})
	p.CompleteGroup("a", terminalPayload(first))
	if groups := p.PendingGroups("file:test"); len(groups) != 1 || groups[0] != "b" {
		t.Fatalf("pending source groups=%v, want [b]", groups)
	}
	p.CompleteGroup("b", terminalPayload(first))
	if groups := p.PendingGroups("file:test"); len(groups) != 0 {
		t.Fatalf("pending selection, invalid barrier or completed record inferred recipients: %v", groups)
	}
	if groups := p.PendingGroups("file:other"); len(groups) != 1 || groups[0] != "c" {
		t.Fatalf("other source groups=%v, want [c]", groups)
	}
	if groups := p.PendingGroups(""); len(groups) != 1 || groups[0] != "anonymous" {
		t.Fatalf("anonymous groups=%v, want [anonymous]", groups)
	}
}
