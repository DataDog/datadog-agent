// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// LORD-FDR hyperparameter defaults. Held as package-level constants so the
// catalog factory stays parameterless; tunable copies are exported fields on
// LORDFDRCorrelator and can be overwritten by tests (same pattern as
// DempsterShaferCorrelator).
const (
	// lordDefaultAlpha is the global FDR target (Javanmard & Montanari 2018
	// §2.1 — LORD-1 Theorem 1.2 bounds the long-run FDR at α). 0.10 matches
	// the brief's "moderate FP discipline" stance: too tight (0.01) starves
	// recall on low-score true positives; too loose (0.20) doesn't suppress
	// enough marginal anomalies to move the FP ceiling.
	lordDefaultAlpha float64 = 0.10

	// lordDefaultEta is the initial wealth fraction for LORD-1. The starting
	// wealth is w_0 = α·η. J&M's recommended default for LORD-1 is η=0.5,
	// giving w_0 = 0.05 when α=0.10.
	lordDefaultEta float64 = 0.5

	// lordDefaultGamma0 is the γ₀ constant for the log²-decaying spending
	// sequence γ_t = γ₀ / (t · ln²(max(t, 2))). Derived from J&M 2018
	// Lemma 4 for the log²-decaying default. Chosen so the partial sum
	// Σ_{t=1}^{T} γ_t stays well-bounded for large T.
	lordDefaultGamma0 float64 = 0.0722

	// lordDefaultScoreScale controls the exponential p-value mapping:
	//   p(s) = exp(-max(s, 0) / scoreScale)
	// 2.5 is calibrated to the scanmw/scanwelch/mannkendall detectors which
	// emit z-score-like values (mannkendall caps at 50, scanmw passes
	// Mahalanobis-like magnitudes). A score=8 (typical strong scanmw fire)
	// gives p≈0.04; a marginal score=2 gives p≈0.45. The exponential
	// mapping is monotone-decreasing in s so LORD's rejection rule "p_t ≤
	// α_t" preserves the score ordering.
	lordDefaultScoreScale float64 = 2.5
)

// lordKept records a single retained (FDR-passing) anomaly together with the
// LORD-1 state at the time of retention, for debugging and future online
// calibration.
type lordKept struct {
	anomaly observer.Anomaly
	t       int     // hypothesis index at time of retention
	pValue  float64 // synthetic p-value
	alphaT  float64 // LORD-1 spending level at time of retention
}

// LORDFDRCorrelator applies the LORD-1 online false-discovery-rate procedure
// (Javanmard & Montanari 2018, "Online rules for control of false discovery
// rate") to the anomaly stream emitted by upstream detectors. Each anomaly is
// converted to a synthetic p-value via an exponential mapping, then tested
// against the LORD-1 spending level α_t = γ_t · W_t. Only anomalies that pass
// the FDR test (p ≤ α_t) are retained and exposed via ActiveCorrelations.
//
// # LORD-1 wealth dynamics
//
// Starting wealth W_0 = α·η. On each hypothesis (anomaly):
//   - α_t = γ_t · W_t          (current spending level)
//   - if p_t ≤ α_t: REJECT — retain anomaly, replenish wealth:
//       W_{t+1} = W_t − α_t + α·η·(1+|R_t|)·γ(t − τ_last)
//     where |R_t| is the count of rejections up to t (inclusive) and τ_last
//     is the index of the previous rejection. Wealth is capped at α.
//   - else: SUPPRESS — W_{t+1} = W_t − α_t (wealth decays; stays ≥ 0 because
//     α_t = γ_t·W_t and γ_t < 1 for all t ≥ 1).
//
// # Per-detector score scaling
//
// bocpd emits change-point probabilities in [0, 1], not z-score-like values.
// Without separate calibration, bocpd scores would all map to p≈0.7–1.0 and
// be catastrophically suppressed (all bocpd-dominant scenarios would lose
// recall). ScoreScales provides a per-detector override for the exponential
// scale parameter; see NewLORDFDRCorrelator for the calibration table.
//
// # Structural note
//
// This is a SUPPRESSIVE correlator: it emits fewer ActiveCorrelations than
// its input stream, never more. The catalog entry for this correlator sets
// defaultEnabled=true and flips dempster_shafer to defaultEnabled=false, so
// the count of default-enabled correlators remains at 2 (time_cluster +
// lord_fdr). No new parallel emission path is introduced.
type LORDFDRCorrelator struct {
	// Alpha is the global FDR target. See lordDefaultAlpha.
	Alpha float64
	// Eta is the initial wealth fraction. See lordDefaultEta.
	Eta float64
	// Gamma0 is the γ₀ constant for the spending sequence. See lordDefaultGamma0.
	Gamma0 float64
	// ScoreScale is the default exponential scale for the p-value mapping.
	// See lordDefaultScoreScale.
	ScoreScale float64
	// ScoreScales overrides ScoreScale on a per-detector basis. Keys are
	// DetectorName values as emitted by the upstream detectors.
	ScoreScales map[string]float64

	// private LORD-1 state — guarded by mu.
	wealth     float64          // current wealth W_t
	tIndex     int              // 1-based hypothesis index; 0 = no hypotheses yet
	lastReject int              // tIndex at last rejection; 0 = no rejections yet
	numRejects int              // total rejections so far (|R_t|)
	kept       map[string][]lordKept // retained anomalies, keyed by DetectorName
	currentDT  int64
	mu         sync.RWMutex
}

// NewLORDFDRCorrelator returns a LORDFDRCorrelator with production
// hyperparameters and the per-detector score-scale calibration table.
func NewLORDFDRCorrelator() *LORDFDRCorrelator {
	c := &LORDFDRCorrelator{
		Alpha:      lordDefaultAlpha,
		Eta:        lordDefaultEta,
		Gamma0:     lordDefaultGamma0,
		ScoreScale: lordDefaultScoreScale,
		// Per-detector score-scale calibration. This table MUST be present
		// from the first iteration — without it bocpd anomalies all get
		// p≈0.7–1.0 and are catastrophically suppressed, regressing every
		// bocpd-dominant scenario.
		//
		// bocpd emits change-point probability ∈ [0, 1]: use scale=0.25 so
		// cp_prob=0.9 → p=exp(-3.6)≈0.027 (below typical α_t) while
		// cp_prob=0.1 → p=exp(-0.4)≈0.67 (appropriately suppressed).
		//
		// mannkendall/scanmw/scanwelch emit z-score-like values: scale=2.5
		// (the global default; listed explicitly for documentation).
		ScoreScales: map[string]float64{
			"bocpd":       0.25,
			"mannkendall": 2.5,
			"scanmw":      2.5,
			"scanwelch":   2.5,
		},
		kept: make(map[string][]lordKept),
	}
	c.wealth = c.Alpha * c.Eta
	return c
}

// Name returns the correlator name. Must match the catalog entry exactly so
// q.eval-scenarios --only and the engine's per-component telemetry resolve.
func (c *LORDFDRCorrelator) Name() string {
	return "lord_fdr_correlator"
}

// gammaAt computes γ_t = Gamma0 / (t · ln²(max(t, 2))) for the LORD-1
// spending sequence. t is clamped to [1, ∞) to avoid division-by-zero; in
// practice callers always pass t ≥ 1 (tIndex is 1-based and gap = t −
// lastReject ≥ 1 at any rejection).
func (c *LORDFDRCorrelator) gammaAt(t int) float64 {
	if t < 1 {
		t = 1
	}
	ft := float64(t)
	logT := math.Log(math.Max(ft, 2))
	return c.Gamma0 / (ft * logT * logT)
}

// scoreScaleFor returns the exponential scale for a detector name, falling
// back to c.ScoreScale for names not in the calibration table.
func (c *LORDFDRCorrelator) scoreScaleFor(detectorName string) float64 {
	if s, ok := c.ScoreScales[detectorName]; ok && s > 0 {
		return s
	}
	return c.ScoreScale
}

// pValueFor maps an anomaly's Score to a synthetic one-sided p-value via
//
//	p(s) = exp(−max(s, 0) / scoreScale)
//
// where scoreScale is detector-specific (see ScoreScales). The mapping is
// monotone-decreasing in s, so LORD's rejection rule "p_t ≤ α_t" preserves
// the score ordering: higher scores → smaller p-values → more likely to be
// retained. A nil Score is treated as s=0, giving p=1.0 (suppressed under
// any finite threshold).
func (c *LORDFDRCorrelator) pValueFor(a observer.Anomaly) float64 {
	score := 0.0
	if a.Score != nil {
		score = *a.Score
	}
	if score < 0 {
		score = 0
	}
	scale := c.scoreScaleFor(a.DetectorName)
	return math.Exp(-score / scale)
}

// ProcessAnomaly runs one step of the LORD-1 procedure on the incoming anomaly.
//
// Complexity: O(1) — three float ops plus one map append on rejection.
// The only allocation on the happy (reject) path is the append to the per-
// detector kept slice; on the suppress path there are zero allocations.
func (c *LORDFDRCorrelator) ProcessAnomaly(a observer.Anomaly) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Step 1: advance the 1-based hypothesis counter.
	c.tIndex++
	t := c.tIndex

	// Step 2: compute the LORD-1 spending level α_t = γ_t · W_t.
	gammaT := c.gammaAt(t)
	alphaT := gammaT * c.wealth

	// Step 3: compute the synthetic p-value for this anomaly.
	p := c.pValueFor(a)

	if p <= alphaT {
		// REJECT: anomaly clears the FDR threshold — retain it.
		c.numRejects++

		// LORD-1 wealth recurrence (J&M 2018 eq. 2.4):
		//   W_{t+1} = W_t − α_t + α·η·(1+|R_t|)·γ(t − τ_last)
		// τ_last is lastReject (index of previous rejection, or 0). The gap
		// t − τ_last is always ≥ 1 since tIndex is 1-based and lastReject < t.
		gap := t - c.lastReject
		bonus := c.Alpha * c.Eta * float64(1+c.numRejects) * c.gammaAt(gap)
		c.wealth = c.wealth - alphaT + bonus
		// Cap wealth at Alpha to prevent runaway accumulation from dense
		// high-score bursts.
		if c.wealth > c.Alpha {
			c.wealth = c.Alpha
		}
		c.lastReject = t

		c.kept[a.DetectorName] = append(c.kept[a.DetectorName], lordKept{
			anomaly: a,
			t:       t,
			pValue:  p,
			alphaT:  alphaT,
		})
	} else {
		// SUPPRESS: anomaly does not clear the FDR threshold. Wealth decreases
		// by the current spending. It stays non-negative because α_t = γ_t·W_t
		// and γ_t < 1 for all t ≥ 1, so W_{t+1} = W_t·(1−γ_t) ≥ 0. The
		// guard below protects against floating-point underflow only.
		c.wealth -= alphaT
		if c.wealth < 0 {
			c.wealth = 0
		}
	}
}

// Advance updates the current data time. LORD-1 does not use time-based
// eviction: the wealth mechanism naturally starves marginal anomalies without
// an explicit window, so accumulated kept anomalies are bounded by the FDR
// budget (typically < 100 for a full eval at α=0.10). CurrentDT is maintained
// for potential downstream use.
func (c *LORDFDRCorrelator) Advance(dataTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dataTime > c.currentDT {
		c.currentDT = dataTime
	}
}

// Reset clears all LORD-1 state and re-initializes the wealth to α·η, making
// the correlator ready for a fresh replay (testbench reanalysis path).
func (c *LORDFDRCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wealth = c.Alpha * c.Eta
	c.tIndex = 0
	c.lastReject = 0
	c.numRejects = 0
	c.kept = make(map[string][]lordKept)
	c.currentDT = 0
}

// ActiveCorrelations returns one ActiveCorrelation per retained anomaly,
// structured to match the DetectorPassthroughCorrelator shape:
//
//	Pattern:          "lord_{detectorName}_{index}"
//	Title:            "LORD-FDR[{detectorName}]: {source}"
//	Members:          single-element slice of the source descriptor
//	Anomalies:        single-element slice of the retained anomaly
//	FirstSeen/LastUpdated: the anomaly's timestamp
//
// Complexity: O(K) where K = number of retained anomalies. K is bounded by
// the FDR budget: at α=0.10 with the baseline of ~53 system FPs, expect
// K ≈ 30–45 retained correlations — strictly fewer than the dempster_shafer
// correlator it replaces. Output is sorted by detector name for deterministic
// ordering.
func (c *LORDFDRCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Sort detector names for deterministic output.
	detNames := make([]string, 0, len(c.kept))
	for name := range c.kept {
		detNames = append(detNames, name)
	}
	sort.Strings(detNames)

	var result []observer.ActiveCorrelation
	for _, detName := range detNames {
		for i, k := range c.kept[detName] {
			result = append(result, observer.ActiveCorrelation{
				Pattern:     fmt.Sprintf("lord_%s_%d", detName, i),
				Title:       fmt.Sprintf("LORD-FDR[%s]: %s", detName, k.anomaly.Source),
				Members:     []observer.SeriesDescriptor{k.anomaly.Source},
				Anomalies:   []observer.Anomaly{k.anomaly},
				FirstSeen:   k.anomaly.Timestamp,
				LastUpdated: k.anomaly.Timestamp,
			})
		}
	}

	return result
}
