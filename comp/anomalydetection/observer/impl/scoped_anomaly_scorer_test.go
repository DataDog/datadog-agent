// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type collectingScopedListener struct {
	mu     sync.Mutex
	events []severityeventsdef.ScopedSeverityEvent
}

func (listener *collectingScopedListener) OnScopedSeverityTransition(evt severityeventsdef.ScopedSeverityEvent) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	listener.events = append(listener.events, evt)
}

func (listener *collectingScopedListener) snapshot() []severityeventsdef.ScopedSeverityEvent {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	out := make([]severityeventsdef.ScopedSeverityEvent, len(listener.events))
	copy(out, listener.events)
	return out
}

func (listener *collectingScopedListener) eventsForService(service string) []severityeventsdef.ScopedSeverityEvent {
	key := severityeventsdef.ScopeKeyForService(service)
	var matched []severityeventsdef.ScopedSeverityEvent
	for _, evt := range listener.snapshot() {
		if evt.ScopeKey == key {
			matched = append(matched, evt)
		}
	}
	return matched
}

func fastScopedScorerConfig() ScopedAnomalyScorerConfig {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 5
	return ScopedAnomalyScorerConfig{Scorer: cfg}
}

func makeServiceMetricAnomaly(seriesName string, ts int64, tags ...string) observerdef.Anomaly {
	return observerdef.Anomaly{
		DetectorName: "bocpd",
		Timestamp:    ts,
		Source: observerdef.SeriesDescriptor{
			Namespace: "test",
			Name:      seriesName,
			Tags:      testCompositeTags(tags),
		},
	}
}

func makeServiceLogAnomaly(seriesName string, ts int64, ctxService string, tags ...string) observerdef.Anomaly {
	anomaly := makeServiceMetricAnomaly(seriesName, ts, tags...)
	anomaly.Type = observerdef.AnomalyTypeLog
	anomaly.Context = &observerdef.MetricContext{Service: ctxService}
	return anomaly
}

func TestScopedAnomalyScorerServiceIsolation(t *testing.T) {
	router := newScopedAnomalyScorer(fastScopedScorerConfig())
	listener := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, listener)
	require.NoError(t, err)

	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 100, "service:checkout", "env:prod"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 100, "service:checkout", "env:prod"))
	router.Advance(100)

	checkoutEvents := listener.eventsForService("checkout")
	require.Len(t, checkoutEvents, 1)
	assert.Equal(t, severityeventsdef.SeverityHigh, checkoutEvents[0].Event.ToLevel)
	assert.Empty(t, listener.eventsForService("payments"))
	assert.Equal(t, 1, router.ActiveScopeCount())
	assert.Nil(t, router.ActiveCorrelations())
	assert.Nil(t, router.PendingEvents())
}

func TestScopedAnomalyScorerCombinesLogsAndMetricsAndPrecedence(t *testing.T) {
	cfg := fastScopedScorerConfig()
	cfg.Scorer.SaturationK = 5.0
	router := newScopedAnomalyScorer(cfg)

	listener := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, listener)
	require.NoError(t, err)

	// Context.Service takes precedence over service: tag; empty "service:" tag is skipped.
	router.ProcessAnomaly(makeServiceLogAnomaly("log-1", 100, "checkout", "service:wrong-svc"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("metric-1", 100, "service:", "service:checkout"))
	router.Advance(100)

	require.Equal(t, 1, router.ActiveScopeCount())
	checkoutEvents := listener.eventsForService("checkout")
	require.Len(t, checkoutEvents, 1)
	assert.Equal(t, severityeventsdef.SeverityMedium, checkoutEvents[0].Event.ToLevel)
	assert.Empty(t, listener.eventsForService("wrong-svc"))
}

func TestScopedAnomalyScorerIgnoresUnscopedAnomalies(t *testing.T) {
	router := newScopedAnomalyScorer(fastScopedScorerConfig())

	listener := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, listener)
	require.NoError(t, err)

	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 100, "host:h1"))
	router.ProcessAnomaly(makeServiceLogAnomaly("log-empty", 100, "", "service:"))
	router.Advance(100)

	assert.Zero(t, router.ActiveScopeCount())
	assert.Empty(t, listener.snapshot())
}

func TestScopedAnomalyScorerPerScopeCooldownAndMidStreamSubscribe(t *testing.T) {
	router := newScopedAnomalyScorer(fastScopedScorerConfig())
	slowListener := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 20,
	}, slowListener)
	require.NoError(t, err)

	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 100, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 100, "service:checkout"))
	router.Advance(100)
	require.Len(t, slowListener.eventsForService("checkout"), 1)

	// Mid-stream subscriber receives Low -> High bootstrap immediately.
	lateListener := &collectingScopedListener{}
	_, err = router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, lateListener)
	require.NoError(t, err)
	lateEvents := lateListener.eventsForService("checkout")
	require.Len(t, lateEvents, 1)
	assert.Equal(t, severityeventsdef.SeverityLow, lateEvents[0].Event.FromLevel)
	assert.Equal(t, severityeventsdef.SeverityHigh, lateEvents[0].Event.ToLevel)

	// Another service escalates independently while checkout cools down.
	router.ProcessAnomaly(makeServiceMetricAnomaly("b1", 102, "service:billing"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("b2", 102, "service:billing"))
	router.Advance(110)

	// Window expires at 105: fast listener de-escalates checkout; cooldown holds slowListener at High.
	assert.Equal(t, severityeventsdef.SeverityLow, lastScopedLevel(lateListener.eventsForService("checkout")))
	assert.Equal(t, severityeventsdef.SeverityHigh, lastScopedLevel(slowListener.eventsForService("checkout")))
	assert.Equal(t, severityeventsdef.SeverityHigh, lastScopedLevel(slowListener.eventsForService("billing")))

	router.Advance(125)
	assert.Equal(t, severityeventsdef.SeverityLow, lastScopedLevel(slowListener.eventsForService("checkout")))
	assert.Equal(t, severityeventsdef.SeverityLow, lastScopedLevel(slowListener.eventsForService("billing")))
}

func TestScopedAnomalyScorerUnsubscribeAndReset(t *testing.T) {
	router := newScopedAnomalyScorer(fastScopedScorerConfig())
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, nil)
	require.Error(t, err)

	listener := &collectingScopedListener{}
	sub, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, listener)
	require.NoError(t, err)

	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 100, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 100, "service:checkout"))
	router.Advance(100)
	require.Len(t, listener.eventsForService("checkout"), 1)

	sub.Unsubscribe()
	sub.Unsubscribe() // idempotent

	router.Advance(110)
	assert.Len(t, listener.eventsForService("checkout"), 1)

	router.Reset()
	assert.Zero(t, router.ActiveScopeCount())
}

func lastScopedLevel(events []severityeventsdef.ScopedSeverityEvent) severityeventsdef.SeverityLevel {
	if len(events) == 0 {
		return severityeventsdef.SeverityLow
	}
	return events[len(events)-1].Event.ToLevel
}
