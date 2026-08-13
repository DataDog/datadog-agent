// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

// scopedAnomalyScorer is an internal correlator that partitions anomalies by
// their normalized service/source scope. It intentionally has no reporter or
// subscription output: the existing global scorer remains the sole producer of
// severity events and adaptive-sampling inputs for this experiment.
type scopedAnomalyScorer struct {
	config    AnomalyScorerConfig
	scopes    *scopeRegistry
	telemetry *observerTelemetry
	scorers   map[scopeKey]*anomalyScorer
}

var _ observerdef.Correlator = (*scopedAnomalyScorer)(nil)

func newScopedAnomalyScorer(config AnomalyScorerConfig, scopes *scopeRegistry, telemetry *observerTelemetry) *scopedAnomalyScorer {
	return &scopedAnomalyScorer{
		config:    config,
		scopes:    scopes,
		telemetry: telemetry,
		scorers:   make(map[scopeKey]*anomalyScorer),
	}
}

func (*scopedAnomalyScorer) Name() string { return "scoped_anomaly_scorer" }

func (s *scopedAnomalyScorer) ProcessAnomaly(anomaly observerdef.Anomaly) {
	scope := scopeFromTags(anomaly.Source.Tags, "", "")
	if !s.scopes.admit(scope, "anomaly") {
		return
	}
	scorer, found := s.scorers[scope]
	if !found {
		childConfig := s.config
		childConfig.Logs = false
		childConfig.CorrelationEvents = false
		scorer = newAnomalyScorerBase(childConfig)
		s.scorers[scope] = scorer
		s.scopes.setScorerCount(len(s.scorers))
	}
	scorer.ProcessAnomaly(anomaly)
	if s.telemetry != nil {
		s.telemetry.recordScopeAnomaly(scope)
	}
}

func (s *scopedAnomalyScorer) Advance(dataTime int64) {
	for scope, scorer := range s.scorers {
		scorer.Advance(dataTime)
		if s.telemetry != nil {
			s.telemetry.setScopeScorerScore(scope, scorer.LastScore())
		}
	}
}

func (*scopedAnomalyScorer) ActiveCorrelations() []observerdef.ActiveCorrelation { return nil }

func (*scopedAnomalyScorer) PendingEvents() []observerdef.CorrelatorEvent { return nil }

// Reset removes scorer state for a replay but deliberately retains the shared
// scope registry, including successful-tailer history, for the Agent lifetime.
func (s *scopedAnomalyScorer) Reset() {
	s.scorers = make(map[scopeKey]*anomalyScorer)
	s.scopes.setScorerCount(0)
}
