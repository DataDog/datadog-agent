// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type tailerScope struct{ service, source string }

// tailerScopeMatcher uses a map lookup on the ingest path and reference counts
// tuples shared by multiple configured LogSources.
type tailerScopeMatcher struct {
	mu                                                           sync.Mutex
	scopes                                                       map[tailerScope]int
	sources                                                      map[*sources.LogSource]tailerScope
	invalid                                                      map[*sources.LogSource]string
	registryUnavailable                                          bool
	telemetry                                                    *observerTelemetry
	started                                                      time.Time
	matchedMetrics, unmatchedMetrics, matchedLogs, unmatchedLogs int
}

func newTailerScopeMatcher(telemetry *observerTelemetry, _ int) *tailerScopeMatcher {
	return &tailerScopeMatcher{scopes: make(map[tailerScope]int), sources: make(map[*sources.LogSource]tailerScope), invalid: make(map[*sources.LogSource]string), telemetry: telemetry, started: time.Now()}
}

func scopeFromStrings(service, source string) (tailerScope, string) {
	service, source = strings.TrimSpace(service), strings.TrimSpace(source)
	switch {
	case service == "" && source == "":
		return tailerScope{}, "missing_service_and_source"
	case service == "":
		return tailerScope{}, "missing_service"
	case source == "":
		return tailerScope{}, "missing_source"
	}
	return tailerScope{service, source}, ""
}
func (m *tailerScopeMatcher) setRegistryUnavailable() {
	m.mu.Lock()
	m.registryUnavailable = true
	m.mu.Unlock()
}
func (m *tailerScopeMatcher) register(source *sources.LogSource, scope tailerScope) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sources[source]; exists {
		return
	}
	m.sources[source] = scope
	m.scopes[scope]++
	m.publishLocked()
}
func (m *tailerScopeMatcher) registerInvalidSource(source *sources.LogSource, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.invalid[source]; !exists {
		m.invalid[source] = reason
		m.publishLocked()
	}
}
func (m *tailerScopeMatcher) remove(source *sources.LogSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if scope, ok := m.sources[source]; ok {
		if m.scopes[scope] == 1 {
			delete(m.scopes, scope)
		} else {
			m.scopes[scope]--
		}
		delete(m.sources, source)
	}
	delete(m.invalid, source)
	m.publishLocked()
}
func (m *tailerScopeMatcher) publishLocked() {
	if m.telemetry != nil {
		m.telemetry.setTailerMatchSources(len(m.scopes), len(m.sources), len(m.invalid))
	}
}
func (m *tailerScopeMatcher) classifyMetric(_ string, metric *metricObs) {
	service, source, outcome := metricScope(metric.tags)
	m.classify("metric", service, source, outcome)
}
func (m *tailerScopeMatcher) classifyLog(obs *logObs) {
	service, source, outcome := scopeIdentity(obs.service, obs.source)
	m.classify("log", service, source, outcome)
}
func metricScope(tags []string) (string, string, string) {
	var service, source string
	ambiguous := false
	for _, tag := range tags {
		key, value, found := strings.Cut(tag, ":")
		if !found || value == "" {
			continue
		}
		switch key {
		case "service":
			if service != "" && service != value {
				ambiguous = true
			}
			service = value
		case "source":
			if source != "" && source != value {
				ambiguous = true
			}
			source = value
		}
	}
	if ambiguous {
		return service, source, "ambiguous_identity"
	}
	return scopeIdentity(service, source)
}
func scopeIdentity(service, source string) (string, string, string) {
	if service == "" && source == "" {
		return service, source, "missing_service_and_source"
	}
	if service == "" {
		return service, source, "missing_service"
	}
	if source == "" {
		return service, source, "missing_source"
	}
	return service, source, ""
}
func (m *tailerScopeMatcher) classify(signalType, service, source, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.registryUnavailable {
		outcome = "no_tailer_registry"
	} else if outcome == "" {
		if m.scopes[tailerScope{service, source}] > 0 {
			outcome = "matched"
		} else {
			outcome = "no_exact_scope"
		}
	}
	if signalType == "metric" {
		if outcome == "matched" {
			m.matchedMetrics++
		} else {
			m.unmatchedMetrics++
		}
	} else if outcome == "matched" {
		m.matchedLogs++
	} else {
		m.unmatchedLogs++
	}
	if m.telemetry != nil {
		m.telemetry.recordTailerMatch(signalType, outcome)
	}
}
func (m *tailerScopeMatcher) SnapshotAndResetTailerMatchReport() observerdef.TailerMatchReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	r := observerdef.TailerMatchReport{StartedAt: m.started.Unix(), EndedAt: now.Unix(), MatchedMetrics: m.matchedMetrics, UnmatchedMetrics: m.unmatchedMetrics, MatchedLogs: m.matchedLogs, UnmatchedLogs: m.unmatchedLogs, InvalidSources: len(m.invalid)}
	m.started = now
	m.matchedMetrics, m.unmatchedMetrics, m.matchedLogs, m.unmatchedLogs = 0, 0, 0, 0
	return r
}
func consumeTailerSources(added, removed <-chan *sources.LogSource, addedDone, removedDone, stop, stopped chan struct{}, matcher *tailerScopeMatcher, logger log.Component) {
	defer close(addedDone)
	defer close(removedDone)
	defer close(stopped)
	for {
		select {
		case <-stop:
			return
		case source := <-added:
			if source == nil {
				return
			}
			if source.Config == nil {
				matcher.registerInvalidSource(source, "missing_service_and_source")
				logger.Warnf("[observer.tailer_match] log source %q cannot receive scoped anomaly severity: missing_service_and_source", source.Name)
			} else if scope, reason := scopeFromStrings(source.Config.Service, source.Config.Source); reason != "" {
				matcher.registerInvalidSource(source, reason)
				logger.Warnf("[observer.tailer_match] log source %q cannot receive scoped anomaly severity: %s", source.Name, reason)
			} else {
				matcher.register(source, scope)
			}
		case source := <-removed:
			if source != nil {
				matcher.remove(source)
			}
		}
	}
}
