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

func newTestDispatcher(listener severityeventsdef.SeverityEventListener, cfg severityeventsdef.SeverityEventsConfiguration) *Dispatcher {
	return NewDispatcher(cfg, listener)
}

func TestDispatcherBasic(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{})

	d.Advance(1000, severityeventsdef.SeverityLow)
	d.Advance(1001, severityeventsdef.SeverityHigh) // escalate

	if len(l.events) != 1 {
		t.Fatalf("expected 1 escalation event, got %d: %v", len(l.events), l.events)
	}
	evt := l.events[0]
	if evt.FromLevel != severityeventsdef.SeverityLow || evt.ToLevel != severityeventsdef.SeverityHigh {
		t.Fatalf("wrong levels: from=%v to=%v", evt.FromLevel, evt.ToLevel)
	}
	if evt.Direction != severityeventsdef.SeverityEventEscalation {
		t.Fatalf("expected escalation direction, got %v", evt.Direction)
	}
}

func TestDispatcherCooldown(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 60,
	})

	d.Advance(1000, severityeventsdef.SeverityLow)
	d.Advance(1001, severityeventsdef.SeverityHigh) // escalate
	d.Advance(1002, severityeventsdef.SeverityLow)  // blocked de-escalation

	escalations, deescalations := 0, 0
	for _, e := range l.events {
		if e.Direction == severityeventsdef.SeverityEventEscalation {
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
		if e.Direction == severityeventsdef.SeverityEventDeescalation {
			deescalations++
		}
	}
	if deescalations != 1 {
		t.Fatalf("expected 1 de-escalation after cooldown, got %d", deescalations)
	}
}

func TestDispatcherFilter(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{
		Filter: severityeventsdef.SeverityEventFilter{
			Direction: severityeventsdef.SeverityEventEscalation,
		},
	})

	d.Advance(1000, severityeventsdef.SeverityLow)
	d.Advance(1001, severityeventsdef.SeverityHigh)
	d.Advance(1002, severityeventsdef.SeverityLow)

	if len(l.events) != 1 {
		t.Fatalf("expected only the escalation to pass the filter, got %d: %v", len(l.events), l.events)
	}
	if l.events[0].Direction != severityeventsdef.SeverityEventEscalation {
		t.Fatalf("expected delivered event to be escalation, got %v", l.events[0].Direction)
	}
}

func TestDispatcherResetClearsSubscriptionState(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 3600,
	})

	d.Advance(1000, severityeventsdef.SeverityLow)
	d.Advance(1001, severityeventsdef.SeverityHigh)

	before := len(l.events)
	d.Reset()
	d.Advance(2000, severityeventsdef.SeverityLow)
	d.Advance(2001, severityeventsdef.SeverityHigh) // escalate again

	if len(l.events)-before != 1 {
		t.Fatalf("expected one new event after reset, got %d", len(l.events)-before)
	}
	if l.events[len(l.events)-1].Direction != severityeventsdef.SeverityEventEscalation {
		t.Fatalf("expected post-reset event to be an escalation, got %v", l.events[len(l.events)-1].Direction)
	}
}

func TestDispatcherDeliverInitialLowEmitsNothing(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{})
	d.DeliverInitial(1000, severityeventsdef.SeverityLow)

	if len(l.events) != 0 {
		t.Fatalf("expected no initial event when current level is Low, got %v", l.events)
	}
}

func TestDispatcherDefaultLowEmitsFirstNonLowTransition(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{})

	if len(l.events) != 0 {
		t.Fatalf("expected no initial event before Advance or DeliverInitial, got %v", l.events)
	}

	d.Advance(1000, severityeventsdef.SeverityHigh)
	if len(l.events) != 1 {
		t.Fatalf("expected first non-low level to emit one event, got %d: %v", len(l.events), l.events)
	}
	evt := l.events[0]
	if evt.FromLevel != severityeventsdef.SeverityLow || evt.ToLevel != severityeventsdef.SeverityHigh {
		t.Fatalf("expected the first event to be Low->High, got from=%v to=%v", evt.FromLevel, evt.ToLevel)
	}
}

func TestDispatcherDeliverInitialBootstrapsFromLow(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{})
	d.DeliverInitial(1001, severityeventsdef.SeverityHigh)

	if len(l.events) != 1 {
		t.Fatalf("expected exactly one initial event, got %d: %v", len(l.events), l.events)
	}
	evt := l.events[0]
	if evt.FromLevel != severityeventsdef.SeverityLow || evt.ToLevel != severityeventsdef.SeverityHigh {
		t.Fatalf("expected initial event to bootstrap from Low to High, got from=%v to=%v", evt.FromLevel, evt.ToLevel)
	}
	if evt.Direction != severityeventsdef.SeverityEventEscalation {
		t.Fatalf("expected initial event direction to be Escalation, got %v", evt.Direction)
	}
	if evt.Timestamp != 1001 {
		t.Fatalf("expected initial event timestamp to be the last Advance second, got %d", evt.Timestamp)
	}

	// The subscriber must not also get a spurious transition on the very next
	// Advance call if the level hasn't actually changed.
	d.Advance(1002, severityeventsdef.SeverityHigh)
	if len(l.events) != 1 {
		t.Fatalf("expected no additional event for an unchanged level, got %d: %v", len(l.events), l.events)
	}
}

func TestDispatcherDeliverInitialRespectsDirectionFilter(t *testing.T) {
	escalationsOnly := &collectingListener{}
	d := newTestDispatcher(escalationsOnly, severityeventsdef.SeverityEventsConfiguration{
		Filter: severityeventsdef.SeverityEventFilter{Direction: severityeventsdef.SeverityEventEscalation},
	})
	d.DeliverInitial(1001, severityeventsdef.SeverityHigh)

	deescalationsOnly := &collectingListener{}
	d = newTestDispatcher(deescalationsOnly, severityeventsdef.SeverityEventsConfiguration{
		Filter: severityeventsdef.SeverityEventFilter{Direction: severityeventsdef.SeverityEventDeescalation},
	})
	d.DeliverInitial(1001, severityeventsdef.SeverityHigh)

	if len(escalationsOnly.events) != 1 {
		t.Fatalf("escalation-only filter should receive the initial Low->High bootstrap, got %v", escalationsOnly.events)
	}
	if len(deescalationsOnly.events) != 0 {
		t.Fatalf("de-escalation-only filter should not receive the initial escalation event, got %v", deescalationsOnly.events)
	}
}

func TestDispatcherDeliverInitialStartsCooldownWhenDelivered(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 60,
	})
	d.DeliverInitial(1001, severityeventsdef.SeverityHigh)
	if len(l.events) != 1 {
		t.Fatalf("expected 1 initial event, got %d: %v", len(l.events), l.events)
	}

	// The bootstrap Low->High event is delivered as a real escalation, so it
	// seeds cooldown for the first de-escalation.
	d.Advance(1010, severityeventsdef.SeverityLow)
	if len(l.events) != 1 {
		t.Fatalf("expected the de-escalation to be blocked by cooldown, got %d: %v", len(l.events), l.events)
	}

	d.Advance(1062, severityeventsdef.SeverityLow)
	if len(l.events) != 2 {
		t.Fatalf("expected the de-escalation to fire after cooldown, got %d: %v", len(l.events), l.events)
	}
}

func TestDispatcherFilteredInitialEventDoesNotStartCooldown(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{
		Filter:       severityeventsdef.SeverityEventFilter{Direction: severityeventsdef.SeverityEventDeescalation},
		CooldownSecs: 60,
	})
	d.DeliverInitial(1000, severityeventsdef.SeverityHigh)
	if len(l.events) != 0 {
		t.Fatalf("expected the filtered initial event to be suppressed, got %v", l.events)
	}

	// The bootstrap escalation was filtered out, so it must not seed cooldown
	// for the first delivered de-escalation.
	d.Advance(1005, severityeventsdef.SeverityLow)
	if len(l.events) != 1 {
		t.Fatalf("expected the first real de-escalation to fire immediately, got %d: %v", len(l.events), l.events)
	}
	if l.events[0].Direction != severityeventsdef.SeverityEventDeescalation {
		t.Fatalf("expected a de-escalation event, got %v", l.events[0].Direction)
	}
}

func TestDispatcherResetClearsKnownLevelForNewSubscribers(t *testing.T) {
	l := &collectingListener{}
	d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{})
	d.DeliverInitial(1001, severityeventsdef.SeverityHigh)

	d.Reset()
	d.Advance(1002, severityeventsdef.SeverityHigh)
	if len(l.events) != 2 {
		t.Fatalf("expected reset to restore Low default and emit a new escalation, got %v", l.events)
	}
	if l.events[1].FromLevel != severityeventsdef.SeverityLow || l.events[1].ToLevel != severityeventsdef.SeverityHigh {
		t.Fatalf("expected post-reset event to be Low->High, got %+v", l.events[1])
	}
}
