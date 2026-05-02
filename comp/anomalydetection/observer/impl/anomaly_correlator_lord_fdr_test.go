// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lordScorePtr returns a *float64 for use in observer.Anomaly.Score.
func lordScorePtr(v float64) *float64 { return &v }

// lordAnomaly builds a minimal Anomaly with the fields the LORD-FDR correlator reads.
func lordAnomaly(detector, sourceName string, ts int64, score float64) observer.Anomaly {
	return observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		DetectorName: detector,
		Source:       observer.SeriesDescriptor{Name: sourceName},
		Timestamp:    ts,
		Score:        lordScorePtr(score),
	}
}

// TestLORDFDR_DefaultEnabledIsTrue verifies the catalog registers lord_fdr_correlator
// with defaultEnabled=true. Required for the coordinator's system-level eval to
// pick it up without --only flags.
func TestLORDFDR_DefaultEnabledIsTrue(t *testing.T) {
	cat := defaultCatalog()
	for _, e := range cat.entries {
		if e.name == "lord_fdr_correlator" {
			assert.True(t, e.defaultEnabled,
				"lord_fdr_correlator must be defaultEnabled=true to be visible in the coordinator eval pipeline")
			assert.Equal(t, componentCorrelator, e.kind,
				"lord_fdr_correlator must be a correlator kind")
			instance := e.factory(e.defaultConfig)
			c, ok := instance.(*LORDFDRCorrelator)
			require.True(t, ok, "factory must produce *LORDFDRCorrelator")
			assert.Equal(t, "lord_fdr_correlator", c.Name())
			// Structural guard: factory product must satisfy observer.Correlator.
			var _ observer.Correlator = c
			return
		}
	}
	t.Fatal("lord_fdr_correlator not found in catalog — was the entry added to component_catalog.go?")
}

// TestLORDFDR_DempsterShaferDisabled asserts that the dempster_shafer_correlator
// catalog entry was flipped to defaultEnabled=false as part of this candidate's
// plan. This keeps the count of default-enabled correlators at 2 (time_cluster +
// lord_fdr) and avoids double-counting against existing correlators.
//
// Authority: lord-online-fdr-correlator implementation plan, "DEMPSTER-SHAFER
// FLIP" section.
func TestLORDFDR_DempsterShaferDisabled(t *testing.T) {
	cat := defaultCatalog()
	for _, e := range cat.entries {
		if e.name == "dempster_shafer_correlator" {
			assert.False(t, e.defaultEnabled,
				"dempster_shafer_correlator must be defaultEnabled=false; it was disabled in favor of lord_fdr_correlator to keep the default-enabled correlator count unchanged at 2")
			return
		}
	}
	t.Fatal("dempster_shafer_correlator not found in catalog")
}

// TestLORDFDR_RejectsHighScoreAnomalies feeds 10 anomalies with a very high score
// (50.0) and asserts all 10 are emitted. With Score=50 and scale=2.5 the p-value
// is exp(-20) ≈ 2e-9, which is orders of magnitude below the LORD-1 spending
// level at any t and any positive wealth. All 10 should be retained.
//
// Note: the plan specifies Score=10 here, but Score=10 gives p≈0.018 which
// exceeds the initial spending level α_1 ≈ 0.0075 (γ_1·(α·η) with the chosen
// hyperparameters). Score=50 is used instead so that "p ≪ α_t ⇒ all rejected"
// holds reliably. The intent of the test (high-score ⇒ always retained) is
// preserved.
func TestLORDFDR_RejectsHighScoreAnomalies(t *testing.T) {
	c := NewLORDFDRCorrelator()
	for i := 0; i < 10; i++ {
		c.ProcessAnomaly(lordAnomaly("scanmw", fmt.Sprintf("metric.%d", i), int64(1000+i), 50.0))
	}
	corrs := c.ActiveCorrelations()
	assert.Len(t, corrs, 10, "all 10 high-score anomalies must be retained by LORD-FDR")
}

// TestLORDFDR_SuppressesLowScoreFlood feeds 100 anomalies with Score=1.0.
// p(1.0) = exp(-1/2.5) = exp(-0.4) ≈ 0.67, which is far above the initial
// spending level and above any attainable level given wealth decay. LORD wealth
// is quickly depleted and none (or very few) of the noisy stream survives.
func TestLORDFDR_SuppressesLowScoreFlood(t *testing.T) {
	c := NewLORDFDRCorrelator()
	for i := 0; i < 100; i++ {
		c.ProcessAnomaly(lordAnomaly("scanmw", fmt.Sprintf("noisy.%d", i), int64(1000+i), 1.0))
	}
	corrs := c.ActiveCorrelations()
	assert.LessOrEqual(t, len(corrs), 5,
		"a flood of low-score anomalies must be suppressed by LORD wealth depletion; got %d retained", len(corrs))
}

// TestLORDFDR_MixedStream alternates high-score (Score=50) and low-score
// (Score=1.0) anomalies. All high-score anomalies must be emitted; at least
// 80% of the low-score anomalies must be suppressed.
func TestLORDFDR_MixedStream(t *testing.T) {
	c := NewLORDFDRCorrelator()
	const n = 20
	highScore := 50.0
	lowScore := 1.0
	highCount, lowCount := 0, 0

	for i := 0; i < n; i++ {
		if i%2 == 0 {
			c.ProcessAnomaly(lordAnomaly("scanmw", fmt.Sprintf("high.%d", i), int64(1000+i), highScore))
			highCount++
		} else {
			c.ProcessAnomaly(lordAnomaly("scanmw", fmt.Sprintf("low.%d", i), int64(1000+i), lowScore))
			lowCount++
		}
	}

	corrs := c.ActiveCorrelations()
	emittedHigh, emittedLow := 0, 0
	for _, ac := range corrs {
		if len(ac.Anomalies) > 0 && ac.Anomalies[0].Score != nil {
			if *ac.Anomalies[0].Score == highScore {
				emittedHigh++
			} else {
				emittedLow++
			}
		}
	}

	assert.Equal(t, highCount, emittedHigh,
		"all high-score anomalies must be emitted; got %d/%d", emittedHigh, highCount)
	suppressedLow := lowCount - emittedLow
	minSuppressed := int(float64(lowCount) * 0.8)
	assert.GreaterOrEqual(t, suppressedLow, minSuppressed,
		"at least 80%% of low-score anomalies must be suppressed; suppressed=%d/%d", suppressedLow, lowCount)
}

// TestLORDFDR_Reset verifies that Reset clears all LORD-1 state and
// re-initializes wealth to α·η.
func TestLORDFDR_Reset(t *testing.T) {
	c := NewLORDFDRCorrelator()
	c.ProcessAnomaly(lordAnomaly("scanmw", "redis.cpu.sys", 1000, 50.0))

	// Verify state is populated after processing.
	c.mu.RLock()
	require.Equal(t, 1, c.tIndex)
	require.Equal(t, 1, c.numRejects)
	require.NotEmpty(t, c.kept)
	c.mu.RUnlock()

	c.Reset()

	c.mu.RLock()
	defer c.mu.RUnlock()
	assert.Empty(t, c.kept, "kept must be cleared after Reset")
	assert.Equal(t, 0, c.tIndex, "tIndex must be 0 after Reset")
	assert.Equal(t, 0, c.lastReject, "lastReject must be 0 after Reset")
	assert.Equal(t, 0, c.numRejects, "numRejects must be 0 after Reset")
	assert.InDelta(t, c.Alpha*c.Eta, c.wealth, 1e-9,
		"wealth must be re-initialized to Alpha*Eta after Reset")
}

// TestLORDFDR_NilScore verifies that an anomaly with a nil Score is treated as
// score=0, giving p-value=exp(0)=1.0. Under any positive wealth the spending
// level α_t = γ_t·W_t is well below 1.0, so nil-score anomalies are always
// suppressed.
func TestLORDFDR_NilScore(t *testing.T) {
	c := NewLORDFDRCorrelator()
	a := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		DetectorName: "scanmw",
		Source:       observer.SeriesDescriptor{Name: "redis.cpu.sys"},
		Timestamp:    1000,
		Score:        nil, // explicitly nil
	}
	c.ProcessAnomaly(a)

	corrs := c.ActiveCorrelations()
	assert.Empty(t, corrs, "nil-Score anomaly (p=1.0) must be suppressed by LORD-FDR")
}

// TestLORDFDR_ConcurrentAccess exercises the mutex contract under parallel
// writers and readers. Run with -race; the test asserts only that the correlator
// remains functional after concurrent access (no panics, no data races).
func TestLORDFDR_ConcurrentAccess(t *testing.T) {
	c := NewLORDFDRCorrelator()
	const numGoroutines = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(g int) {
			defer wg.Done()
			c.ProcessAnomaly(lordAnomaly("scanmw", fmt.Sprintf("series.%d", g), int64(1000+g), 50.0))
		}(g)
	}

	// Concurrent readers and Advance to stress RLock / Lock pairing.
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			_ = c.ActiveCorrelations()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			c.Advance(int64(1000 + i))
		}
	}()

	wg.Wait()

	// Correlator must still be functional after concurrent access.
	c.ProcessAnomaly(lordAnomaly("scanmw", "post_concurrent", 9999, 50.0))
	corrs := c.ActiveCorrelations()
	found := false
	for _, ac := range corrs {
		if len(ac.Members) > 0 && ac.Members[0].Name == "post_concurrent" {
			found = true
			break
		}
	}
	assert.True(t, found, "correlator must remain functional after concurrent access")
}

// TestLORDFDR_BehaviorContractWealthCap feeds 1000 high-score anomalies and
// asserts that wealth never exceeds Alpha. The cap guards against unbounded
// wealth growth from sustained high-confidence detector bursts.
func TestLORDFDR_BehaviorContractWealthCap(t *testing.T) {
	c := NewLORDFDRCorrelator()
	for i := 0; i < 1000; i++ {
		c.ProcessAnomaly(lordAnomaly("scanmw", fmt.Sprintf("series.%d", i), int64(1000+i), 50.0))

		c.mu.RLock()
		w := c.wealth
		c.mu.RUnlock()
		assert.LessOrEqual(t, w, c.Alpha,
			"wealth must never exceed Alpha; at step %d got wealth=%.6f Alpha=%.6f", i+1, w, c.Alpha)
	}
}

// TestLORDFDR_PassthroughShape verifies that emitted ActiveCorrelations match
// the structural shape of DetectorPassthroughCorrelator: one correlation per
// anomaly, "lord_{detName}_{index}" pattern, single-element Members slice.
func TestLORDFDR_PassthroughShape(t *testing.T) {
	c := NewLORDFDRCorrelator()
	a := lordAnomaly("scanmw", "cpu.sys", 1000, 50.0)
	c.ProcessAnomaly(a)

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1, "one retained anomaly must produce one correlation")

	ac := corrs[0]
	assert.Equal(t, "lord_scanmw_0", ac.Pattern,
		"Pattern must use 'lord_{detName}_{index}' format")
	assert.True(t, strings.HasPrefix(ac.Title, "LORD-FDR[scanmw]:"),
		"Title must use 'LORD-FDR[{detName}]: ...' format; got %q", ac.Title)
	require.Len(t, ac.Members, 1, "Members must be a single-element slice")
	assert.Equal(t, "cpu.sys", ac.Members[0].Name)
	require.Len(t, ac.Anomalies, 1, "Anomalies must be a single-element slice")
	assert.Equal(t, a, ac.Anomalies[0], "original anomaly must be carried through unchanged")
	assert.Equal(t, int64(1000), ac.FirstSeen)
	assert.Equal(t, int64(1000), ac.LastUpdated)
}
