// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"sync"
)

// defaultMaxScopes bounds all scope-keyed live state. It deliberately remains
// finite even if an invalid config value is supplied.
const defaultMaxScopes = 4096

// scopeKey is the routing identity used by the scoped scorer experiment. An
// empty member represents an unset service or source.
type scopeKey struct {
	service string
	source  string
}

// scopeFromTags resolves one routing scope. Explicit values from a log Origin
// or log source configuration take precedence; otherwise the first tag for
// each key wins. This makes the result independent of later tag sorting.
func scopeFromTags(tags []string, service, source string) scopeKey {
	if service == "" || source == "" {
		for _, tag := range tags {
			key, value, ok := strings.Cut(tag, ":")
			if !ok || value == "" {
				continue
			}
			switch key {
			case "service":
				if service == "" {
					service = value
				}
			case "source":
				if source == "" {
					source = value
				}
			}
		}
	}
	return scopeKey{service: service, source: source}
}

// normalizeScopeTags returns an observer-owned tag copy where routing tags
// describe exactly scope. Future ingestion uses this before storage so
// canonical tag sorting cannot alter scope selection.
func normalizeScopeTags(tags []string, scope scopeKey) []string {
	normalized := make([]string, 0, len(tags)+2)
	for _, tag := range tags {
		key, _, ok := strings.Cut(tag, ":")
		if ok && (key == "service" || key == "source") {
			continue
		}
		normalized = append(normalized, tag)
	}
	if scope.service != "" {
		normalized = append(normalized, "service:"+scope.service)
	}
	if scope.source != "" {
		normalized = append(normalized, "source:"+scope.source)
	}
	return normalized
}

func (s scopeKey) telemetryTags() (service, source string) {
	service = s.service
	if service == "" {
		service = "none"
	}
	source = s.source
	if source == "" {
		source = "none"
	}
	return service, source
}

// scopeRegistry owns cardinality-driven state shared by scoped input
// telemetry, tailer registration, and scoped scorers. It never evicts: scope
// history intentionally remains available for the lifetime of the Agent.
type scopeRegistry struct {
	mu sync.RWMutex

	maxScopes int
	admitted  map[scopeKey]struct{}
	tailers   map[scopeKey]struct{}
	telemetry *observerTelemetry
}

func newScopeRegistry(maxScopes int, telemetry *observerTelemetry) *scopeRegistry {
	if maxScopes <= 0 {
		maxScopes = defaultMaxScopes
	}
	registry := &scopeRegistry{
		maxScopes: maxScopes,
		admitted:  make(map[scopeKey]struct{}),
		tailers:   make(map[scopeKey]struct{}),
		telemetry: telemetry,
	}
	if telemetry != nil {
		telemetry.setScopeAdmitted(0)
		telemetry.setScopeScorers(0)
	}
	return registry
}

// admit records scope if capacity permits. Overflow telemetry intentionally
// has no scope labels, avoiding a cardinality leak at the rejection boundary.
func (r *scopeRegistry) admit(scope scopeKey, kind string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.admitted[scope]; exists {
		return true
	}
	if len(r.admitted) >= r.maxScopes {
		if r.telemetry != nil {
			r.telemetry.recordScopeOverflow(kind)
		}
		return false
	}
	r.admitted[scope] = struct{}{}
	if r.telemetry != nil {
		r.telemetry.setScopeAdmitted(len(r.admitted))
	}
	return true
}

func (r *scopeRegistry) markTailerStarted(scope scopeKey) bool {
	if !r.admit(scope, "tailer") {
		return false
	}
	r.mu.Lock()
	r.tailers[scope] = struct{}{}
	r.mu.Unlock()
	return true
}

func (r *scopeRegistry) hasTailer(scope scopeKey) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.tailers[scope]
	return exists
}

func (r *scopeRegistry) setScorerCount(count int) {
	if r.telemetry != nil {
		r.telemetry.setScopeScorers(count)
	}
}

func scorerScopeLimit(scorer *anomalyScorer) int {
	if scorer == nil || scorer.config.MaxScopes <= 0 {
		return defaultMaxScopes
	}
	return scorer.config.MaxScopes
}
