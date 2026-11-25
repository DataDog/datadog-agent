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

func TestDispatcherAdvanceWithDiscovery(t *testing.T) {
	t.Run("activates historical medium and holds it", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{CooldownSecs: 3})

		d.AdvanceWithDiscovery(1005, severityeventsdef.SeverityLow, severityeventsdef.SeverityMedium, 1000)
		if len(l.events) != 1 {
			t.Fatalf("expected 1 Medium discovery event, got %v", l.events)
		}
		if l.events[0].ToLevel != severityeventsdef.SeverityMedium {
			t.Fatalf("expected Medium, got %v", l.events[0].ToLevel)
		}
		if l.events[0].Timestamp != 1005 || l.events[0].HistoricalTimestamp != 1000 {
			t.Fatalf("expected trigger 1005 and crossing 1000, got %+v", l.events[0])
		}

		d.Advance(1006, severityeventsdef.SeverityLow)
		if len(l.events) != 1 {
			t.Fatalf("the discovered Medium hold must block Low, got %v", l.events)
		}

		d.Advance(1008, severityeventsdef.SeverityLow)
		if len(l.events) != 2 {
			t.Fatalf("Medium must fall back to Low after cooldown, got %v", l.events)
		}
	})

	t.Run("distinct medium renews hold without another event", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{CooldownSecs: 5})

		d.AdvanceWithDiscovery(1000, severityeventsdef.SeverityMedium, severityeventsdef.SeverityMedium, 1000)
		d.AdvanceWithDiscovery(1003, severityeventsdef.SeverityLow, severityeventsdef.SeverityMedium, 1003)
		if len(l.events) != 1 {
			t.Fatalf("expected no transition for the renewed Medium hold, got %v", l.events)
		}

		d.Advance(1007, severityeventsdef.SeverityLow)
		if len(l.events) != 1 {
			t.Fatalf("the renewed Medium hold must still block Low, got %v", l.events)
		}
		d.Advance(1008, severityeventsdef.SeverityLow)
		if len(l.events) != 2 {
			t.Fatalf("expected Medium to fall back to Low, got %v", l.events)
		}
	})

	t.Run("continuous medium does not renew hold", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{CooldownSecs: 3})

		d.Advance(1000, severityeventsdef.SeverityMedium)
		d.Advance(1002, severityeventsdef.SeverityMedium)
		d.Advance(1003, severityeventsdef.SeverityLow)
		if len(l.events) != 2 || l.events[1].ToLevel != severityeventsdef.SeverityLow {
			t.Fatalf("continuous Medium must not renew its hold, got %v", l.events)
		}
	})

	t.Run("high preempts medium atomically", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{})

		d.AdvanceWithDiscovery(1000, severityeventsdef.SeverityMedium, severityeventsdef.SeverityHigh, 999)
		if len(l.events) != 1 || l.events[0].ToLevel != severityeventsdef.SeverityHigh || l.events[0].HistoricalTimestamp != 999 {
			t.Fatalf("expected one atomic High discovery, got %v", l.events)
		}
	})

	t.Run("medium does not affect high hold", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{CooldownSecs: 5})

		d.Advance(1000, severityeventsdef.SeverityHigh)
		d.AdvanceWithDiscovery(1001, severityeventsdef.SeverityMedium, severityeventsdef.SeverityMedium, 1000)
		if len(l.events) != 1 {
			t.Fatalf("the Medium discovery must not affect the High hold, got %v", l.events)
		}

		d.Advance(1004, severityeventsdef.SeverityLow)
		if len(l.events) != 1 {
			t.Fatalf("the Medium discovery must not end the High hold early, got %v", l.events)
		}
		d.Advance(1005, severityeventsdef.SeverityLow)
		if len(l.events) != 2 || l.events[1].ToLevel != severityeventsdef.SeverityLow {
			t.Fatalf("expected High to expire on its own cooldown, got %v", l.events)
		}
	})

	t.Run("medium starts after high cooldown expires", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{CooldownSecs: 3})

		d.Advance(1000, severityeventsdef.SeverityHigh)
		d.Advance(1002, severityeventsdef.SeverityMedium)
		if len(l.events) != 1 {
			t.Fatalf("expected the High de-escalation to be blocked, got %v", l.events)
		}
		d.Advance(1003, severityeventsdef.SeverityMedium)
		if len(l.events) != 2 || l.events[1].ToLevel != severityeventsdef.SeverityMedium {
			t.Fatalf("expected High to fall back to Medium, got %v", l.events)
		}

		d.Advance(1005, severityeventsdef.SeverityLow)
		if len(l.events) != 2 {
			t.Fatalf("the fresh Medium hold must block Low, got %v", l.events)
		}
		d.Advance(1006, severityeventsdef.SeverityLow)
		if len(l.events) != 3 {
			t.Fatalf("expected Medium to fall back to Low, got %v", l.events)
		}
	})

	t.Run("historical medium wins at high expiry", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{CooldownSecs: 3})

		d.Advance(1000, severityeventsdef.SeverityHigh)
		d.AdvanceWithDiscovery(1003, severityeventsdef.SeverityLow, severityeventsdef.SeverityMedium, 1001)
		if len(l.events) != 2 || l.events[1].ToLevel != severityeventsdef.SeverityMedium || l.events[1].HistoricalTimestamp != 1001 {
			t.Fatalf("expected a historical Medium activation at High expiry, got %v", l.events)
		}
	})

	t.Run("raw and discovery levels are coalesced", func(t *testing.T) {
		l := &collectingListener{}
		d := newTestDispatcher(l, severityeventsdef.SeverityEventsConfiguration{})

		d.AdvanceWithDiscovery(1000, severityeventsdef.SeverityHigh, severityeventsdef.SeverityMedium, 999)
		if len(l.events) != 1 || l.events[0].ToLevel != severityeventsdef.SeverityHigh {
			t.Fatalf("expected one coalesced High transition, got %v", l.events)
		}
	})
}
