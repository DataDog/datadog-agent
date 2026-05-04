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

// testPHTDetector returns a detector restricted to AggregateAverage so each
// test only sees one state entry per series — keeps anomaly-count assertions
// unambiguous.
func testPHTDetector() *PHTDetector {
	d := NewPHTDetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	return d
}

// feedPHTSeries feeds values into a single-series storage with consecutive
// timestamps starting at t=1 and runs Detect once at the final timestamp.
func feedPHTSeries(t *testing.T, d *PHTDetector, name string, values []float64) observer.DetectionResult {
	t.Helper()
	storage := newTimeSeriesStorage()
	// Positive offset keeps values in the storage's accepted range; PHT is
	// translation-invariant so the offset has no effect on detection.
	const offset = 100.0
	for i, v := range values {
		storage.Add("ns", name, offset+v, int64(i+1), nil)
	}
	return d.Detect(storage, int64(len(values)))
}

func TestPHT_Name(t *testing.T) {
	d := NewPHTDetector()
	assert.Equal(t, "pht", d.Name())
}

// TestPHT_WhiteNoise_NoFire: 1000 i.i.d. N(0,1) → 0 anomalies. PHT must
// tolerate stationary noise: the cumulative deviation grows like a √t random
// walk reflected at zero, and λ=50·σ̂ is well above any plausible noise gap
// over 1000 ticks.
func TestPHT_WhiteNoise_NoFire(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	values := make([]float64, 1000)
	for i := range values {
		values[i] = rng.NormFloat64()
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "noise", values)

	assert.Empty(t, result.Anomalies, "stationary white noise should not trigger PHT")
}

// TestPHT_ConstantSeries_NoFire: 500 ticks of the same value → 0 anomalies.
// This is the σ̂ floor case: the P² estimator hovers at 0, so the trigger
// reduces to (m_t−M_t) > λ·1e-9 which is unreachable from m=0/M=0.
func TestPHT_ConstantSeries_NoFire(t *testing.T) {
	values := make([]float64, 500)
	for i := range values {
		values[i] = 7.0
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "const", values)

	assert.Empty(t, result.Anomalies, "constant series must not trigger PHT")
}

// TestPHT_StepUp_Fires: 600 N(0,1) followed by 600 N(0,1)+10 → exactly one
// anomaly during/just after the transition. A 10σ step is far above the slow
// drift PHT is tuned for, but it's the canonical positive control for
// sequential change-point tests.
func TestPHT_StepUp_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := make([]float64, 0, 1200)
	for i := 0; i < 600; i++ {
		values = append(values, rng.NormFloat64())
	}
	for i := 0; i < 600; i++ {
		values = append(values, 10.0+rng.NormFloat64())
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "step_up", values)

	require.Len(t, result.Anomalies, 1, "one positive step should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "pht", a.DetectorName)
	assert.Contains(t, a.Title, "PHT")
	require.NotNil(t, a.DebugInfo)
	assert.Greater(t, a.DebugInfo.DeviationSigma, 0.0,
		"deviation in σ-equivalent units must be positive at fire time")
	assert.Greater(t, a.Timestamp, int64(600), "fire must happen after the regime change")
	require.NotNil(t, a.SourceRef, "SourceRef must be populated by Detect")
}

// TestPHT_StepDown_Fires: same shape as StepUp but the post-change mean is
// −10. Verifies the symmetric (negative-side) trigger path, which is its own
// counter and ring.
func TestPHT_StepDown_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := make([]float64, 0, 1200)
	for i := 0; i < 600; i++ {
		values = append(values, rng.NormFloat64())
	}
	for i := 0; i < 600; i++ {
		values = append(values, -10.0+rng.NormFloat64())
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "step_down", values)

	require.Len(t, result.Anomalies, 1, "one negative step should fire exactly once")
	a := result.Anomalies[0]
	assert.Equal(t, "pht", a.DetectorName)
	assert.Greater(t, a.Timestamp, int64(600))
	require.NotNil(t, a.DebugInfo)
	assert.Contains(t, a.Description, "downward",
		"negative-side fire must be labelled downward in the description")
}

// TestPHT_SlowRamp_Fires: a slow linear ramp climbing 5 units over 800 ticks
// after a 200-tick stationary prefix. This is the bread-and-butter case PHT
// targets: the EWMA can't keep up with a steady drift, deviations accumulate.
// Mean-baseline detectors (CUSUM with fixed reference, BOCPD with hazard
// 0.05) tend to miss this pattern; we verify PHT fires at least once.
func TestPHT_SlowRamp_Fires(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	values := make([]float64, 0, 1000)
	for i := 0; i < 200; i++ {
		values = append(values, rng.NormFloat64())
	}
	// Ramp climbs from 0 to 5 over 800 ticks, slope 5/800 = 0.00625 per tick.
	for i := 0; i < 800; i++ {
		drift := float64(i) * (5.0 / 800.0)
		values = append(values, drift+rng.NormFloat64())
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "slow_ramp", values)

	require.NotEmpty(t, result.Anomalies, "slow positive ramp should fire at least once")
	a := result.Anomalies[0]
	assert.Equal(t, "pht", a.DetectorName)
	assert.Greater(t, a.Timestamp, int64(200), "fire must be in the ramp regime, not the prefix")
}

// TestPHT_AlertSuppression_NoReFire: a permanent step change should produce
// exactly one anomaly even when the post-change regime continues for many
// times the recovery window. This locks in the inAlert/freeze contract — a
// shift that never recovers must not emit a flood of repeated anomalies.
func TestPHT_AlertSuppression_NoReFire(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	values := make([]float64, 0, 2000)
	for i := 0; i < 200; i++ {
		values = append(values, rng.NormFloat64())
	}
	for i := 0; i < 1800; i++ {
		values = append(values, 8.0+rng.NormFloat64())
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "permastep", values)

	require.Len(t, result.Anomalies, 1,
		"a permanent step must produce exactly one anomaly while alert is held")
}

// TestPHT_WarmupGate: a step that occurs entirely within the warmup window
// must not fire. WarmupPoints is 60 by default; a step at t=10 with only 50
// total points should be silent because the σ̂ estimator has not stabilised.
func TestPHT_WarmupGate(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	values := make([]float64, 0, 50)
	for i := 0; i < 10; i++ {
		values = append(values, rng.NormFloat64())
	}
	for i := 0; i < 40; i++ {
		values = append(values, 100.0+rng.NormFloat64())
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "warmup", values)

	assert.Empty(t, result.Anomalies, "no fire is allowed inside the warmup window")
}

// TestPHT_RemoveSeries: SeriesRemover contract — RemoveSeries must drop
// per-series state and invalidate the cached series list.
func TestPHT_RemoveSeries(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	values := make([]float64, 200)
	for i := range values {
		values[i] = rng.NormFloat64()
	}

	d := testPHTDetector()
	storage := newTimeSeriesStorage()
	for i, v := range values {
		storage.Add("ns", "metric", 100+v, int64(i+1), nil)
	}
	d.Detect(storage, int64(len(values)))
	require.NotEmpty(t, d.series, "Detect must populate per-series state")

	var refs []observer.SeriesRef
	for k := range d.series {
		refs = append(refs, k.ref)
	}
	d.RemoveSeries(refs)
	assert.Empty(t, d.series, "RemoveSeries must drop state for freed refs")
	assert.Nil(t, d.cachedSeries, "RemoveSeries must invalidate cachedSeries")
}

// TestPHT_Reset: Reset clears all per-series state.
func TestPHT_Reset(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	values := make([]float64, 80)
	for i := range values {
		values[i] = rng.NormFloat64()
	}

	d := testPHTDetector()
	feedPHTSeries(t, d, "metric", values)
	require.NotEmpty(t, d.series, "expected per-series state after Detect")

	d.Reset()
	assert.Empty(t, d.series, "Reset must clear per-series state")
	assert.Nil(t, d.cachedSeries, "Reset must clear cached series")
}

// TestPHT_EnsureDefaults: a zero-valued struct must be usable. ensureDefaults
// is invoked from RemoveSeries / Detect so the public API tolerates &PHTDetector{}.
func TestPHT_EnsureDefaults(t *testing.T) {
	d := &PHTDetector{}
	d.ensureDefaults()
	assert.Equal(t, phtLambda, d.Lambda)
	assert.Equal(t, phtPersistenceK, d.PersistenceK)
	assert.Equal(t, phtRecoveryPoints, d.RecoveryPoints)
	assert.Equal(t, phtWarmupPoints, d.WarmupPoints)
	assert.Equal(t,
		[]observer.Aggregate{observer.AggregateAverage, observer.AggregateCount},
		d.Aggregations)
	assert.NotNil(t, d.series)
}

// TestPHT_RingMin_BasicMonotone: pushing a strictly decreasing sequence into
// the ring keeps the cached min at every push. Documents the fast-path
// "new min beats cached min" branch.
func TestPHT_RingMin_BasicMonotone(t *testing.T) {
	var r phtRingMin
	for i := 0; i < 50; i++ {
		r.push(float64(-i))
		assert.Equal(t, float64(-i), r.minValue(),
			"min must track the running minimum on a strictly decreasing input")
	}
}

// TestPHT_RingMin_EvictionRecompute: filling the ring with an increasing
// ramp, then continuing past capacity, must trigger argmin recompute when
// the original minimum (the first value pushed) gets evicted. After 2W
// pushes the ring contents should correspond to indices [W, 2W) and the
// minimum should be float64(W).
func TestPHT_RingMin_EvictionRecompute(t *testing.T) {
	var r phtRingMin
	// Push 0,1,...,2W-1 — first W fill the ring, second W evict.
	for i := 0; i < 2*phtMinWindow; i++ {
		r.push(float64(i))
	}
	// After 2W pushes, ring holds {W, W+1, ..., 2W-1}, so min = W.
	assert.Equal(t, float64(phtMinWindow), r.minValue(),
		"after eviction the minimum must reflect remaining entries")
}

// TestPHT_RingMin_EmptyMinIsZero: an empty ring must read 0, matching the
// PHT convention M_0 = 0. Guarantees that the very first comparison
// (m_0 − M_0) doesn't pull from +Inf and over-trigger.
func TestPHT_RingMin_EmptyMinIsZero(t *testing.T) {
	var r phtRingMin
	assert.Equal(t, 0.0, r.minValue(), "empty ring's min must be 0 by PHT convention")
}

// TestPHT_RingMin_Reset: reset returns the ring to the empty state.
func TestPHT_RingMin_Reset(t *testing.T) {
	var r phtRingMin
	for i := 0; i < 100; i++ {
		r.push(float64(i))
	}
	r.reset()
	assert.Equal(t, 0.0, r.minValue(), "reset must clear cached min back to 0")
	// And a subsequent push must seed a fresh argmin without surprises.
	r.push(42.0)
	assert.Equal(t, 42.0, r.minValue(), "post-reset min must reflect the next push")
}

// TestPHT_DeviationSigma_AtFire: at fire time, gap/σ̂ must exceed Lambda
// (that's the trigger condition). Documents the relationship between the
// reported DeviationSigma and the configured Lambda threshold.
func TestPHT_DeviationSigma_AtFire(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	values := make([]float64, 0, 1200)
	for i := 0; i < 600; i++ {
		values = append(values, rng.NormFloat64())
	}
	for i := 0; i < 600; i++ {
		values = append(values, 10.0+rng.NormFloat64())
	}

	d := testPHTDetector()
	result := feedPHTSeries(t, d, "sigma_check", values)
	require.NotEmpty(t, result.Anomalies)
	a := result.Anomalies[0]
	require.NotNil(t, a.DebugInfo)
	assert.Greater(t, a.DebugInfo.DeviationSigma, d.Lambda,
		"DeviationSigma at fire must exceed λ — that's the trigger inequality")
	assert.False(t, math.IsNaN(a.DebugInfo.DeviationSigma),
		"DeviationSigma must be finite (σ̂ floor prevents division-by-zero)")
}
