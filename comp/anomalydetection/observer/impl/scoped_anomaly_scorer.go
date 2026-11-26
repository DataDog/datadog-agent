// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"container/list"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	severityeventsimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/impl"
)

const (
	scopedAnomalyScorerName            = "scoped_anomaly_scorer"
	scopedScorerConfigPrefix           = "anomaly_detection.anomaly_scorer.scoped."
	defaultScopedScorerMaxScopes       = 128
	defaultScopedScorerIdleEvictionSec = 3600
	scopedEvictionReasonIdle           = "idle"
	scopedEvictionReasonCapacity       = "capacity"
)

// ScopedAnomalyScorerConfig configures a service-scoped anomaly scorer router.
type ScopedAnomalyScorerConfig struct {
	// Scorer configures per-service child scorers (CorrelationEvents and Logs are forced false).
	Scorer AnomalyScorerConfig
	// MaxScopes caps active per-service scorers (<= 0 uses defaultScopedScorerMaxScopes).
	MaxScopes int
	// IdleEvictionSec sets idle TTL in data-time seconds after returning to Low (<= 0 uses default).
	IdleEvictionSec int64
	// OnScopeEvicted is called when a scope is evicted ("idle" or "capacity").
	OnScopeEvicted func(reason string)
	// Gauges configures optional per-service EWMA/severity gauges on child scorers.
	Gauges scorerGauges
}

// readScopedAnomalyScorerConfig reads scoped scorer settings from the agent config.
// prefix is the key prefix, e.g. "anomaly_detection.anomaly_scorer.scoped.".
func readScopedAnomalyScorerConfig(r ConfigReader, prefix string, scorerCfg AnomalyScorerConfig) (ScopedAnomalyScorerConfig, bool) {
	cfg := ScopedAnomalyScorerConfig{
		Scorer:          scorerCfg,
		MaxScopes:       defaultScopedScorerMaxScopes,
		IdleEvictionSec: defaultScopedScorerIdleEvictionSec,
	}

	key := prefix + "max_scopes"
	if maxScopes := r.GetInt(key); maxScopes > 0 {
		cfg.MaxScopes = maxScopes
	} else {
		logging.Warnf("scoped anomaly scorer: %s must be > 0, got %d — using default %d", key, maxScopes, defaultScopedScorerMaxScopes)
	}

	key = prefix + "idle_eviction_ttl"
	if ttl := r.GetDuration(key); ttl >= time.Second {
		cfg.IdleEvictionSec = int64(ttl.Seconds())
	} else {
		logging.Warnf("scoped anomaly scorer: %s must be >= 1s, got %s — using default %ds", key, ttl, defaultScopedScorerIdleEvictionSec)
	}

	perServiceTelemetry := r.GetBool(prefix + "per_service_telemetry")
	return cfg, perServiceTelemetry
}

type scopedSubscription struct {
	mu           sync.Mutex
	unsubscribed atomic.Bool
	cfg          severityeventsdef.SeverityEventsConfiguration
	listener     severityeventsdef.ScopedSeverityEventListener
}

type scopedChildDispatcher struct {
	sub                *scopedSubscription
	dispatcher         *severityeventsimpl.Dispatcher
	deescalatedOnEvict atomic.Bool
}

type scopedListenerAdapter struct {
	key      severityeventsdef.ScopeKey
	listener severityeventsdef.ScopedSeverityEventListener
}

func (adapter *scopedListenerAdapter) OnSeverityTransition(evt severityeventsdef.SeverityEvent) {
	adapter.listener.OnScopedSeverityTransition(severityeventsdef.ScopedSeverityEvent{
		ScopeKey: adapter.key,
		Event:    evt,
	})
}

type scopedScorerEntry struct {
	key         severityeventsdef.ScopeKey
	scorer      *anomalyScorer
	dispatchers []*scopedChildDispatcher
	idleSince   int64
	lruElem     *list.Element
}

// scopedAnomalyScorer routes anomalies by service ScopeKey into lazy per-service scorers.
type scopedAnomalyScorer struct {
	cfg ScopedAnomalyScorerConfig

	mu              sync.Mutex
	entries         map[severityeventsdef.ScopeKey]*scopedScorerEntry
	lru             *list.List
	subs            []*scopedSubscription
	lastAdvancedSec int64

	scopedAnomalies   atomic.Uint64
	unscopedAnomalies atomic.Uint64
}

var (
	_ observerdef.Correlator             = (*scopedAnomalyScorer)(nil)
	_ severityeventsdef.ScopedSubscriber = (*scopedAnomalyScorer)(nil)
)

func newScopedAnomalyScorer(cfg ScopedAnomalyScorerConfig) *scopedAnomalyScorer {
	cfg.Scorer.CorrelationEvents = false
	cfg.Scorer.Logs = false
	cfg.Scorer.MaxBuckets = -1
	cfg.Scorer = newAnomalyScorerBase(cfg.Scorer).config
	if cfg.MaxScopes <= 0 {
		cfg.MaxScopes = defaultScopedScorerMaxScopes
	}
	if cfg.IdleEvictionSec <= 0 {
		cfg.IdleEvictionSec = defaultScopedScorerIdleEvictionSec
	}
	return &scopedAnomalyScorer{
		cfg:     cfg,
		entries: make(map[severityeventsdef.ScopeKey]*scopedScorerEntry),
		lru:     list.New(),
	}
}

func anomalyService(anomaly observerdef.Anomaly) string {
	if anomaly.Context != nil && anomaly.Context.Service != "" {
		return anomaly.Context.Service
	}
	var service string
	anomaly.Source.Tags.Find(func(tag string) bool {
		if strings.HasPrefix(tag, "service:") && len(tag) > len("service:") {
			service = tag[len("service:"):]
			return true
		}
		return false
	})
	return service
}

// Name implements observerdef.Correlator.
func (router *scopedAnomalyScorer) Name() string {
	return scopedAnomalyScorerName
}

// ActiveScopeCount returns the number of active per-service child scorers.
func (router *scopedAnomalyScorer) ActiveScopeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.entries)
}

// drainAnomalyCounts returns and resets pending scoped and unscoped anomaly counts.
func (router *scopedAnomalyScorer) drainAnomalyCounts() (scoped, unscoped uint64) {
	return router.scopedAnomalies.Swap(0), router.unscopedAnomalies.Swap(0)
}

// ProcessAnomaly implements observerdef.Correlator.
func (router *scopedAnomalyScorer) ProcessAnomaly(anomaly observerdef.Anomaly) {
	service := anomalyService(anomaly)
	key := severityeventsdef.ScopeKeyForService(service)
	if key == 0 {
		router.unscopedAnomalies.Add(1)
		return
	}

	var evictedScorer *anomalyScorer
	var deescalations []pendingScopedDeescalation

	router.mu.Lock()
	if router.lastAdvancedSec > 0 && router.cfg.Scorer.LateAnomalyBehavior == lateAnomalyIgnore {
		cutoff := router.lastAdvancedSec - router.cfg.Scorer.ReorderingWindowSecs
		if anomaly.Timestamp <= cutoff {
			router.mu.Unlock()
			return
		}
	}
	entry, ok := router.entries[key]
	if ok {
		entry.idleSince = 0
		router.lru.MoveToFront(entry.lruElem)
	} else {
		if len(router.entries) >= router.cfg.MaxScopes {
			evictTime := max(anomaly.Timestamp, router.lastAdvancedSec)
			evictedScorer, deescalations = router.evictOneLRULocked(evictTime)
		}
		entry = router.createEntryLocked(key, service)
	}
	router.scopedAnomalies.Add(1)
	router.mu.Unlock()

	if evictedScorer != nil {
		evictedScorer.clearGauges()
		deliverPendingDeescalations(deescalations)
		if router.cfg.OnScopeEvicted != nil {
			router.cfg.OnScopeEvicted(scopedEvictionReasonCapacity)
		}
	}
	entry.scorer.ProcessAnomaly(anomaly)
}

func (router *scopedAnomalyScorer) createEntryLocked(key severityeventsdef.ScopeKey, service string) *scopedScorerEntry {
	child := newAnomalyScorerBase(router.cfg.Scorer)
	if router.lastAdvancedSec > 0 {
		child.advanceStates(router.lastAdvancedSec)
	}
	if router.cfg.Gauges.enabled() {
		child.gauges = router.cfg.Gauges
		child.gaugeTag = service
	}
	entry := &scopedScorerEntry{
		key:         key,
		scorer:      child,
		dispatchers: make([]*scopedChildDispatcher, 0, len(router.subs)),
	}
	for _, sub := range router.subs {
		adapter := &scopedListenerAdapter{key: key, listener: sub.listener}
		entry.dispatchers = append(entry.dispatchers, &scopedChildDispatcher{
			sub:        sub,
			dispatcher: severityeventsimpl.NewDispatcher(sub.cfg, adapter),
		})
	}
	entry.lruElem = router.lru.PushFront(entry)
	router.entries[key] = entry
	return entry
}

type pendingScopedDeescalation struct {
	sub      *scopedSubscription
	listener severityeventsdef.ScopedSeverityEventListener
	event    severityeventsdef.ScopedSeverityEvent
}

func deliverPendingDeescalations(deescalations []pendingScopedDeescalation) {
	for _, pending := range deescalations {
		pending.sub.mu.Lock()
		if !pending.sub.unsubscribed.Load() {
			pending.listener.OnScopedSeverityTransition(pending.event)
		}
		pending.sub.mu.Unlock()
	}
}

func appendDispatcherCOW(existing []*scopedChildDispatcher, child *scopedChildDispatcher) []*scopedChildDispatcher {
	next := make([]*scopedChildDispatcher, len(existing)+1)
	copy(next, existing)
	next[len(existing)] = child
	return next
}

func removeSubDispatcherCOW(existing []*scopedChildDispatcher, sub *scopedSubscription) ([]*scopedChildDispatcher, bool) {
	for idx, child := range existing {
		if child.sub != sub {
			continue
		}
		next := make([]*scopedChildDispatcher, 0, len(existing)-1)
		next = append(next, existing[:idx]...)
		next = append(next, existing[idx+1:]...)
		return next, true
	}
	return existing, false
}

func (router *scopedAnomalyScorer) evictOneLRULocked(evictSec int64) (*anomalyScorer, []pendingScopedDeescalation) {
	for elem := router.lru.Back(); elem != nil; elem = elem.Prev() {
		candidate := elem.Value.(*scopedScorerEntry)
		if router.entryIsIdle(candidate) {
			router.lru.Remove(elem)
			delete(router.entries, candidate.key)
			return candidate.scorer, nil
		}
	}

	elem := router.lru.Back()
	if elem == nil {
		return nil, nil
	}
	victim := elem.Value.(*scopedScorerEntry)
	router.lru.Remove(elem)
	delete(router.entries, victim.key)

	var deescalations []pendingScopedDeescalation
	for _, child := range victim.dispatchers {
		deliveredLevel := child.dispatcher.Level()
		if deliveredLevel <= severityeventsdef.SeverityLow {
			continue
		}
		if !child.deescalatedOnEvict.CompareAndSwap(false, true) {
			continue
		}
		evt := severityeventsdef.SeverityEvent{
			Timestamp: evictSec,
			FromLevel: deliveredLevel,
			ToLevel:   severityeventsdef.SeverityLow,
			Direction: severityeventsdef.SeverityEventDeescalation,
		}
		if severityeventsimpl.EventFilterMatches(child.sub.cfg.Filter, evt) {
			deescalations = append(deescalations, pendingScopedDeescalation{
				sub:      child.sub,
				listener: child.sub.listener,
				event: severityeventsdef.ScopedSeverityEvent{
					ScopeKey: victim.key,
					Event:    evt,
				},
			})
		}
	}
	return victim.scorer, deescalations
}

func (s *anomalyScorer) isIdleForEviction() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.score.initialized &&
		s.score.level == severityeventsdef.SeverityLow &&
		len(s.pending) == 0 &&
		len(s.late) == 0 &&
		len(s.windowMap) == 0
}

func (router *scopedAnomalyScorer) entryIsIdle(entry *scopedScorerEntry) bool {
	if !entry.scorer.isIdleForEviction() {
		return false
	}
	for _, child := range entry.dispatchers {
		if child.dispatcher.Level() > severityeventsdef.SeverityLow {
			return false
		}
	}
	return true
}

// Advance implements observerdef.Correlator.
func (router *scopedAnomalyScorer) Advance(dataTime int64) {
	type advanceEntry struct {
		entry       *scopedScorerEntry
		states      []secState
		missedHigh  int64
		dispatchers []*scopedChildDispatcher
	}

	router.mu.Lock()
	if dataTime > router.lastAdvancedSec {
		router.lastAdvancedSec = dataTime
	}
	if len(router.entries) == 0 {
		router.mu.Unlock()
		return
	}
	snapshot := make([]advanceEntry, 0, len(router.entries))
	for _, entry := range router.entries {
		snapshot = append(snapshot, advanceEntry{entry: entry})
	}
	router.mu.Unlock()

	for idx := range snapshot {
		snapshot[idx].states, snapshot[idx].missedHigh = snapshot[idx].entry.scorer.advanceStates(dataTime)
	}

	router.mu.Lock()
	for idx := range snapshot {
		entry := snapshot[idx].entry
		if router.entries[entry.key] == entry {
			snapshot[idx].dispatchers = entry.dispatchers
		}
	}
	router.mu.Unlock()

	for _, item := range snapshot {
		if len(item.states) == 0 || len(item.dispatchers) == 0 {
			continue
		}
		for _, child := range item.dispatchers {
			child.sub.mu.Lock()
			if !child.sub.unsubscribed.Load() {
				for _, st := range item.states {
					feedDispatcherState(child.dispatcher, dataTime, st, item.missedHigh)
				}
			}
			child.sub.mu.Unlock()
		}
	}

	idleEvicted := 0
	var evictedScorers []*anomalyScorer
	router.mu.Lock()
	for _, item := range snapshot {
		entry := item.entry
		if router.entries[entry.key] != entry {
			continue
		}
		if router.entryIsIdle(entry) {
			if entry.idleSince == 0 {
				entry.idleSince = dataTime
			}
			if dataTime-entry.idleSince >= router.cfg.IdleEvictionSec {
				router.lru.Remove(entry.lruElem)
				delete(router.entries, entry.key)
				idleEvicted++
				if router.cfg.Gauges.enabled() {
					evictedScorers = append(evictedScorers, entry.scorer)
				}
			}
		} else {
			entry.idleSince = 0
		}
	}
	router.mu.Unlock()

	for _, scorer := range evictedScorers {
		scorer.clearGauges()
	}
	if idleEvicted > 0 && router.cfg.OnScopeEvicted != nil {
		for range idleEvicted {
			router.cfg.OnScopeEvicted(scopedEvictionReasonIdle)
		}
	}
}

// ActiveCorrelations implements observerdef.Correlator.
func (router *scopedAnomalyScorer) ActiveCorrelations() []observerdef.ActiveCorrelation {
	return nil
}

// PendingEvents implements observerdef.Correlator.
func (router *scopedAnomalyScorer) PendingEvents() []observerdef.CorrelatorEvent {
	return nil
}

// Reset implements observerdef.Correlator.
func (router *scopedAnomalyScorer) Reset() {
	router.mu.Lock()
	oldEntries := router.entries
	router.entries = make(map[severityeventsdef.ScopeKey]*scopedScorerEntry)
	router.lru = list.New()
	router.lastAdvancedSec = 0
	router.mu.Unlock()

	if router.cfg.Gauges.enabled() {
		for _, entry := range oldEntries {
			entry.scorer.clearGauges()
		}
	}
}

// SubscribeScopedSeverityEvents implements severityeventsdef.ScopedSubscriber.
func (router *scopedAnomalyScorer) SubscribeScopedSeverityEvents(
	cfg severityeventsdef.SeverityEventsConfiguration,
	listener severityeventsdef.ScopedSeverityEventListener,
) (severityeventsdef.ScopedSeverityEventsSubscription, error) {
	if listener == nil {
		return severityeventsdef.ScopedSeverityEventsSubscription{}, errors.New("nil scoped severity event listener")
	}

	sub := &scopedSubscription{
		cfg:      cfg,
		listener: listener,
	}
	sub.mu.Lock()
	defer sub.mu.Unlock()

	type initialSeed struct {
		entry    *scopedScorerEntry
		child    *scopedChildDispatcher
		score    scoringState
		entrySec int64
	}

	router.mu.Lock()
	router.subs = append(router.subs, sub)
	seeds := make([]initialSeed, 0, len(router.entries))
	for _, entry := range router.entries {
		score, entryLastSec := entry.scorer.rawScoreSnapshot()
		adapter := &scopedListenerAdapter{key: entry.key, listener: listener}
		child := &scopedChildDispatcher{
			sub:        sub,
			dispatcher: severityeventsimpl.NewDispatcher(cfg, adapter),
		}
		entry.dispatchers = appendDispatcherCOW(entry.dispatchers, child)
		seeds = append(seeds, initialSeed{
			entry:    entry,
			child:    child,
			score:    score,
			entrySec: entryLastSec,
		})
	}
	router.mu.Unlock()

	for _, seed := range seeds {
		if sub.unsubscribed.Load() {
			break
		}
		if !seed.score.initialized {
			continue
		}
		seed.child.dispatcher.DeliverInitial(seed.entrySec, seed.score.level)
		if seed.score.level <= severityeventsdef.SeverityLow || sub.unsubscribed.Load() {
			continue
		}
		router.mu.Lock()
		stillActive := router.entries[seed.entry.key] == seed.entry
		router.mu.Unlock()
		if stillActive {
			continue
		}
		deliveredLevel := seed.child.dispatcher.Level()
		if deliveredLevel <= severityeventsdef.SeverityLow || !seed.child.deescalatedOnEvict.CompareAndSwap(false, true) {
			continue
		}
		evt := severityeventsdef.SeverityEvent{
			Timestamp: seed.entrySec,
			FromLevel: deliveredLevel,
			ToLevel:   severityeventsdef.SeverityLow,
			Direction: severityeventsdef.SeverityEventDeescalation,
		}
		if severityeventsimpl.EventFilterMatches(cfg.Filter, evt) {
			listener.OnScopedSeverityTransition(severityeventsdef.ScopedSeverityEvent{
				ScopeKey: seed.entry.key,
				Event:    evt,
			})
		}
	}

	var unsubOnce sync.Once
	return severityeventsdef.ScopedSeverityEventsSubscription{
		Unsubscribe: func() {
			unsubOnce.Do(func() {
				sub.unsubscribed.Store(true)
				router.unsubscribe(sub)
			})
		},
	}, nil
}

func (router *scopedAnomalyScorer) unsubscribe(sub *scopedSubscription) {
	router.mu.Lock()
	subIdx := slices.Index(router.subs, sub)
	if subIdx < 0 {
		router.mu.Unlock()
		return
	}
	router.subs = slices.Delete(router.subs, subIdx, subIdx+1)
	for _, entry := range router.entries {
		entry.dispatchers, _ = removeSubDispatcherCOW(entry.dispatchers, sub)
	}
	router.mu.Unlock()
}
