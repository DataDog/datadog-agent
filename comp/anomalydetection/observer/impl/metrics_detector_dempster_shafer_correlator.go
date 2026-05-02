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

// Dempster-Shafer hyperparameters. Held as constants so the catalog factory
// stays parameterless; tunable copies live on DempsterShaferCorrelator and
// can be overwritten by tests.
const (
	dsDefaultProximitySeconds   int64   = 30
	dsDefaultWindowSeconds      int64   = 300
	dsDefaultConflictCeiling    float64 = 0.7
	dsDefaultBeliefThreshold    float64 = 0.6
	dsDefaultDetectorReliability float64 = 0.7
	dsScoreNormalizationCap     float64 = 50.0
)

// dsState is the per-series fused belief state in the proximity window.
//
// mA, mN, mU are the masses Dempster's rule assigns to the focal sets {A},
// {N}, and {A,N} (ignorance) respectively, and always sum to 1 within
// floating-point tolerance. contributing accumulates the anomalies that
// produced this fused belief so the emitted ActiveCorrelation carries the
// underlying evidence.
type dsState struct {
	mA, mN, mU   float64
	contributing []observer.Anomaly
	firstSeen    int64
	lastUpdated  int64
}

// DempsterShaferCorrelator fuses per-detector anomaly evidence on the same
// series via Dempster's rule of combination. Each anomaly becomes a Basic
// Probability Assignment (BPA) over the frame {Anomalous, Normal}; BPAs that
// arrive on the same series within ProximitySeconds are combined; the
// correlator emits an ActiveCorrelation when the fused belief in {Anomalous}
// clears BeliefThreshold AND Dempster's conflict K stays below
// ConflictCeiling.
//
// Design choices worth flagging:
//
//   - m({N}) is set to zero on every BPA (Yager / Smets discounting). A
//     detector firing is by construction evidence FOR {A}; the residual mass
//     is uncertainty m({A,N}), not affirmative normality. This avoids
//     gratuitous conflict between two firing detectors of different
//     reliabilities.
//
//   - Single high-confidence fires are NOT suppressed. A reliability=0.8
//     detector with a normalized score=0.9 produces m({A})=0.72 which
//     exceeds the 0.6 threshold on its own. This is intentional and is the
//     fix for the consensus-correlator failure mode in exp-0070, where
//     scenarios with a single strong detector lost all recall.
//
//   - State is keyed by SeriesDescriptor.Key(); cross-series fusion is out
//     of scope for this correlator (the time_cluster correlator handles
//     that). The bound is num_active_series_in_window * sizeof(dsState).
type DempsterShaferCorrelator struct {
	ProximitySeconds   int64
	WindowSeconds      int64
	ConflictCeiling    float64
	BeliefThreshold    float64
	DefaultReliability float64
	// reliability is a per-detector prior weight. Unknown names fall back
	// to DefaultReliability. Values were seeded from exp-0070 detector FP
	// rates and can be tuned (or learned online) in a follow-up.
	reliability     map[string]float64
	state           map[string]*dsState
	currentDataTime int64
	mu              sync.RWMutex
}

// NewDempsterShaferCorrelator returns a correlator with the production
// hyperparameters and a hardcoded reliability map covering the catalog
// detectors.
func NewDempsterShaferCorrelator() *DempsterShaferCorrelator {
	return &DempsterShaferCorrelator{
		ProximitySeconds:   dsDefaultProximitySeconds,
		WindowSeconds:      dsDefaultWindowSeconds,
		ConflictCeiling:    dsDefaultConflictCeiling,
		BeliefThreshold:    dsDefaultBeliefThreshold,
		DefaultReliability: dsDefaultDetectorReliability,
		reliability: map[string]float64{
			"bocpd":       0.7,
			"scanmw":      0.8,
			"scanwelch":   0.8,
			"mannkendall": 0.6,
		},
		state: make(map[string]*dsState),
	}
}

// Name returns the correlator name. It must match the catalog entry exactly so
// q.eval-scenarios --only and the engine's per-component telemetry resolve.
func (c *DempsterShaferCorrelator) Name() string {
	return "dempster_shafer_correlator"
}

// reliabilityFor returns the prior reliability for a detector name, falling
// back to DefaultReliability for names not in the map. Read-only on a
// pre-populated map: safe under c.mu read-lock.
func (c *DempsterShaferCorrelator) reliabilityFor(name string) float64 {
	if r, ok := c.reliability[name]; ok {
		return r
	}
	return c.DefaultReliability
}

// bpaFromAnomaly turns an Anomaly into a BPA (mA, mN, mU) on the frame
// {A, N}. m({N}) is fixed at 0 — a firing detector provides no positive
// evidence for "Normal"; whatever isn't on {A} is ignorance. See the
// type-level comment on the Yager/Smets discounting rationale.
func (c *DempsterShaferCorrelator) bpaFromAnomaly(a observer.Anomaly) (mA, mN, mU float64) {
	score := 0.0
	if a.Score != nil {
		score = *a.Score
	}
	// Clamp to [0, dsScoreNormalizationCap], normalize to [0, 1].
	if score < 0 {
		score = 0
	}
	if score > dsScoreNormalizationCap {
		score = dsScoreNormalizationCap
	}
	s := score / dsScoreNormalizationCap

	r := c.reliabilityFor(a.DetectorName)
	mA = r * s
	mN = 0.0
	mU = 1.0 - mA - mN
	if mU < 0 {
		mU = 0
	}
	return mA, mN, mU
}

// combineBPA applies Dempster's rule of combination to two BPAs over the
// 2-element frame {A, N} with focal sets {A}, {N}, {A,N}. Returns the fused
// (mA, mN, mU) and the conflict K. When K exceeds the ConflictCeiling, the
// caller treats the evidence as incoherent and skips the update.
//
// Closed form for {A}, {N}, {A,N}:
//
//	K       = m1A*m2N + m1N*m2A
//	mA_new  = (m1A*m2A + m1A*m2U + m1U*m2A) / (1 - K)
//	mN_new  = (m1N*m2N + m1N*m2U + m1U*m2N) / (1 - K)
//	mU_new  = (m1U*m2U)                     / (1 - K)
func (c *DempsterShaferCorrelator) combineBPA(m1A, m1N, m1U, m2A, m2N, m2U float64) (mA, mN, mU, K float64) {
	K = m1A*m2N + m1N*m2A
	denom := 1.0 - K
	if denom <= 0 {
		// Total conflict; caller will detect K > ceiling and skip.
		return m1A, m1N, m1U, K
	}
	mA = (m1A*m2A + m1A*m2U + m1U*m2A) / denom
	mN = (m1N*m2N + m1N*m2U + m1U*m2N) / denom
	mU = (m1U * m2U) / denom
	return mA, mN, mU, K
}

// ProcessAnomaly receives a per-detector anomaly, computes its BPA, and either
// seeds a fresh per-series state or fuses the BPA into the existing state via
// Dempster's rule. Stale state (last update older than ProximitySeconds) is
// replaced rather than fused — fusing across long gaps would let arbitrarily
// distant evidence accumulate.
func (c *DempsterShaferCorrelator) ProcessAnomaly(a observer.Anomaly) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if a.Timestamp > c.currentDataTime {
		c.currentDataTime = a.Timestamp
	}

	mA, mN, mU := c.bpaFromAnomaly(a)
	key := a.Source.Key()

	st, ok := c.state[key]
	if !ok || a.Timestamp-st.lastUpdated > c.ProximitySeconds {
		// Seed a fresh state — either no prior evidence on this series or
		// the previous evidence is outside the proximity window.
		c.state[key] = &dsState{
			mA:           mA,
			mN:           mN,
			mU:           mU,
			contributing: []observer.Anomaly{a},
			firstSeen:    a.Timestamp,
			lastUpdated:  a.Timestamp,
		}
		return
	}

	newA, newN, newU, K := c.combineBPA(st.mA, st.mN, st.mU, mA, mN, mU)
	if K > c.ConflictCeiling {
		// Detectors disagree too strongly; refuse to fuse this evidence.
		// Keep the existing state untouched but still record the anomaly
		// as contributing context for downstream display, and update
		// lastUpdated so the proximity window slides forward (otherwise
		// repeated high-conflict fires would never refresh and the next
		// arrival would replace state entirely).
		st.contributing = append(st.contributing, a)
		st.lastUpdated = a.Timestamp
		return
	}
	st.mA = newA
	st.mN = newN
	st.mU = newU
	st.contributing = append(st.contributing, a)
	st.lastUpdated = a.Timestamp
}

// Advance updates the data clock and evicts any per-series state whose latest
// contributing anomaly is older than WindowSeconds. Eviction is amortized
// O(active series) per call.
func (c *DempsterShaferCorrelator) Advance(dataTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dataTime > c.currentDataTime {
		c.currentDataTime = dataTime
	}
	cutoff := c.currentDataTime - c.WindowSeconds
	for key, st := range c.state {
		if st.lastUpdated < cutoff {
			delete(c.state, key)
		}
	}
}

// Reset clears all internal state for reanalysis (testbench replay path).
func (c *DempsterShaferCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = make(map[string]*dsState)
	c.currentDataTime = 0
}

// ActiveCorrelations emits one ActiveCorrelation per series whose fused belief
// in {Anomalous} clears BeliefThreshold. Output is sorted by FirstSeen for
// deterministic ordering (testbench scoring requires stable output across
// runs).
func (c *DempsterShaferCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]observer.ActiveCorrelation, 0, len(c.state))
	for key, st := range c.state {
		if st.mA <= c.BeliefThreshold {
			continue
		}
		// Members deduplicated/sorted by SeriesDescriptor.Key() — for a
		// single-series state this collapses to one entry, but we route
		// through the shared helper so behavior matches other correlators
		// and tagged-variant handling stays consistent.
		members := sortedUniqueMembers(st.contributing)
		anomalies := make([]observer.Anomaly, len(st.contributing))
		copy(anomalies, st.contributing)

		result = append(result, observer.ActiveCorrelation{
			Pattern:     "dempster_shafer_" + key,
			Title:       fmt.Sprintf("Fused belief: %s (bel=%.2f)", key, st.mA),
			Members:     members,
			Anomalies:   anomalies,
			FirstSeen:   st.firstSeen,
			LastUpdated: st.lastUpdated,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].FirstSeen != result[j].FirstSeen {
			return result[i].FirstSeen < result[j].FirstSeen
		}
		return result[i].Pattern < result[j].Pattern
	})
	return result
}
