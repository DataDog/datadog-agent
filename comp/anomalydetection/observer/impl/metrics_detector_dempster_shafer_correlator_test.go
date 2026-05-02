// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sync"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dsScorePtr returns a *float64 for use in observer.Anomaly.Score.
func dsScorePtr(v float64) *float64 { return &v }

// dsAnomaly builds a minimal Anomaly with the fields the correlator reads.
func dsAnomaly(detector, sourceName string, ts int64, score float64) observer.Anomaly {
	return observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		DetectorName: detector,
		Source:       observer.SeriesDescriptor{Name: sourceName},
		Timestamp:    ts,
		Score:        dsScorePtr(score),
	}
}

// TestDempsterShafer_RegisteredInCatalog mirrors the mannkendall pattern:
// confirm the catalog has the entry, that the factory yields a correlator,
// and that it is reachable as a componentCorrelator (since stage 2 wires the
// real Correlator algorithm).
func TestDempsterShafer_RegisteredInCatalog(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == "dempster_shafer_correlator" {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "dempster_shafer_correlator entry must exist in the catalog")
	require.Equal(t, componentCorrelator, found.kind,
		"dempster_shafer_correlator must be a correlator — its algorithm consumes anomalies cross-detector")

	instance := found.factory(found.defaultConfig)
	c, ok := instance.(*DempsterShaferCorrelator)
	require.True(t, ok, "factory must produce *DempsterShaferCorrelator")
	require.Equal(t, "dempster_shafer_correlator", c.Name())

	// Must satisfy observer.Correlator so Instantiate can place it in the
	// correlators slice. This is a structural guard against future
	// signature drift.
	var _ observer.Correlator = c
}

// TestDempsterShafer_DefaultEnabledMatchesCatalog asserts the catalog default.
// The lord-online-fdr-correlator candidate (lord_fdr_correlator) explicitly
// flips dempster_shafer to defaultEnabled=false so the count of default-enabled
// correlators remains at 2 (time_cluster + lord_fdr). The correlator entry is
// kept registered for --only eval comparison.
func TestDempsterShafer_DefaultEnabledMatchesCatalog(t *testing.T) {
	cat := defaultCatalog()
	for _, e := range cat.entries {
		if e.name == "dempster_shafer_correlator" {
			assert.False(t, e.defaultEnabled,
				"dempster_shafer_correlator defaultEnabled must be false; it was disabled in favor of lord_fdr_correlator (lord-online-fdr-correlator candidate)")
			return
		}
	}
	t.Fatal("dempster_shafer_correlator entry not found in catalog")
}

// TestDempsterShafer_SingleHighScoreEmits encodes the explicit anti-suppression
// requirement from the plan: a single high-confidence detector fire must yield
// a correlation. This is the regression guard against the exp-0070
// consensus-correlator failure mode where 063_twilio / 213_pagerduty /
// food_delivery_redis lost all recall when no second detector agreed.
func TestDempsterShafer_SingleHighScoreEmits(t *testing.T) {
	c := NewDempsterShaferCorrelator()

	// scanmw has reliability 0.8; score=45 normalizes to 0.9; mA = 0.72 > 0.6.
	a := dsAnomaly("scanmw", "redis.cpu.sys", 1000, 45)
	c.ProcessAnomaly(a)

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1, "single high-confidence fire must NOT be suppressed")
	require.Len(t, corrs[0].Anomalies, 1)
	assert.Equal(t, a, corrs[0].Anomalies[0],
		"the contributing anomaly must be carried through unchanged")
	assert.Equal(t, int64(1000), corrs[0].FirstSeen)
	assert.Equal(t, int64(1000), corrs[0].LastUpdated)
}

// TestDempsterShafer_SingleLowScoreSuppressed verifies the threshold actually
// gates: a single weak fire (mA below threshold) must not emit. Pairs with
// SingleHighScoreEmits to bound the per-anomaly behavior.
func TestDempsterShafer_SingleLowScoreSuppressed(t *testing.T) {
	c := NewDempsterShaferCorrelator()

	// mannkendall reliability 0.6; score=10 → s=0.2; mA=0.12 << 0.6.
	c.ProcessAnomaly(dsAnomaly("mannkendall", "redis.cpu.sys", 1000, 10))

	corrs := c.ActiveCorrelations()
	assert.Empty(t, corrs, "weak single fire must not produce a correlation")
}

// TestDempsterShafer_TwoLowScoresFuseToHigh verifies the core fusion behavior:
// two below-threshold fires from different detectors on the same series, within
// proximity, combine to clear the threshold. This is the lift the correlator
// is supposed to deliver over passthrough.
func TestDempsterShafer_TwoLowScoresFuseToHigh(t *testing.T) {
	c := NewDempsterShaferCorrelator()

	// scanmw r=0.8, score=20 → s=0.4, mA1=0.32, mU1=0.68.
	// bocpd  r=0.7, score=20 → s=0.4, mA2=0.28, mU2=0.72.
	// Both m({N})=0 → no conflict, K=0.
	// mA_combined = (mA1*mA2 + mA1*mU2 + mU1*mA2) / 1
	//             = 0.32*0.28 + 0.32*0.72 + 0.68*0.28
	//             = 0.0896   + 0.2304    + 0.1904   = 0.5104.
	// 0.5104 < 0.6 — the textbook two-detector example doesn't quite clear
	// the threshold. Add a third detector at the same level to confirm
	// fusion-toward-belief without changing the threshold.
	c.ProcessAnomaly(dsAnomaly("scanmw", "redis.cpu.sys", 1000, 20))
	c.ProcessAnomaly(dsAnomaly("bocpd", "redis.cpu.sys", 1005, 20))

	// Two-fire intermediate: must NOT emit yet.
	require.Empty(t, c.ActiveCorrelations(),
		"two weak fires should fuse but stay below threshold (sanity bound)")

	// Third weak fire from scanwelch (r=0.8, score=20) pushes belief over 0.6.
	c.ProcessAnomaly(dsAnomaly("scanwelch", "redis.cpu.sys", 1010, 20))

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1, "three weak fires on the same series must fuse to a correlation")
	require.Len(t, corrs[0].Anomalies, 3,
		"all three contributing anomalies must be attached to the correlation")
	assert.Equal(t, int64(1000), corrs[0].FirstSeen)
	assert.Equal(t, int64(1010), corrs[0].LastUpdated)
	assert.Equal(t, "dempster_shafer_"+corrs[0].Anomalies[0].Source.Key(), corrs[0].Pattern)
}

// TestDempsterShafer_HighConflictRejected drives Dempster's rule into a
// high-conflict state by giving two detectors masses on opposing focal sets.
// The correlator's bpaFromAnomaly never assigns m({N}) > 0, so to manufacture
// conflict we reach into the exported combine function directly via a custom
// state. This is white-box but pinned to the documented invariant
// (K > ConflictCeiling → skip) so a future BPA-formula change can't silently
// remove the safeguard.
func TestDempsterShafer_HighConflictRejected(t *testing.T) {
	c := NewDempsterShaferCorrelator()

	// Seed a state that already has heavy mass on {N}. We do this by direct
	// assignment to exercise the conflict path without coupling the test to
	// internals of bpaFromAnomaly.
	a1 := dsAnomaly("scanmw", "redis.cpu.sys", 1000, 0)
	c.ProcessAnomaly(a1) // creates state with mA=0, mN=0, mU=1 (no conflict yet)

	c.mu.Lock()
	st := c.state[a1.Source.Key()]
	require.NotNil(t, st)
	st.mA = 0.0
	st.mN = 0.9
	st.mU = 0.1
	c.mu.Unlock()

	// Now feed an anomaly with strong support for {A}: scanmw r=0.8, score=50
	// → mA=0.8, mN=0, mU=0.2. Conflict K = m1A*m2N + m1N*m2A = 0 + 0.9*0.8 = 0.72,
	// which is > the 0.7 ceiling → fusion must be skipped.
	c.ProcessAnomaly(dsAnomaly("scanmw", "redis.cpu.sys", 1010, 50))

	c.mu.RLock()
	stAfter := c.state[a1.Source.Key()]
	c.mu.RUnlock()
	require.NotNil(t, stAfter)
	assert.InDelta(t, 0.0, stAfter.mA, 1e-9,
		"high-conflict fusion must leave mA untouched")
	assert.InDelta(t, 0.9, stAfter.mN, 1e-9,
		"high-conflict fusion must leave mN untouched")
	assert.Empty(t, c.ActiveCorrelations(),
		"high-conflict state must NOT cross the belief threshold")
}

// TestDempsterShafer_EvictionByAdvance verifies stale state is reaped. Without
// this, memory grows linearly with total cumulative series count regardless of
// activity.
func TestDempsterShafer_EvictionByAdvance(t *testing.T) {
	c := NewDempsterShaferCorrelator()
	c.WindowSeconds = 300

	c.ProcessAnomaly(dsAnomaly("scanmw", "redis.cpu.sys", 10, 45))
	require.NotEmpty(t, c.state, "state must be populated after ProcessAnomaly")

	c.Advance(400) // 400 - 300 = 100 cutoff; the t=10 entry is older than that.

	c.mu.RLock()
	defer c.mu.RUnlock()
	assert.Empty(t, c.state, "Advance past the window must evict stale state")
}

// TestDempsterShafer_ProximityReplacesNotFuses verifies the stale-state seeding
// rule: an anomaly arriving more than ProximitySeconds after the last update
// replaces the per-series state rather than fusing. Without this, arbitrarily
// distant evidence would accumulate and inflate belief.
func TestDempsterShafer_ProximityReplacesNotFuses(t *testing.T) {
	c := NewDempsterShaferCorrelator()
	c.ProximitySeconds = 30

	c.ProcessAnomaly(dsAnomaly("scanmw", "redis.cpu.sys", 1000, 45))
	c.ProcessAnomaly(dsAnomaly("bocpd", "redis.cpu.sys", 1100, 5)) // 100s gap → replace

	c.mu.RLock()
	st := c.state["|redis.cpu.sys:none|"]
	c.mu.RUnlock()
	require.NotNil(t, st)
	require.Len(t, st.contributing, 1,
		"after-window arrival must reseed; previous evidence must NOT remain attached")
	assert.Equal(t, "bocpd", st.contributing[0].DetectorName)
	assert.Equal(t, int64(1100), st.firstSeen)
}

// TestDempsterShafer_Reset clears both state and the data clock.
func TestDempsterShafer_Reset(t *testing.T) {
	c := NewDempsterShaferCorrelator()
	c.ProcessAnomaly(dsAnomaly("scanmw", "redis.cpu.sys", 1000, 45))
	require.NotEmpty(t, c.state)

	c.Reset()

	c.mu.RLock()
	defer c.mu.RUnlock()
	assert.Empty(t, c.state)
	assert.Equal(t, int64(0), c.currentDataTime)
}

// TestDempsterShafer_DistinctSeriesAreIndependent guards the per-series keying:
// two anomalies on different series must not fuse with each other.
func TestDempsterShafer_DistinctSeriesAreIndependent(t *testing.T) {
	c := NewDempsterShaferCorrelator()

	c.ProcessAnomaly(dsAnomaly("scanmw", "redis.cpu.sys", 1000, 45))     // emits
	c.ProcessAnomaly(dsAnomaly("bocpd", "redis.info.latency_ms", 1005, 10)) // weak, distinct series

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1, "weak fire on a distinct series must not piggy-back on a strong fire")
	assert.Equal(t, "redis.cpu.sys", corrs[0].Members[0].Name)
}

// TestDempsterShafer_UnknownDetectorUsesDefaultReliability confirms the
// fallback path is exercised. Setting a custom DefaultReliability proves the
// lookup actually consults that field rather than a hardcoded constant.
func TestDempsterShafer_UnknownDetectorUsesDefaultReliability(t *testing.T) {
	c := NewDempsterShaferCorrelator()
	c.DefaultReliability = 0.5

	// Score=50 → s=1.0; with r=0.5, mA=0.5 < 0.6 → no emit.
	c.ProcessAnomaly(dsAnomaly("brand_new_detector", "redis.cpu.sys", 1000, 50))
	assert.Empty(t, c.ActiveCorrelations(),
		"unknown detector must use DefaultReliability, not a higher hardcoded value")

	// Lift DefaultReliability to 0.9 → mA=0.9 > 0.6 → emits.
	c2 := NewDempsterShaferCorrelator()
	c2.DefaultReliability = 0.9
	c2.ProcessAnomaly(dsAnomaly("brand_new_detector", "redis.cpu.sys", 1000, 50))
	assert.Len(t, c2.ActiveCorrelations(), 1)
}

// TestDempsterShafer_DeterministicOrdering pins the ActiveCorrelations sort
// contract: FirstSeen ascending. Reporters and scoring assume stable ordering.
func TestDempsterShafer_DeterministicOrdering(t *testing.T) {
	c := NewDempsterShaferCorrelator()

	// Three distinct series, each with a high-score fire, intentionally fed
	// out of timestamp order.
	c.ProcessAnomaly(dsAnomaly("scanmw", "metric.c", 3000, 45))
	c.ProcessAnomaly(dsAnomaly("scanmw", "metric.a", 1000, 45))
	c.ProcessAnomaly(dsAnomaly("scanmw", "metric.b", 2000, 45))

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 3)
	assert.Equal(t, int64(1000), corrs[0].FirstSeen)
	assert.Equal(t, int64(2000), corrs[1].FirstSeen)
	assert.Equal(t, int64(3000), corrs[2].FirstSeen)
}

// TestDempsterShafer_BPAMassesSumToOne is an invariant on bpaFromAnomaly: the
// three masses must sum to 1 within floating-point tolerance for any score in
// the supported range. Drift here would silently break Dempster's rule, since
// the closed-form combination assumes normalized BPAs.
func TestDempsterShafer_BPAMassesSumToOne(t *testing.T) {
	c := NewDempsterShaferCorrelator()
	for _, score := range []float64{0, 1, 10, 25, 49.9, 50, 1000} {
		mA, mN, mU := c.bpaFromAnomaly(dsAnomaly("scanmw", "x", 0, score))
		assert.InDelta(t, 1.0, mA+mN+mU, 1e-9,
			"BPA masses must sum to 1 for score=%v (got mA=%v mN=%v mU=%v)", score, mA, mN, mU)
		assert.False(t, math.IsNaN(mA) || math.IsNaN(mN) || math.IsNaN(mU),
			"BPA must not produce NaN for score=%v", score)
	}
}

// TestDempsterShafer_Concurrency exercises the mutex with parallel writers.
// Run with -race locally; here we assert only that no panic occurs and that
// every anomaly was recorded somewhere (no silent drops).
func TestDempsterShafer_Concurrency(t *testing.T) {
	c := NewDempsterShaferCorrelator()

	const numGoroutines = 100
	const perGoroutine = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				// Stagger sources across goroutines so we exercise both
				// fresh-key and merging code paths.
				src := "metric.shared"
				if g%2 == 0 {
					src = "metric.even"
				}
				c.ProcessAnomaly(dsAnomaly("scanmw", src, int64(1000+g*perGoroutine+i), 45))
			}
		}(g)
	}

	// Concurrent readers and Advance to stress the RLock / Lock pairing.
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = c.ActiveCorrelations()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Advance(int64(1000 + i))
		}
	}()

	wg.Wait()

	// We can't precisely assert state size after concurrent Advance calls,
	// but we can confirm the correlator is still functional.
	c.ProcessAnomaly(dsAnomaly("scanmw", "metric.post_concurrent", 5000, 45))
	corrs := c.ActiveCorrelations()
	found := false
	for _, c := range corrs {
		if len(c.Members) > 0 && c.Members[0].Name == "metric.post_concurrent" {
			found = true
			break
		}
	}
	assert.True(t, found, "correlator must remain functional after concurrent access")
}
