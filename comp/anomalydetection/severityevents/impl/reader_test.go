// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityeventsimpl

import (
	"errors"
	"testing"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// fakeScorer captures the listener passed to SubscribeSeverityEvents so
// tests can simulate transitions directly, without a real anomaly_scorer.
type fakeScorer struct {
	listener     severityeventsdef.SeverityEventListener
	unsubscribed bool
}

func (f *fakeScorer) SubscribeSeverityEvents(_ severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	f.listener = listener
	return severityeventsdef.SeverityEventsSubscription{
		Unsubscribe: func() { f.unsubscribed = true },
	}, nil
}

func (f *fakeScorer) SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	return NewSeverityReader(f, cfg)
}

func (f *fakeScorer) emit(toLevel severityeventsdef.SeverityLevel) {
	f.listener.OnSeverityTransition(severityeventsdef.SeverityEvent{ToLevel: toLevel})
}

func mustNewSeverityReader(t *testing.T, o severityeventsdef.Subscriber, cfg severityeventsdef.SeverityEventsConfiguration) severityeventsdef.SeverityEventsReaderSubscription {
	t.Helper()
	sub, err := NewSeverityReader(o, cfg)
	if err != nil {
		t.Fatalf("NewSeverityReader() error = %v", err)
	}
	return sub
}

func TestNewSeverityReader_InitialSeverityIsLow(t *testing.T) {
	fake := &fakeScorer{}
	sub := mustNewSeverityReader(t, fake, severityeventsdef.SeverityEventsConfiguration{})

	if got := sub.Reader.GetSeverity(); got != severityeventsdef.SeverityLow {
		t.Fatalf("expected initial Low, got %v", got)
	}
}

func TestSeverityReader_ReflectsEmittedTransitions(t *testing.T) {
	fake := &fakeScorer{}
	sub := mustNewSeverityReader(t, fake, severityeventsdef.SeverityEventsConfiguration{})

	fake.emit(severityeventsdef.SeverityHigh)
	if got := sub.Reader.GetSeverity(); got != severityeventsdef.SeverityHigh {
		t.Fatalf("expected High after first emit, got %v", got)
	}

	fake.emit(severityeventsdef.SeverityMedium)
	if got := sub.Reader.GetSeverity(); got != severityeventsdef.SeverityMedium {
		t.Fatalf("expected Medium after second emit, got %v", got)
	}
}

func TestSeverityReader_ClampsOutOfRangeLevels(t *testing.T) {
	fake := &fakeScorer{}
	sub := mustNewSeverityReader(t, fake, severityeventsdef.SeverityEventsConfiguration{})

	fake.emit(severityeventsdef.SeverityLevel(-1))
	if got := sub.Reader.GetSeverity(); got != severityeventsdef.SeverityLow {
		t.Fatalf("expected out-of-range low clamp, got %v", got)
	}

	fake.emit(severityeventsdef.SeverityLevel(severityeventsdef.NumSeverityLevels + 5))
	if got := sub.Reader.GetSeverity(); got != severityeventsdef.SeverityHigh {
		t.Fatalf("expected out-of-range high clamp, got %v", got)
	}
}

func TestSeverityReader_Unsubscribe(t *testing.T) {
	fake := &fakeScorer{}
	sub := mustNewSeverityReader(t, fake, severityeventsdef.SeverityEventsConfiguration{})
	if fake.unsubscribed {
		t.Fatal("reader should not start unsubscribed")
	}

	sub.Unsubscribe()

	if !fake.unsubscribed {
		t.Fatal("reader unsubscribe should unsubscribe")
	}
}

// TestSeverityReader_PicksUpCurrentLevelFromRealDispatcher verifies that a
// reader created against a live Dispatcher that already knows the current
// severity level reflects it immediately, via the initial synthetic event
// SubscribeSeverityEvents delivers to mid-stream subscribers.
func TestSeverityReader_PicksUpCurrentLevelFromRealDispatcher(t *testing.T) {
	sub := &dispatcherSubscriber{knownLevel: true, lastSec: 1001, level: severityeventsdef.SeverityHigh}
	readerSub := mustNewSeverityReader(t, sub, severityeventsdef.SeverityEventsConfiguration{})
	defer readerSub.Unsubscribe()

	if got := readerSub.Reader.GetSeverity(); got != severityeventsdef.SeverityHigh {
		t.Fatalf("expected reader to pick up current High level immediately, got %v", got)
	}
}

// dispatcherSubscriber mimics the minimal real Subscriber behavior: it
// creates one severityeventsimpl.Dispatcher per subscription and, when a
// current level is already known, seeds it via DeliverInitial before
// returning, just like anomalyScorer.SubscribeSeverityEvents does.
type dispatcherSubscriber struct {
	knownLevel bool
	lastSec    int64
	level      severityeventsdef.SeverityLevel
}

func (s *dispatcherSubscriber) SubscribeSeverityEvents(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	if listener == nil {
		return severityeventsdef.SeverityEventsSubscription{}, errors.New("nil severity event listener")
	}
	d := NewDispatcher(cfg, listener)
	if s.knownLevel {
		d.DeliverInitial(s.lastSec, s.level)
	}
	return severityeventsdef.SeverityEventsSubscription{
		Dispatcher:  d,
		Unsubscribe: func() {},
	}, nil
}

func (s *dispatcherSubscriber) SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	return NewSeverityReader(s, cfg)
}
