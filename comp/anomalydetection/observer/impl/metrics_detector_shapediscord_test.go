// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"math/rand"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testShapeDiscordDetector returns a detector restricted to AggregateAverage
// so each test only sees one state entry per series — keeps anomaly-count
// assertions unambiguous.
func testShapeDiscordDetector() *ShapeDiscordDetector {
	d := NewShapeDiscordDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedShapeDiscord adds values to fresh storage with consecutive timestamps
// starting at t=1 and runs Detect once at the final timestamp. ShapeDiscord
// is invariant to translation (z-normalised), so a positive offset keeps
// values inside the storage's accepted finite range without affecting the
// detector.
func feedShapeDiscord(t *testing.T, d *ShapeDiscordDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

func TestShapeDiscord_Name(t *testing.T) {
	d := NewShapeDiscordDetector()
	assert.Equal(t, "shapediscord", d.Name())
}

// TestShapeDiscord_FlagsShapeDiscordOnSawtoothBurst feeds a smooth sine
// baseline followed by a brief, sharp sawtooth segment. The sawtooth has the
// same approximate range as the sine but a structurally different shape per
// 16-point window — exactly the signal class this detector targets. We
// expect at least one anomaly to fire inside or shortly after the burst.
func TestShapeDiscord_FlagsShapeDiscordOnSawtoothBurst(t *testing.T) {
	const baseline = 200
	const burst = 40

	values := make([]float64, 0, baseline+burst+20)
	// Sine baseline — smooth, locally near-monotonic on the m=16 scale.
	for i := 0; i < baseline; i++ {
		values = append(values, math.Sin(float64(i)*0.15))
	}
	// Sawtooth burst — flips direction every step. The z-normalised shape
	// is dramatically different from any sine subsequence in the anchors.
	for i := 0; i < burst; i++ {
		if i%2 == 0 {
			values = append(values, 2.0)
		} else {
			values = append(values, -2.0)
		}
	}
	// Trailing sine so we can verify the alert clears (and to satisfy the
	// "burst window" timestamp assertion below).
	for i := 0; i < 20; i++ {
		values = append(values, math.Sin(float64(i)*0.15))
	}

	d := testShapeDiscordDetector()
	result := feedShapeDiscord(t, d, "sawtooth_burst", values)

	require.NotEmpty(t, result.Anomalies, "sawtooth burst should fire at least once")
	a := result.Anomalies[0]
	assert.Equal(t, "shapediscord", a.DetectorName)
	assert.Contains(t, a.Title, "ShapeDiscord")
	require.NotNil(t, a.DebugInfo)
	assert.Greater(t, a.DebugInfo.DeviationSigma, a.DebugInfo.Threshold,
		"firing tick must clear the Z-sigma trigger threshold")
	// Trigger must occur after the burst starts (ts > baseline) and within
	// a reasonable window after the burst tail. The persistence ring needs
	// PersistenceK=4 over-threshold ticks, so allow a small lag.
	assert.Greater(t, a.Timestamp, int64(baseline),
		"anomaly timestamp must be in or after the burst")
	assert.LessOrEqual(t, a.Timestamp, int64(baseline+burst+10),
		"anomaly timestamp must be within the burst neighbourhood")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestShapeDiscord_NoFireOnSinusoidalBaseline feeds 400 points of a steady
// sine with a small noise floor. Every m-window is shape-similar to its
// neighbours, so the min-anchor distance never escapes the rolling baseline
// and 0 anomalies are expected.
func TestShapeDiscord_NoFireOnSinusoidalBaseline(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	values := make([]float64, 400)
	for i := range values {
		values[i] = math.Sin(float64(i)*0.1) + 0.02*rng.NormFloat64()
	}

	d := testShapeDiscordDetector()
	result := feedShapeDiscord(t, d, "sin_baseline", values)

	assert.Empty(t, result.Anomalies,
		"steady sinusoid should not trigger any shapediscord anomaly")
}

// TestShapeDiscord_DeterministicAnchorReplacement runs the detector twice
// over identical input streams and asserts the resulting anomaly counts and
// timestamps match. This pins the deterministic-LCG contract: tests and
// downstream tooling must produce reproducible anomaly streams from a given
// input.
func TestShapeDiscord_DeterministicAnchorReplacement(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	values := make([]float64, 0, 260)
	for i := 0; i < 200; i++ {
		values = append(values, math.Sin(float64(i)*0.13)+0.05*rng.NormFloat64())
	}
	for i := 0; i < 30; i++ {
		// A short alternating pattern — predictably anomalous.
		if i%2 == 0 {
			values = append(values, 3.0)
		} else {
			values = append(values, -3.0)
		}
	}
	for i := 0; i < 30; i++ {
		values = append(values, math.Sin(float64(i)*0.13))
	}

	d1 := testShapeDiscordDetector()
	r1 := feedShapeDiscord(t, d1, "det", values)

	d2 := testShapeDiscordDetector()
	r2 := feedShapeDiscord(t, d2, "det", values)

	require.Equal(t, len(r1.Anomalies), len(r2.Anomalies),
		"identical input must produce identical anomaly counts")
	for i := range r1.Anomalies {
		assert.Equal(t, r1.Anomalies[i].Timestamp, r2.Anomalies[i].Timestamp,
			"anomaly %d timestamp must be deterministic", i)
		assert.Equal(t, r1.Anomalies[i].DebugInfo.DeviationSigma,
			r2.Anomalies[i].DebugInfo.DeviationSigma,
			"anomaly %d score must be deterministic", i)
	}
}

// TestShapeDiscord_RecoverAndRefire exercises the alert lifecycle: a burst,
// followed by a long quiet stretch, followed by a second burst. The
// detector should fire once per burst (alert raised → recovery clears it →
// alert re-raised on the next burst).
func TestShapeDiscord_RecoverAndRefire(t *testing.T) {
	const baseline = 200
	const burst = 30
	const recovery = 120

	values := make([]float64, 0, baseline+burst+recovery+burst+30)
	// Smooth baseline.
	for i := 0; i < baseline; i++ {
		values = append(values, math.Sin(float64(i)*0.12))
	}
	burstStart1 := int64(len(values))
	// First burst — sharp alternation.
	for i := 0; i < burst; i++ {
		if i%2 == 0 {
			values = append(values, 2.5)
		} else {
			values = append(values, -2.5)
		}
	}
	burstEnd1 := int64(len(values))
	// Long recovery — back to a smooth sine. Must be long enough for the
	// rolling min-dist baseline to refill with calm samples and for
	// RecoveryPoints=12 consecutive low-score ticks to clear the alert.
	for i := 0; i < recovery; i++ {
		values = append(values, math.Sin(float64(i)*0.12))
	}
	burstStart2 := int64(len(values))
	// Second burst.
	for i := 0; i < burst; i++ {
		if i%2 == 0 {
			values = append(values, 2.5)
		} else {
			values = append(values, -2.5)
		}
	}
	burstEnd2 := int64(len(values))
	for i := 0; i < 30; i++ {
		values = append(values, math.Sin(float64(i)*0.12))
	}

	d := testShapeDiscordDetector()
	result := feedShapeDiscord(t, d, "two_bursts", values)

	require.Len(t, result.Anomalies, 2,
		"two distinct bursts separated by a calm recovery should fire exactly twice")

	// Timestamps are 1-indexed in feedShapeDiscord (i+1), so a burst at
	// indices [burstStart, burstEnd) maps to timestamps in
	// (burstStart, burstEnd]. Allow a small post-burst lag for the
	// persistence ring to fill on each burst's first detection.
	const slack = 8
	a1 := result.Anomalies[0]
	a2 := result.Anomalies[1]
	assert.Greater(t, a1.Timestamp, burstStart1,
		"first anomaly must be inside or after the first burst")
	assert.LessOrEqual(t, a1.Timestamp, burstEnd1+slack,
		"first anomaly must not be far after the first burst")
	assert.Greater(t, a2.Timestamp, burstStart2,
		"second anomaly must be inside or after the second burst")
	assert.LessOrEqual(t, a2.Timestamp, burstEnd2+slack,
		"second anomaly must not be far after the second burst")
}

// TestShapeDiscord_RemoveSeries verifies that RemoveSeries shrinks the
// per-series state map — the SeriesRemover contract that keeps detector-side
// memory in step with storage eviction. Without this, the catalog teardown
// contract test in component_catalog_test.go would also fail (it calls
// validateDetectorTeardownContract against the live catalog).
func TestShapeDiscord_RemoveSeries(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := make([]float64, 200)
	for i := range values {
		values[i] = rng.NormFloat64()
	}

	d := testShapeDiscordDetector()
	storage := newTimeSeriesStorage()
	for i, v := range values {
		storage.Add("ns", "metric", 100+v, int64(i+1), nil)
	}
	d.Detect(storage, int64(len(values)))
	require.NotEmpty(t, d.series, "detector must record per-series state during Detect")

	var refs []observer.SeriesRef
	for k := range d.series {
		refs = append(refs, k.ref)
	}
	d.RemoveSeries(refs)
	assert.Empty(t, d.series, "RemoveSeries must drop state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries must invalidate cachedSeries")
}

// TestShapeDiscord_Reset verifies that Reset clears all per-series state.
func TestShapeDiscord_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := make([]float64, 100)
	for i := range values {
		values[i] = rng.NormFloat64()
	}

	d := testShapeDiscordDetector()
	feedShapeDiscord(t, d, "metric", values)
	require.NotEmpty(t, d.series, "should have state after detection")

	d.Reset()
	assert.Empty(t, d.series, "reset should clear all state")
	assert.Nil(t, d.cachedSeries, "reset should clear cached series")
}
