// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
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

func TestScopedAnomalyScorerIdleEviction(t *testing.T) {
	var reasons []string
	cfg := fastScopedScorerConfig()
	cfg.IdleEvictionSec = 10
	cfg.OnScopeEvicted = func(reason string) { reasons = append(reasons, reason) }
	router := newScopedAnomalyScorer(cfg)

	listener := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 25,
	}, listener)
	require.NoError(t, err)

	// Escalation-only subscriber must not block idle eviction.
	escOnly := &collectingScopedListener{}
	_, err = router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		Filter: severityeventsdef.SeverityEventFilter{
			Direction: severityeventsdef.SeverityEventEscalation,
		},
	}, escOnly)
	require.NoError(t, err)

	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 100, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 100, "service:checkout"))
	router.Advance(100)
	require.Equal(t, 1, router.ActiveScopeCount())

	// Raw score decays after window (105), but cooldown holds listener at High until 125.
	router.Advance(120)
	assert.Equal(t, 1, router.ActiveScopeCount())
	assert.Empty(t, reasons)

	// Cooldown clears at 125; idle TTL (10s) elapses at 135.
	router.Advance(125)
	assert.Equal(t, severityeventsdef.SeverityLow, lastScopedLevel(listener.eventsForService("checkout")))
	router.Advance(134)
	assert.Equal(t, 1, router.ActiveScopeCount())
	router.Advance(135)
	assert.Zero(t, router.ActiveScopeCount())
	assert.Equal(t, []string{scopedEvictionReasonIdle}, reasons)

	// Recreates cleanly on a later anomaly.
	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 200, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 200, "service:checkout"))
	router.Advance(200)
	assert.Equal(t, 1, router.ActiveScopeCount())
	assert.Equal(t, severityeventsdef.SeverityHigh, lastScopedLevel(listener.eventsForService("checkout")))
}

func TestScopedAnomalyScorerLRUCapacityEvictionPrefersLowThenDeescalates(t *testing.T) {
	var reasons []string
	cfg := fastScopedScorerConfig()
	cfg.Scorer.SaturationK = 10.0
	cfg.MaxScopes = 2
	cfg.IdleEvictionSec = 1000
	cfg.OnScopeEvicted = func(reason string) { reasons = append(reasons, reason) }
	router := newScopedAnomalyScorer(cfg)

	listener := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 30,
	}, listener)
	require.NoError(t, err)

	// Escalation-only subscriber must not receive synthetic Low de-escalation on eviction.
	escOnly := &collectingScopedListener{}
	_, err = router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		Filter: severityeventsdef.SeverityEventFilter{
			Direction: severityeventsdef.SeverityEventEscalation,
		},
	}, escOnly)
	require.NoError(t, err)

	// Escalate svc-elevated at t=100 (older in LRU), then keep svc-low at Low at t=101 (newer in LRU).
	for idx := range 6 {
		router.ProcessAnomaly(makeServiceMetricAnomaly(fmt.Sprintf("e%d", idx), 100, "service:svc-elevated"))
	}
	router.Advance(100)
	require.Equal(t, severityeventsdef.SeverityHigh, lastScopedLevel(listener.eventsForService("svc-elevated")))

	router.ProcessAnomaly(makeServiceMetricAnomaly("l1", 101, "service:svc-low"))
	router.Advance(106) // svc-low window is empty (Low); svc-elevated stays High due to 30s cooldown.

	// Adding svc-new at MaxScopes=2 must evict svc-low (Pass 1) instead of older svc-elevated.
	for idx := range 6 {
		router.ProcessAnomaly(makeServiceMetricAnomaly(fmt.Sprintf("n%d", idx), 107, "service:svc-new"))
	}
	router.Advance(107)
	assert.Equal(t, 2, router.ActiveScopeCount())
	assert.Equal(t, []string{scopedEvictionReasonCapacity}, reasons)
	assert.Equal(t, severityeventsdef.SeverityHigh, lastScopedLevel(listener.eventsForService("svc-elevated")))
	assert.Equal(t, severityeventsdef.SeverityHigh, lastScopedLevel(listener.eventsForService("svc-new")))

	// Now both scopes are elevated; adding svc-fourth forces Pass 2 eviction of svc-elevated with synthetic Low.
	router.ProcessAnomaly(makeServiceMetricAnomaly("f1", 108, "service:svc-fourth"))
	assert.Equal(t, 2, router.ActiveScopeCount())
	assert.Equal(t, []string{scopedEvictionReasonCapacity, scopedEvictionReasonCapacity}, reasons)

	elevatedEvents := listener.eventsForService("svc-elevated")
	require.Len(t, elevatedEvents, 2)
	assert.Equal(t, severityeventsdef.SeverityHigh, elevatedEvents[1].Event.FromLevel)
	assert.Equal(t, severityeventsdef.SeverityLow, elevatedEvents[1].Event.ToLevel)
	assert.Equal(t, severityeventsdef.SeverityEventDeescalation, elevatedEvents[1].Event.Direction)

	escEvents := escOnly.eventsForService("svc-elevated")
	require.Len(t, escEvents, 1)
	assert.Equal(t, severityeventsdef.SeverityHigh, escEvents[0].Event.ToLevel)
}

type callbackScopedListener struct {
	fn func(evt severityeventsdef.ScopedSeverityEvent)
}

func (listener *callbackScopedListener) OnScopedSeverityTransition(evt severityeventsdef.ScopedSeverityEvent) {
	listener.fn(evt)
}

func TestScopedAnomalyScorerFilteredSubscribersIdleEviction(t *testing.T) {
	cfg := fastScopedScorerConfig()
	cfg.IdleEvictionSec = 5
	router := newScopedAnomalyScorer(cfg)

	deescOnly := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 20,
		Filter: severityeventsdef.SeverityEventFilter{
			Direction: severityeventsdef.SeverityEventDeescalation,
		},
	}, deescOnly)
	require.NoError(t, err)

	// Matches Low->High only, missing High->Medium and Medium->Low transitions.
	skipMedium := &collectingScopedListener{}
	_, err = router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		Filter: severityeventsdef.SeverityEventFilter{
			FromLevels: []severityeventsdef.SeverityLevel{severityeventsdef.SeverityLow, severityeventsdef.SeverityHigh},
			ToLevels:   []severityeventsdef.SeverityLevel{severityeventsdef.SeverityHigh, severityeventsdef.SeverityLow},
		},
	}, skipMedium)
	require.NoError(t, err)

	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 100, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 100, "service:checkout"))
	router.Advance(100)

	// Cooldown holds deescOnly's dispatcher at High even though its filter dropped Low->High.
	router.Advance(115)
	assert.Equal(t, 1, router.ActiveScopeCount())
	assert.Empty(t, deescOnly.eventsForService("checkout"))

	// At 120 cooldown expires and deescOnly receives High->Low; skipMedium reaches Low internally too.
	router.Advance(120)
	deescEvents := deescOnly.eventsForService("checkout")
	require.Len(t, deescEvents, 1)
	assert.Equal(t, severityeventsdef.SeverityLow, deescEvents[0].Event.ToLevel)

	router.Advance(125)
	assert.Zero(t, router.ActiveScopeCount())
}

func TestScopedAnomalyScorerIgnoredLateAnomalyDoesNotEvictOrTouchLRU(t *testing.T) {
	var evicted int
	cfg := fastScopedScorerConfig()
	cfg.MaxScopes = 1
	cfg.IdleEvictionSec = 5
	cfg.Scorer.LateAnomalyBehavior = lateAnomalyIgnore
	cfg.OnScopeEvicted = func(string) { evicted++ }
	router := newScopedAnomalyScorer(cfg)

	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 100, "service:svc-a"))
	router.Advance(100)
	router.Advance(106) // svc-a becomes idle at 106 (lastAdvancedSec=106)

	// Ignored late anomaly for svc-b must not evict svc-a.
	router.ProcessAnomaly(makeServiceMetricAnomaly("late-b", 105, "service:svc-b"))
	assert.Equal(t, 1, router.ActiveScopeCount())
	assert.Zero(t, evicted)

	// Ignored late anomaly for svc-a must not reset idleSince.
	router.ProcessAnomaly(makeServiceMetricAnomaly("late-a", 105, "service:svc-a"))
	router.Advance(111)
	assert.Zero(t, router.ActiveScopeCount())
	assert.Equal(t, 1, evicted)
}

func TestScopedAnomalyScorerReorderingWindowReplay(t *testing.T) {
	cfg := fastScopedScorerConfig()
	cfg.Scorer.ReorderingWindowSecs = 5
	cfg.Scorer.LateAnomalyBehavior = lateAnomalyIgnore
	router := newScopedAnomalyScorer(cfg)

	listener := &collectingScopedListener{}
	_, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, listener)
	require.NoError(t, err)

	router.Advance(100)
	router.Advance(103)
	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 101, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 101, "service:checkout"))
	router.Advance(104)

	events := listener.eventsForService("checkout")
	require.Len(t, events, 1)
	assert.Equal(t, severityeventsdef.SeverityHigh, events[0].Event.ToLevel)
	assert.Equal(t, int64(101), events[0].Event.HistoricalTimestamp)
}

func TestScopedAnomalyScorerReentrantUnsubscribe(t *testing.T) {
	router := newScopedAnomalyScorer(fastScopedScorerConfig())

	var sub severityeventsdef.ScopedSeverityEventsSubscription
	var calls int
	var err error
	sub, err = router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, &callbackScopedListener{
		fn: func(severityeventsdef.ScopedSeverityEvent) {
			calls++
			sub.Unsubscribe()
		},
	})
	require.NoError(t, err)

	router.ProcessAnomaly(makeServiceMetricAnomaly("s1", 100, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("s2", 100, "service:checkout"))
	router.Advance(100)
	router.Advance(110)

	assert.Equal(t, 1, calls)
}

func TestScopedAnomalyScorerConcurrentSubscribeRace(t *testing.T) {
	cfg := fastScopedScorerConfig()
	cfg.MaxScopes = 4
	cfg.IdleEvictionSec = 3
	router := newScopedAnomalyScorer(cfg)

	var activeCallbacks atomic.Int32
	var concurrentViolation atomic.Bool
	sharedListener := &callbackScopedListener{
		fn: func(severityeventsdef.ScopedSeverityEvent) {
			if activeCallbacks.Add(1) > 1 {
				concurrentViolation.Store(true)
			}
			activeCallbacks.Add(-1)
		},
	}

	stopCh := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				sub, err := router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, sharedListener)
				assert.NoError(t, err)
				sub.Unsubscribe()
				runtime.Gosched()
			}
		}
	}()

	for ts := int64(100); ts < 160; ts++ {
		svc := fmt.Sprintf("svc-%d", ts%8)
		router.ProcessAnomaly(makeServiceMetricAnomaly("a1", ts, "service:"+svc))
		router.ProcessAnomaly(makeServiceMetricAnomaly("a2", ts, "service:"+svc))
		router.Advance(ts)
		require.LessOrEqual(t, router.ActiveScopeCount(), cfg.MaxScopes)
	}

	close(stopCh)
	wg.Wait()
	assert.False(t, concurrentViolation.Load(), "listener invoked concurrently")
}

func BenchmarkScopedAnomalyScorerProcessAnomaly(b *testing.B) {
	router := newScopedAnomalyScorer(fastScopedScorerConfig())
	_, _ = router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, &callbackScopedListener{
		fn: func(severityeventsdef.ScopedSeverityEvent) {},
	})
	anomalies := make([]observerdef.Anomaly, 128)
	for idx := range anomalies {
		anomalies[idx] = makeServiceMetricAnomaly("m", 100, fmt.Sprintf("service:svc-%d", idx))
	}

	b.ResetTimer()
	for idx := 0; idx < b.N; idx++ {
		router.ProcessAnomaly(anomalies[idx%len(anomalies)])
	}
}

func BenchmarkScopedAnomalyScorerAdvance128Scopes(b *testing.B) {
	cfg := fastScopedScorerConfig()
	cfg.IdleEvictionSec = 1 << 30
	router := newScopedAnomalyScorer(cfg)
	_, _ = router.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, &callbackScopedListener{
		fn: func(severityeventsdef.ScopedSeverityEvent) {},
	})
	for idx := range 128 {
		router.ProcessAnomaly(makeServiceMetricAnomaly("m", 100, fmt.Sprintf("service:svc-%d", idx)))
	}

	b.ResetTimer()
	for idx := 0; idx < b.N; idx++ {
		router.Advance(int64(100 + idx))
	}
}

func lastScopedLevel(events []severityeventsdef.ScopedSeverityEvent) severityeventsdef.SeverityLevel {
	if len(events) == 0 {
		return severityeventsdef.SeverityLow
	}
	return events[len(events)-1].Event.ToLevel
}

func TestObserverImplSubscribeScopedSeverityEventsThroughEngine(t *testing.T) {
	storage := newTimeSeriesStorage()
	svcCheckout := storage.Add("check", "cpu", 1, 1, []string{"service:checkout"})
	svcBilling := storage.Add("check", "cpu", 1, 1, []string{"service:billing"})

	detector := &anomalyDetector{
		name: "test",
		anomalies: []observerdef.Anomaly{
			{
				Source: observerdef.SeriesDescriptor{
					Namespace: "check",
					Name:      "cpu",
					Tags:      testCompositeTags([]string{"service:checkout"}),
					Aggregate: observerdef.AggregateAverage,
				},
				SourceRef:    &observerdef.QueryHandle{Ref: svcCheckout.Ref, Aggregate: observerdef.AggregateAverage},
				DetectorName: "test",
				Timestamp:    100,
			},
		},
	}

	router := newScopedAnomalyScorer(fastScopedScorerConfig())
	eng := mustNewEngine(engineConfig{
		storage:      storage,
		detectors:    []detectorRegistration{{detector: detector}},
		scopedScorer: router,
	})
	obs := &observerImpl{engine: eng}

	listener := &collectingScopedListener{}
	sub, err := obs.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, listener)
	require.NoError(t, err)
	t.Cleanup(sub.Unsubscribe)

	eng.Advance(100)

	require.Greater(t, lastScopedLevel(listener.eventsForService("checkout")), severityeventsdef.SeverityLow)
	require.Empty(t, listener.eventsForService("billing"))
	require.NotNil(t, svcBilling)

	require.NoError(t, eng.ResetForReplay(nil, nil, nil, nil, StorageConfig{}, BaselineConfig{}))
	_, err = obs.SubscribeScopedSeverityEvents(severityeventsdef.SeverityEventsConfiguration{}, listener)
	require.EqualError(t, err, "no active scoped anomaly scorer")
}

func TestScopedAnomalyScorerDrainAnomalyCounts(t *testing.T) {
	cfg := fastScopedScorerConfig()
	cfg.Scorer.LateAnomalyBehavior = lateAnomalyIgnore
	cfg.Scorer.ReorderingWindowSecs = 0
	router := newScopedAnomalyScorer(cfg)
	router.Advance(100)

	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 101, "service:checkout"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 101, "service:billing"))
	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 101))
	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 99, "service:checkout")) // dropped by lateAnomalyIgnore

	scoped, unscoped := router.drainAnomalyCounts()
	require.EqualValues(t, 2, scoped)
	require.EqualValues(t, 1, unscoped)

	scoped, unscoped = router.drainAnomalyCounts()
	require.Zero(t, scoped)
	require.Zero(t, unscoped)
}

func hasServiceSeries(t *testing.T, telComp telemetry.Component, metricName, service string) bool {
	t.Helper()
	families, err := telComp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	fullMetricName := "observer__" + metricName
	for _, family := range families {
		if family.GetName() != fullMetricName {
			continue
		}
		for _, m := range family.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "service" && lp.GetValue() == service {
					return true
				}
			}
		}
	}
	return false
}

func TestScopedAnomalyScorerPerServiceGaugesLifecycle(t *testing.T) {
	telComp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(telComp)

	cfg := fastScopedScorerConfig()
	cfg.MaxScopes = 2
	cfg.IdleEvictionSec = 2
	cfg.Scorer.WindowSecs = 1
	cfg.Scorer.ReorderingWindowSecs = 2
	cfg.Gauges = tel.scopedScorerGauges

	router := newScopedAnomalyScorer(cfg)
	router.Advance(100)
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "checkout"))

	// Creating checkout after Advance(100) runs catch-up before attaching gauges, so no spurious 0 series yet.
	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 101, "service:checkout"))
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "checkout"))

	router.Advance(101)
	require.True(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "checkout"))
	require.True(t, hasServiceSeries(t, telComp, telemetryScopedScorerSeverity, "checkout"))
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMAFinalized, "checkout"))

	router.Advance(102)
	require.True(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMAFinalized, "checkout"))

	// Capacity LRU eviction deletes victim gauges.
	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 102, "service:billing"))
	router.Advance(103)
	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 103, "service:payments"))
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "checkout"))
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMAFinalized, "checkout"))
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerSeverity, "checkout"))
	require.True(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "billing"))

	// Idle eviction deletes remaining gauges once settled back to Low.
	for sec := int64(104); sec <= 130; sec++ {
		router.Advance(sec)
	}
	require.Zero(t, router.ActiveScopeCount())
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "billing"))
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerSeverity, "payments"))

	// Reset deletes gauges for active scopes.
	router.ProcessAnomaly(makeServiceMetricAnomaly("cpu", 131, "service:auth"))
	router.Advance(131)
	require.True(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "auth"))
	router.Reset()
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerEWMA, "auth"))
	require.False(t, hasServiceSeries(t, telComp, telemetryScopedScorerSeverity, "auth"))
}
