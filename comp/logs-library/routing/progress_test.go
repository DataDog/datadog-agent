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
