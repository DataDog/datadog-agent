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

// anchorTrustTestConfig returns a config with a known proximity / window so
// tests don't depend on the package defaults.
func anchorTrustTestConfig(anchors []string, minDistinct int) AnchorTrustConfig {
	return AnchorTrustConfig{
		TimeClusterConfig: TimeClusterConfig{
			ProximitySeconds: 60,
			WindowSeconds:    600,
		},
		AnchorDetectors:      anchors,
		MinDistinctDetectors: minDistinct,
	}
}

func anchorTrustAnomaly(detector string, ts int64) observer.Anomaly {
	return observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Name: detector + ".series"},
		DetectorName: detector,
		Timestamp:    ts,
	}
}

// TestAnchorTrustCorrelator_AnchorPreservesNarrowSpan: the cluster spans 100..130
// from raw clustering, but because bocpd_detector (anchor) only fired at t=100,
// the emitted FirstSeen / LastUpdated must be 100 / 100 — protecting bocpd's
// narrow ground-truth window from the wider cluster span.
func TestAnchorTrustCorrelator_AnchorPreservesNarrowSpan(t *testing.T) {
	c := NewAnchorTrustCorrelator(anchorTrustTestConfig([]string{"bocpd_detector"}, 2))

	c.ProcessAnomaly(anchorTrustAnomaly("bocpd_detector", 100))
	c.ProcessAnomaly(anchorTrustAnomaly("scanwelch", 130))

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1)
	assert.Equal(t, int64(100), corrs[0].FirstSeen, "FirstSeen must be the anchor's timestamp, not the cluster min")
	assert.Equal(t, int64(100), corrs[0].LastUpdated, "LastUpdated must be the anchor's timestamp, not the cluster max")
	assert.Len(t, corrs[0].Anomalies, 2, "all anomalies should still pass through to the scorer")
}

// TestAnchorTrustCorrelator_QuorumFiresOnTwoNonAnchorDetectors: no anchor in
// the cluster, but two distinct non-anchor detectors satisfy the quorum, so
// the cluster is emitted unchanged (FirstSeen / LastUpdated are the raw
// cluster span).
func TestAnchorTrustCorrelator_QuorumFiresOnTwoNonAnchorDetectors(t *testing.T) {
	c := NewAnchorTrustCorrelator(anchorTrustTestConfig([]string{"bocpd_detector"}, 2))

	c.ProcessAnomaly(anchorTrustAnomaly("scanwelch", 200))
	c.ProcessAnomaly(anchorTrustAnomaly("holt_residual", 205))

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1)
	assert.Equal(t, int64(200), corrs[0].FirstSeen, "non-anchor cluster keeps its full span")
	assert.Equal(t, int64(205), corrs[0].LastUpdated, "non-anchor cluster keeps its full span")
}

// TestAnchorTrustCorrelator_SingleNonAnchorDropped: a single non-anchor
// detector firing in isolation does not meet quorum and is dropped.
func TestAnchorTrustCorrelator_SingleNonAnchorDropped(t *testing.T) {
	c := NewAnchorTrustCorrelator(anchorTrustTestConfig([]string{"bocpd_detector"}, 2))

	c.ProcessAnomaly(anchorTrustAnomaly("ks_drift", 300))

	corrs := c.ActiveCorrelations()
	assert.Empty(t, corrs, "single-detector non-anchor cluster must be dropped")
}

// TestAnchorTrustCorrelator_MultipleAnchorsAndNonAnchor: with two anchor
// anomalies at t=400 and t=420 plus a non-anchor at t=410 (all clustered),
// the emitted span is the anchor min/max (400..420), even though we add
// another non-anchor anomaly at t=460 that pushes the cluster's full span
// further out. The anchor span must NOT be 460.
func TestAnchorTrustCorrelator_MultipleAnchorsAndNonAnchor(t *testing.T) {
	c := NewAnchorTrustCorrelator(anchorTrustTestConfig([]string{"bocpd_detector"}, 2))

	c.ProcessAnomaly(anchorTrustAnomaly("bocpd_detector", 400))
	c.ProcessAnomaly(anchorTrustAnomaly("holt_residual", 410))
	c.ProcessAnomaly(anchorTrustAnomaly("bocpd_detector", 420))
	c.ProcessAnomaly(anchorTrustAnomaly("holt_residual", 460))

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1)
	assert.Equal(t, int64(400), corrs[0].FirstSeen, "FirstSeen is anchor min")
	assert.Equal(t, int64(420), corrs[0].LastUpdated, "LastUpdated is anchor max, not cluster max (460)")
	assert.Len(t, corrs[0].Anomalies, 4, "all 4 anomalies still pass through")
}

// TestAnchorTrustCorrelator_ResetPropagates: Reset() must clear the inner
// correlator's state.
func TestAnchorTrustCorrelator_ResetPropagates(t *testing.T) {
	c := NewAnchorTrustCorrelator(anchorTrustTestConfig([]string{"bocpd_detector"}, 2))

	c.ProcessAnomaly(anchorTrustAnomaly("bocpd_detector", 500))
	require.NotEmpty(t, c.ActiveCorrelations())

	c.Reset()
	assert.Empty(t, c.ActiveCorrelations(), "Reset must clear inner state")
}

// TestAnchorTrustCorrelator_AdvancePropagates: Advance() must reach the inner
// correlator and trigger window eviction.
func TestAnchorTrustCorrelator_AdvancePropagates(t *testing.T) {
	c := NewAnchorTrustCorrelator(AnchorTrustConfig{
		TimeClusterConfig: TimeClusterConfig{
			ProximitySeconds: 5,
			WindowSeconds:    30,
		},
		AnchorDetectors:      []string{"bocpd_detector"},
		MinDistinctDetectors: 2,
	})

	c.ProcessAnomaly(anchorTrustAnomaly("bocpd_detector", 100))
	require.Len(t, c.ActiveCorrelations(), 1)

	// Advance past the window — eviction should drop the cluster.
	c.Advance(200)
	assert.Empty(t, c.ActiveCorrelations(), "Advance past window must evict old clusters")
}

// TestAnchorTrustCorrelator_ZeroValueConfig: a zero AnchorTrustConfig must
// still produce a working correlator with bocpd as the anchor and quorum=2.
func TestAnchorTrustCorrelator_ZeroValueConfig(t *testing.T) {
	c := NewAnchorTrustCorrelator(AnchorTrustConfig{})
	require.NotNil(t, c)
	require.NotNil(t, c.inner, "inner correlator must be constructed")
	require.Contains(t, c.anchorSet, "bocpd_detector", "default anchor must be applied")
	assert.Equal(t, 2, c.minDistinctDet, "default quorum must be 2")

	// Anchor at t=600 with a non-anchor at t=610 — anchor span wins.
	c.ProcessAnomaly(anchorTrustAnomaly("bocpd_detector", 600))
	c.ProcessAnomaly(anchorTrustAnomaly("scanwelch", 610))
	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1)
	assert.Equal(t, int64(600), corrs[0].FirstSeen)
	assert.Equal(t, int64(600), corrs[0].LastUpdated)
}

// TestAnchorTrustCorrelator_DefaultConfigDefaults: spot-check the public
// Default constructor — these are the values the catalog passes in by default.
func TestAnchorTrustCorrelator_DefaultConfigDefaults(t *testing.T) {
	cfg := DefaultAnchorTrustConfig()
	assert.Equal(t, []string{"bocpd_detector"}, cfg.AnchorDetectors)
	assert.Equal(t, 2, cfg.MinDistinctDetectors)
	assert.Equal(t, int64(10), cfg.ProximitySeconds, "embedded TimeClusterConfig defaults must apply")
	assert.Equal(t, int64(120), cfg.WindowSeconds)
}

// TestAnchorTrustCorrelator_Name: the Name() must match the catalog key so
// testbench --only and config readers stay aligned.
func TestAnchorTrustCorrelator_Name(t *testing.T) {
	c := NewAnchorTrustCorrelator(DefaultAnchorTrustConfig())
	assert.Equal(t, "anchor_trust_correlator", c.Name())
}

// TestAnchorTrustCorrelator_ImplementsCorrelator is a compile-time guard that
// the type satisfies the observer.Correlator interface.
func TestAnchorTrustCorrelator_ImplementsCorrelator(_ *testing.T) {
	var _ observer.Correlator = NewAnchorTrustCorrelator(DefaultAnchorTrustConfig())
}

// TestAnchorTrustCorrelator_NonAnchorWithThreeDetectorsEmits: a non-anchor
// cluster with three distinct detectors should still pass quorum.
func TestAnchorTrustCorrelator_NonAnchorWithThreeDetectorsEmits(t *testing.T) {
	c := NewAnchorTrustCorrelator(anchorTrustTestConfig([]string{"bocpd_detector"}, 2))

	c.ProcessAnomaly(anchorTrustAnomaly("scanwelch", 700))
	c.ProcessAnomaly(anchorTrustAnomaly("holt_residual", 710))
	c.ProcessAnomaly(anchorTrustAnomaly("ks_drift", 720))

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1)
	assert.Equal(t, int64(700), corrs[0].FirstSeen)
	assert.Equal(t, int64(720), corrs[0].LastUpdated)
}

// TestAnchorTrustCorrelator_SameDetectorTwiceFailsQuorum: two anomalies from
// the same non-anchor detector do NOT satisfy quorum (which requires distinct
// DetectorName values).
func TestAnchorTrustCorrelator_SameDetectorTwiceFailsQuorum(t *testing.T) {
	c := NewAnchorTrustCorrelator(anchorTrustTestConfig([]string{"bocpd_detector"}, 2))

	c.ProcessAnomaly(anchorTrustAnomaly("scanwelch", 800))
	c.ProcessAnomaly(anchorTrustAnomaly("scanwelch", 810))

	assert.Empty(t, c.ActiveCorrelations(), "two anomalies from one non-anchor detector must not satisfy quorum")
}
