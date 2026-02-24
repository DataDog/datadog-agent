// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// RCAService computes RCA results from active correlation state.
type RCAService struct {
	config   RCAConfig
	builders *rcaBuilderRegistry

	mu      sync.RWMutex
	results map[string]RCAResult
}

// NewRCAService creates a new RCA service instance.
func NewRCAService(config RCAConfig) *RCAService {
	normalized := config.normalized()
	return &RCAService{
		config:   normalized,
		builders: newRCABuilderRegistry(normalized),
		results:  make(map[string]RCAResult),
	}
}

// Update recomputes RCA for the current active correlations.
func (s *RCAService) Update(correlations []observer.ActiveCorrelation) error {
	if s == nil || !s.config.Enabled {
		return nil
	}

	next := make(map[string]RCAResult)
	var firstErr error

	for _, correlation := range correlations {
		graph, supported, err := s.builders.build(correlation)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("build RCA graph for %s: %w", correlation.Pattern, err)
			}
			continue
		}
		if !supported || len(graph.Nodes) == 0 {
			continue
		}

		seriesCandidates := rankSeriesRootCandidates(graph, s.config)
		metricCandidates := rollupMetricRootCandidates(graph, seriesCandidates, s.config.MaxRootCandidates)
		evidencePaths := extractEvidencePaths(graph, seriesCandidates, s.config)
		confidence := buildRCAConfidence(graph, seriesCandidates, s.config)

		next[correlation.Pattern] = RCAResult{
			CorrelationPattern:   correlation.Pattern,
			RootCandidatesSeries: seriesCandidates,
			RootCandidatesMetric: metricCandidates,
			EvidencePaths:        evidencePaths,
			Confidence:           confidence,
			Summary:              buildRCASummary(correlation, seriesCandidates, metricCandidates, confidence),
		}
	}

	s.mu.Lock()
	s.results = next
	s.mu.Unlock()

	return firstErr
}

// Results returns a stable snapshot sorted by correlation pattern.
func (s *RCAService) Results() []RCAResult {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]RCAResult, 0, len(s.results))
	for _, result := range s.results {
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].CorrelationPattern < results[j].CorrelationPattern
	})
	return results
}

// ResultForPattern returns the current RCA result for one correlation pattern.
func (s *RCAService) ResultForPattern(pattern string) (RCAResult, bool) {
	if s == nil {
		return RCAResult{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result, ok := s.results[pattern]
	return result, ok
}

func buildRCASummary(correlation observer.ActiveCorrelation, series []RCARootCandidate, metric []RCARootCandidate, confidence RCAConfidence) string {
	if len(series) == 0 {
		return "insufficient temporal evidence to rank root candidates"
	}

	topSeries := series[0].ID
	topMetric := ""
	if len(metric) > 0 {
		topMetric = metric[0].ID
	}

	if topMetric == "" {
		return fmt.Sprintf("%s likely initiates %s (confidence %.2f)", topSeries, correlation.Pattern, confidence.Score)
	}

	return fmt.Sprintf("%s (metric %s) is the top likely root for %s (confidence %.2f)", topSeries, topMetric, correlation.Pattern, confidence.Score)
}
