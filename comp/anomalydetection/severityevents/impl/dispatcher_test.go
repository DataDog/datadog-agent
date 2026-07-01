// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityeventsimpl

import (
	"testing"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

type collectingListener struct {
	events []severityeventsdef.SeverityEvent
}

func (l *collectingListener) OnSeverityTransition(evt severityeventsdef.SeverityEvent) {
	l.events = append(l.events, evt)
}

func TestDispatcherBasic(t *testing.T) {
	d := NewDispatcher()
	l := &collectingListener{}
	d.SubscribeScorer(severityeventsdef.AnomalyScorerConfiguration{Listener: l})

	d.Advance(1000, severityeventsdef.SeverityLow)  // seed
	d.Advance(1001, severityeventsdef.SeverityHigh) // escalate

	if len(l.events) != 1 {
		t.Fatalf("expected 1 escalation event, got %d: %v", len(l.events), l.events)
	}
	evt := l.events[0]
	if evt.FromLevel != severityeventsdef.SeverityLow || evt.ToLevel != severityeventsdef.SeverityHigh {
		t.Fatalf("wrong levels: from=%v to=%v", evt.FromLevel, evt.ToLevel)
	}
	if evt.Direction != severityeventsdef.AnomalyScorerEventEscalation {
		t.Fatalf("expected escalation direction, got %v", evt.Direction)
	}
}

func TestDispatcherCooldown(t *testing.T) {
	d := NewDispatcher()
	l := &collectingListener{}
	d.SubscribeScorer(severityeventsdef.AnomalyScorerConfiguration{
		Listener:     l,
		CooldownSecs: 60,
	})

	d.Advance(1000, severityeventsdef.SeverityLow)  // seed
	d.Advance(1001, severityeventsdef.SeverityHigh) // escalate
	d.Advance(1002, severityeventsdef.SeverityLow)  // blocked de-escalation

	escalations, deescalations := 0, 0
	for _, e := range l.events {
		if e.Direction == severityeventsdef.AnomalyScorerEventEscalation {
			escalations++
		} else {
			deescalations++
		}
	}
	if escalations != 1 {
		t.Fatalf("expected 1 escalation, got %d", escalations)
	}
	if deescalations != 0 {
		t.Fatalf("expected 0 de-escalations within cooldown, got %d", deescalations)
	}

	d.Advance(1062, severityeventsdef.SeverityLow)

	deescalations = 0
	for _, e := range l.events {
		if e.Direction == severityeventsdef.AnomalyScorerEventDeescalation {
			deescalations++
		}
	}
	if deescalations != 1 {
		t.Fatalf("expected 1 de-escalation after cooldown, got %d", deescalations)
	}
}

func TestDispatcherFilter(t *testing.T) {
	d := NewDispatcher()
	l := &collectingListener{}
	d.SubscribeScorer(severityeventsdef.AnomalyScorerConfiguration{
		Listener: l,
		Filter: severityeventsdef.AnomalyScorerEventFilter{
			Direction: severityeventsdef.AnomalyScorerEventEscalation,
		},
	})

	d.Advance(1000, severityeventsdef.SeverityLow)
	d.Advance(1001, severityeventsdef.SeverityHigh)
	d.Advance(1002, severityeventsdef.SeverityLow)

	if len(l.events) != 1 {
		t.Fatalf("expected only the escalation to pass the filter, got %d: %v", len(l.events), l.events)
	}
	if l.events[0].Direction != severityeventsdef.AnomalyScorerEventEscalation {
		t.Fatalf("expected delivered event to be escalation, got %v", l.events[0].Direction)
	}
}

func TestDispatcherUnsubscribe(t *testing.T) {
	d := NewDispatcher()
	l := &collectingListener{}
	unsub := d.SubscribeScorer(severityeventsdef.AnomalyScorerConfiguration{Listener: l})

	d.Advance(1000, severityeventsdef.SeverityLow)
	d.Advance(1001, severityeventsdef.SeverityHigh)
	unsub()
	d.Advance(1002, severityeventsdef.SeverityLow)

	if len(l.events) != 1 {
		t.Fatalf("expected only one event before unsubscribe, got %d: %v", len(l.events), l.events)
	}
}

func TestDispatcherResetClearsSubscriptionState(t *testing.T) {
	d := NewDispatcher()
	l := &collectingListener{}
	d.SubscribeScorer(severityeventsdef.AnomalyScorerConfiguration{
		Listener:     l,
		CooldownSecs: 3600,
	})

	d.Advance(1000, severityeventsdef.SeverityLow)
	d.Advance(1001, severityeventsdef.SeverityHigh)

	before := len(l.events)
	d.Reset()
	d.Advance(2000, severityeventsdef.SeverityLow)  // seed again
	d.Advance(2001, severityeventsdef.SeverityHigh) // escalate again

	if len(l.events)-before != 1 {
		t.Fatalf("expected one new event after reset, got %d", len(l.events)-before)
	}
	if l.events[len(l.events)-1].Direction != severityeventsdef.AnomalyScorerEventEscalation {
		t.Fatalf("expected post-reset event to be an escalation, got %v", l.events[len(l.events)-1].Direction)
	}
}

func TestDispatcherNilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil listener")
		}
	}()
	NewDispatcher().SubscribeScorer(severityeventsdef.AnomalyScorerConfiguration{})
}
