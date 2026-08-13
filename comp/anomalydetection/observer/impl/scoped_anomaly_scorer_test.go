// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func scopedAnomaly(service, source string, timestamp int64) observer.Anomaly {
	return observer.Anomaly{
		Source:    observer.SeriesDescriptor{Tags: []string{"service:" + service, "source:" + source}},
		Timestamp: timestamp,
	}
}

func TestScopedAnomalyScorerCreatesOneScorerPerScope(t *testing.T) {
	registry := newScopeRegistry(4, nil)
	scorer := newScopedAnomalyScorer(DefaultAnomalyScorerConfig(), registry, nil)
	scorer.ProcessAnomaly(scopedAnomaly("api", "nginx", 10))
	scorer.ProcessAnomaly(scopedAnomaly("api", "nginx", 11))
	scorer.ProcessAnomaly(scopedAnomaly("worker", "app", 11))
	if got, want := len(scorer.scorers), 2; got != want {
		t.Fatalf("scorer count = %d, want %d", got, want)
	}
	for _, child := range scorer.scorers {
		if child.config.Logs || child.config.CorrelationEvents || child.topAnomalies != nil {
			t.Fatal("scoped child must not produce logs, events, or event buffers")
		}
	}
}

func TestScopedAnomalyScorerRespectsScopeCapacity(t *testing.T) {
	registry := newScopeRegistry(1, nil)
	scorer := newScopedAnomalyScorer(DefaultAnomalyScorerConfig(), registry, nil)
	scorer.ProcessAnomaly(scopedAnomaly("api", "nginx", 10))
	scorer.ProcessAnomaly(scopedAnomaly("worker", "app", 10))
	if got, want := len(scorer.scorers), 1; got != want {
		t.Fatalf("scorer count = %d, want %d", got, want)
	}
}

func TestScopedAnomalyScorerAdvancesAndResetPreservesTailerHistory(t *testing.T) {
	registry := newScopeRegistry(4, nil)
	tailerScope := scopeKey{service: "api", source: "nginx"}
	if !registry.markTailerStarted(tailerScope) {
		t.Fatal("failed to register tailer scope")
	}
	scorer := newScopedAnomalyScorer(DefaultAnomalyScorerConfig(), registry, nil)
	scorer.ProcessAnomaly(scopedAnomaly("api", "nginx", 10))
	scorer.Advance(10)
	child := scorer.scorers[tailerScope]
	if child == nil || child.LastScore() <= 0 {
		t.Fatal("expected scoped scorer to advance its child")
	}
	scorer.Reset()
	if len(scorer.scorers) != 0 {
		t.Fatal("reset did not clear scoped scorers")
	}
	if !registry.hasTailer(tailerScope) {
		t.Fatal("reset unexpectedly cleared tailer history")
	}
}
