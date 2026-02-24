// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRCAServiceUpdateProducesResultsForTimeCluster(t *testing.T) {
	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.MaxRootCandidates = 2
	cfg.MaxEvidencePaths = 2

	svc := NewRCAService(cfg)
	err := svc.Update([]observer.ActiveCorrelation{
		{
			Pattern: "time_cluster_3",
			Title:   "TimeCluster: 3 anomalies",
			Anomalies: []observer.AnomalyOutput{
				{Source: "metric.a", SourceSeriesID: "ns|metric.a|host:a", Timestamp: 100},
				{Source: "metric.b", SourceSeriesID: "ns|metric.b|host:b", Timestamp: 102},
				{Source: "metric.c", SourceSeriesID: "ns|metric.c|host:c", Timestamp: 104},
			},
			FirstSeen:   100,
			LastUpdated: 104,
		},
	})
	require.NoError(t, err)

	results := svc.Results()
	require.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "time_cluster_3", result.CorrelationPattern)
	assert.NotEmpty(t, result.RootCandidatesSeries)
	assert.NotEmpty(t, result.RootCandidatesMetric)
	assert.NotEmpty(t, result.EvidencePaths)
	assert.NotEmpty(t, result.Summary)
}

func TestRCAServiceDropsUnsupportedPatterns(t *testing.T) {
	cfg := DefaultRCAConfig()
	cfg.Enabled = true

	svc := NewRCAService(cfg)
	err := svc.Update([]observer.ActiveCorrelation{{Pattern: "kernel_bottleneck", Title: "Correlated: Kernel network bottleneck"}})
	require.NoError(t, err)
	assert.Empty(t, svc.Results())
}

func TestDigestHighConfidenceIncludesOnsetChainAndRankedSources(t *testing.T) {
	sources := []string{
		"app|metric.cpu|host:a",
		"app|metric.mem|host:b",
		"app|metric.io|host:c",
	}
	rca := RCAResult{
		RootCandidatesSeries: []RCARootCandidate{
			{ID: "app|metric.cpu|host:a", Score: 0.9, OnsetTime: 100, Why: []string{"earliest onset"}},
			{ID: "app|metric.mem|host:b", Score: 0.7, OnsetTime: 105, Why: []string{"high severity"}},
			{ID: "app|metric.io|host:c", Score: 0.5, OnsetTime: 110},
		},
		Confidence: RCAConfidence{Score: 0.75}, // above threshold
	}

	digest := buildCorrelationDigest(sources, rca)

	assert.GreaterOrEqual(t, digest.RCAConfidence, digestHighConfidenceThreshold)
	assert.NotEmpty(t, digest.OnsetChain, "high confidence should include onset chain")
	assert.NotEmpty(t, digest.KeySources, "high confidence should include key sources")

	// Key sources should have non-zero scores (from RCA ranking).
	assert.Greater(t, digest.KeySources[0].Score, 0.0)
	// Onset chain should be sorted by onset time.
	for i := 1; i < len(digest.OnsetChain); i++ {
		assert.LessOrEqual(t, digest.OnsetChain[i-1].OnsetTime, digest.OnsetChain[i].OnsetTime)
	}
}

func TestDigestLowConfidenceOmitsOnsetChainUsesFamilySamples(t *testing.T) {
	sources := []string{
		"app|metric.cpu|host:a",
		"app|metric.cpu|host:b",
		"app|metric.cpu|host:c",
		"app|metric.mem|host:d",
		"cleanup|metric.x|host:z",
	}
	rca := RCAResult{
		RootCandidatesSeries: []RCARootCandidate{
			{ID: "cleanup|metric.x|host:z", Score: 0.6, OnsetTime: 99, Why: []string{"earliest onset"}},
			{ID: "app|metric.cpu|host:a", Score: 0.55, OnsetTime: 100},
		},
		Confidence: RCAConfidence{Score: 0.45, AmbiguousRoots: true}, // below threshold
	}

	digest := buildCorrelationDigest(sources, rca)

	assert.Less(t, digest.RCAConfidence, digestHighConfidenceThreshold)
	assert.Empty(t, digest.OnsetChain, "low confidence should omit onset chain")
	assert.NotEmpty(t, digest.KeySources, "low confidence should still have key sources (family samples)")

	// Key sources should have zero scores (representative samples, not RCA-ranked).
	for _, ks := range digest.KeySources {
		assert.Equal(t, 0.0, ks.Score, "family samples should have zero score")
		assert.NotEmpty(t, ks.Why, "family samples should explain representativeness")
	}

	// The top metric family (metric.cpu with 3 series) should appear first.
	assert.Equal(t, "metric.cpu", digest.KeySources[0].MetricName,
		"most impacted metric family should appear first")
}

func TestRCAServiceAlwaysEmitsResults(t *testing.T) {
	cfg := DefaultRCAConfig()
	cfg.Enabled = true

	svc := NewRCAService(cfg)

	// Even with low confidence, Results() should return results
	// (digest compression replaces suppression as the quality mechanism).
	err := svc.Update([]observer.ActiveCorrelation{
		{
			Pattern: "time_cluster_always",
			Title:   "TimeCluster: 2 anomalies",
			Anomalies: []observer.AnomalyOutput{
				{Source: "metric.a", SourceSeriesID: "ns|metric.a|host:a", Timestamp: 100},
				{Source: "metric.b", SourceSeriesID: "ns|metric.b|host:b", Timestamp: 102},
			},
			FirstSeen:   100,
			LastUpdated: 102,
		},
	})
	require.NoError(t, err)

	results := svc.Results()
	require.Len(t, results, 1, "low-confidence results should still be emitted")
	assert.Equal(t, "time_cluster_always", results[0].CorrelationPattern)

	// Also accessible via ResultForPattern.
	r, ok := svc.ResultForPattern("time_cluster_always")
	assert.True(t, ok)
	assert.Greater(t, r.Confidence.Score, 0.0, "confidence should be computed")
}
