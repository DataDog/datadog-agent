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

// rankSource is a tiny helper to build a SeriesDescriptor for tests, mirroring
// the passthroughSource helper in anomaly_correlator_passthrough_test.go.
func rankSource(name string) observer.SeriesDescriptor {
	return observer.SeriesDescriptor{Name: name}
}

// rankAnomaly builds an Anomaly with an explicit Score and Timestamp. The
// Source is derived from the detector + score so each anomaly has a unique
// series identity, which keeps downstream Members/Source assertions clean.
func rankAnomaly(detector string, score float64, ts int64) observer.Anomaly {
	s := score // local for &
	return observer.Anomaly{
		DetectorName: detector,
		Source:       rankSource("metric.test"),
		Timestamp:    ts,
		Score:        &s,
	}
}

func TestAnomalyRank_Name(t *testing.T) {
	c := NewAnomalyRankCorrelator(DefaultAnomalyRankConfig())
	assert.Equal(t, "anomaly_rank_correlator", c.Name())
}

func TestAnomalyRank_ImplementsCorrelator(_ *testing.T) {
	var _ observer.Correlator = NewAnomalyRankCorrelator(DefaultAnomalyRankConfig())
}

// PassesAllWhenWithinTopK: with TopK=8 and quantile gating disabled, all 3
// anomalies for one detector are emitted.
func TestAnomalyRank_PassesAllWhenWithinTopK(t *testing.T) {
	c := NewAnomalyRankCorrelator(AnomalyRankConfig{
		WindowSeconds:   60,
		TopKPerDetector: 8,
		QuantileFloor:   1.0, // disable quantile gating
	})

	c.ProcessAnomaly(rankAnomaly("cusum", 1.0, 100))
	c.ProcessAnomaly(rankAnomaly("cusum", 5.0, 101))
	c.ProcessAnomaly(rankAnomaly("cusum", 9.0, 102))

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 3)
	// Emission is timestamp-ascending per detector.
	assert.Equal(t, int64(100), corrs[0].FirstSeen)
	assert.Equal(t, int64(101), corrs[1].FirstSeen)
	assert.Equal(t, int64(102), corrs[2].FirstSeen)
	// Patterns and titles use the documented format.
	assert.Equal(t, "anomaly_rank_cusum_0", corrs[0].Pattern)
	assert.Contains(t, corrs[0].Title, "Ranked[cusum]")
}

// PrunesBelowQuantile: 10 anomalies with scores 1..10, QuantileFloor=0.5 → top
// 5 (scores 6..10) survive.
func TestAnomalyRank_PrunesBelowQuantile(t *testing.T) {
	c := NewAnomalyRankCorrelator(AnomalyRankConfig{
		WindowSeconds:   60,
		TopKPerDetector: 100, // don't let TopK clamp before quantile gate
		QuantileFloor:   0.5,
	})

	for i := 1; i <= 10; i++ {
		c.ProcessAnomaly(rankAnomaly("cusum", float64(i), int64(100+i)))
	}

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 5)

	// Each correlation wraps a single anomaly; collect their scores.
	got := make([]float64, 0, len(corrs))
	for _, c := range corrs {
		require.Len(t, c.Anomalies, 1)
		require.NotNil(t, c.Anomalies[0].Score)
		got = append(got, *c.Anomalies[0].Score)
	}
	// Order is timestamp-ascending; scores 6..10 were inserted at ts 106..110.
	assert.Equal(t, []float64{6, 7, 8, 9, 10}, got)
}

// PerDetectorTopK: 12 anomalies for "scanmw" plus 12 for "bocpd"; TopK=4,
// QuantileFloor=1 → exactly 4 from each detector.
func TestAnomalyRank_PerDetectorTopK(t *testing.T) {
	c := NewAnomalyRankCorrelator(AnomalyRankConfig{
		WindowSeconds:   600,
		TopKPerDetector: 4,
		QuantileFloor:   1.0,
	})

	for i := 1; i <= 12; i++ {
		c.ProcessAnomaly(rankAnomaly("scanmw", float64(i), int64(100+i)))
		c.ProcessAnomaly(rankAnomaly("bocpd", float64(i), int64(200+i)))
	}

	corrs := c.ActiveCorrelations()
	// 4 from each = 8 total.
	require.Len(t, corrs, 8)

	perDetector := map[string]int{}
	for _, ac := range corrs {
		require.Len(t, ac.Anomalies, 1)
		perDetector[ac.Anomalies[0].DetectorName]++
	}
	assert.Equal(t, 4, perDetector["scanmw"])
	assert.Equal(t, 4, perDetector["bocpd"])
}

// AdvanceEvicts: anomaly at ts=10, Advance(80) with WindowSeconds=60 → cutoff
// is 20 → buffer becomes empty.
func TestAnomalyRank_AdvanceEvicts(t *testing.T) {
	c := NewAnomalyRankCorrelator(AnomalyRankConfig{
		WindowSeconds:   60,
		TopKPerDetector: 8,
		QuantileFloor:   1.0,
	})

	c.ProcessAnomaly(rankAnomaly("cusum", 5.0, 10))
	require.Len(t, c.ActiveCorrelations(), 1)

	c.Advance(80)
	assert.Empty(t, c.ActiveCorrelations(), "anomaly older than cutoff should be evicted")
}

// AdvanceKeepsInsideWindow: an anomaly inside the window survives Advance.
func TestAnomalyRank_AdvanceKeepsInsideWindow(t *testing.T) {
	c := NewAnomalyRankCorrelator(AnomalyRankConfig{
		WindowSeconds:   60,
		TopKPerDetector: 8,
		QuantileFloor:   1.0,
	})

	c.ProcessAnomaly(rankAnomaly("cusum", 5.0, 50))
	c.Advance(80) // cutoff = 80 - 60 = 20; ts=50 >= 20, so keep
	require.Len(t, c.ActiveCorrelations(), 1)
}

// MinScoreFloor: ProcessAnomaly drops anomalies below MinScore.
func TestAnomalyRank_MinScoreFloor(t *testing.T) {
	c := NewAnomalyRankCorrelator(AnomalyRankConfig{
		WindowSeconds:   60,
		TopKPerDetector: 8,
		MinScore:        1.0,
		QuantileFloor:   1.0,
	})

	c.ProcessAnomaly(rankAnomaly("cusum", 0.1, 100))
	assert.Empty(t, c.ActiveCorrelations(), "score below MinScore should be dropped")
	assert.Empty(t, c.perDetector, "MinScore-rejected anomalies must not be tracked")
}

// HandlesMissingScore: nil Score and nil DebugInfo → rankScore returns 0.
// MinScore=0 keeps it; MinScore>0 drops it.
func TestAnomalyRank_HandlesMissingScore(t *testing.T) {
	t.Run("kept when MinScore=0", func(t *testing.T) {
		c := NewAnomalyRankCorrelator(AnomalyRankConfig{
			WindowSeconds:   60,
			TopKPerDetector: 8,
			MinScore:        0,
			QuantileFloor:   1.0,
		})
		c.ProcessAnomaly(observer.Anomaly{
			DetectorName: "cusum",
			Source:       rankSource("m"),
			Timestamp:    100,
			// Score nil, DebugInfo nil → rankScore = 0
		})
		require.Len(t, c.ActiveCorrelations(), 1)
	})

	t.Run("dropped when MinScore>0", func(t *testing.T) {
		c := NewAnomalyRankCorrelator(AnomalyRankConfig{
			WindowSeconds:   60,
			TopKPerDetector: 8,
			MinScore:        0.0001,
			QuantileFloor:   1.0,
		})
		c.ProcessAnomaly(observer.Anomaly{
			DetectorName: "cusum",
			Source:       rankSource("m"),
			Timestamp:    100,
		})
		assert.Empty(t, c.ActiveCorrelations())
	})
}

// rankScore_PrefersExplicitScore: when Score is set, rankScore ignores
// DebugInfo. When Score is nil, falls back to |DeviationSigma|.
func TestAnomalyRank_RankScoreFallback(t *testing.T) {
	score := 3.5
	a1 := observer.Anomaly{Score: &score, DebugInfo: &observer.AnomalyDebugInfo{DeviationSigma: 99}}
	assert.InDelta(t, 3.5, rankScore(a1), 1e-9)

	a2 := observer.Anomaly{DebugInfo: &observer.AnomalyDebugInfo{DeviationSigma: -4}}
	assert.InDelta(t, 4.0, rankScore(a2), 1e-9, "absolute value of DeviationSigma when Score is nil")

	a3 := observer.Anomaly{}
	assert.InDelta(t, 0.0, rankScore(a3), 1e-9)
}

// Reset: clears all internal state.
func TestAnomalyRank_Reset(t *testing.T) {
	c := NewAnomalyRankCorrelator(AnomalyRankConfig{
		WindowSeconds:   60,
		TopKPerDetector: 8,
		QuantileFloor:   1.0,
	})
	c.ProcessAnomaly(rankAnomaly("cusum", 5.0, 100))
	require.Len(t, c.ActiveCorrelations(), 1)

	c.Reset()
	assert.Empty(t, c.ActiveCorrelations())
	assert.Equal(t, int64(0), c.currentDataTs)
}

// Empty: ActiveCorrelations on a fresh correlator returns no items.
func TestAnomalyRank_Empty(t *testing.T) {
	c := NewAnomalyRankCorrelator(DefaultAnomalyRankConfig())
	assert.Empty(t, c.ActiveCorrelations())
}

// CatalogRegistration: anomaly_rank is wired into the default catalog as a
// correlator and is enabled by default.
func TestAnomalyRank_CatalogRegistration(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "anomaly_rank" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "anomaly_rank must be registered in the default catalog")
	assert.Equal(t, componentCorrelator, found.kind)
	assert.True(t, found.defaultEnabled)
	// Factory must accept the typed config and return a Correlator.
	inst := found.factory(found.defaultConfig)
	_, ok := inst.(observer.Correlator)
	assert.True(t, ok, "anomaly_rank factory must produce an observer.Correlator")
}
