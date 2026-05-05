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

// AnomalyRankConfig configures the AnomalyRankCorrelator.
//
// AnomalyRank is a watchdog-style post-filter on raw anomalies: it sits
// alongside the other correlators, ingests the same per-anomaly stream, and
// re-emits only the highest-ranked anomalies per detector per window. Its
// purpose is to suppress weak / borderline detections rather than create new
// ones — see the SIGMOD 2020 paper "AnomalyRank: A Two-Stage Anomaly Detection
// System" and Datadog's watchdog blog post.
type AnomalyRankConfig struct {
	// WindowSeconds is the rolling window the correlator considers. Anomalies
	// older than (currentDataTs - WindowSeconds) are evicted on Advance.
	// Default: 60.
	WindowSeconds int64 `json:"window_seconds"`
	// TopKPerDetector caps the number of anomalies emitted per detector per
	// ActiveCorrelations call. Default: 8.
	TopKPerDetector int `json:"top_k_per_detector"`
	// MinScore is a hard floor: anomalies with rankScore < MinScore are
	// dropped at ProcessAnomaly time and never tracked. Default: 0 (disabled).
	MinScore float64 `json:"min_score"`
	// QuantileFloor is a dynamic floor: only anomalies in the top
	// QuantileFloor fraction of seen scores in the window are emitted.
	// Default: 0.5 (top half). Set to 1.0 to disable quantile gating.
	QuantileFloor float64 `json:"quantile_floor"`
}

// DefaultAnomalyRankConfig returns the canonical default AnomalyRankConfig.
func DefaultAnomalyRankConfig() AnomalyRankConfig {
	return AnomalyRankConfig{
		WindowSeconds:   60,
		TopKPerDetector: 8,
		MinScore:        0,
		QuantileFloor:   0.5,
	}
}

// rankedAnomaly pairs an anomaly with its precomputed rank score so we don't
// recompute on every ActiveCorrelations sort.
type rankedAnomaly struct {
	anomaly observer.Anomaly
	score   float64
	ts      int64
}

// rankBuffer is a per-detector ring of anomalies in arrival order. It is
// trimmed on Advance based on timestamp; ActiveCorrelations sorts a copy.
type rankBuffer struct {
	items []rankedAnomaly
}

// AnomalyRankCorrelator implements a top-K-per-detector post-filter on the
// raw anomaly stream. It is purely additive: it cannot create anomalies, only
// suppress weak ones. The hot path runs only on detection events
// (ProcessAnomaly), so ticks where no detector fires incur zero overhead.
type AnomalyRankCorrelator struct {
	cfg           AnomalyRankConfig
	perDetector   map[string]*rankBuffer
	currentDataTs int64
	mu            sync.RWMutex
}

// NewAnomalyRankCorrelator returns a configured AnomalyRankCorrelator.
// Zero-valued config fields fall back to the documented defaults.
func NewAnomalyRankCorrelator(cfg AnomalyRankConfig) *AnomalyRankCorrelator {
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = 60
	}
	if cfg.TopKPerDetector <= 0 {
		cfg.TopKPerDetector = 8
	}
	// QuantileFloor is allowed to be 0 (= emit nothing) explicitly via JSON,
	// but a struct-zero default is treated as "use the documented default".
	// Distinguish "set to 0.0 deliberately" from "unset" is awkward without a
	// pointer; we rely on the catalog's DefaultAnomalyRankConfig() seeding 0.5
	// before JSON unmarshal overlays caller values, so this branch only fires
	// for callers who construct the config directly with the zero struct.
	if cfg.QuantileFloor == 0 {
		cfg.QuantileFloor = 0.5
	}
	if cfg.QuantileFloor > 1 {
		cfg.QuantileFloor = 1
	}
	return &AnomalyRankCorrelator{
		cfg:         cfg,
		perDetector: make(map[string]*rankBuffer),
	}
}

// Name returns the correlator name.
func (c *AnomalyRankCorrelator) Name() string {
	return "anomaly_rank_correlator"
}

// rankScore returns a comparable float for an anomaly. Order of preference:
// explicit Score field, |DeviationSigma| from DebugInfo, else 0. This
// single-source helper keeps rank semantics consistent everywhere.
func rankScore(a observer.Anomaly) float64 {
	if a.Score != nil {
		return *a.Score
	}
	if a.DebugInfo != nil {
		return math.Abs(a.DebugInfo.DeviationSigma)
	}
	return 0
}

// ProcessAnomaly accumulates an anomaly into its detector's window buffer.
// Anomalies below MinScore are dropped without being tracked.
func (c *AnomalyRankCorrelator) ProcessAnomaly(a observer.Anomaly) {
	score := rankScore(a)
	if score < c.cfg.MinScore {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if a.Timestamp > c.currentDataTs {
		c.currentDataTs = a.Timestamp
	}

	rb, ok := c.perDetector[a.DetectorName]
	if !ok {
		rb = &rankBuffer{}
		c.perDetector[a.DetectorName] = rb
	}
	rb.items = append(rb.items, rankedAnomaly{anomaly: a, score: score, ts: a.Timestamp})
}

// Advance drops anomalies whose timestamps are older than
// (currentDataTs - WindowSeconds). Same in-place filter pattern as
// TimeClusterCorrelator.evictOldClustersLocked.
func (c *AnomalyRankCorrelator) Advance(dataTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if dataTime > c.currentDataTs {
		c.currentDataTs = dataTime
	}
	cutoff := c.currentDataTs - c.cfg.WindowSeconds

	for det, rb := range c.perDetector {
		kept := rb.items[:0]
		for _, it := range rb.items {
			if it.ts >= cutoff {
				kept = append(kept, it)
			}
		}
		rb.items = kept
		if len(rb.items) == 0 {
			// Free the detector slot to keep iteration tight when a noisy
			// detector goes silent for a while.
			delete(c.perDetector, det)
		}
	}
}

// ActiveCorrelations returns the top-K-per-detector anomalies that survive
// the QuantileFloor gate, one ActiveCorrelation per surviving anomaly. Each
// emitted correlation mirrors DetectorPassthroughCorrelator's single-anomaly
// shape so downstream reporters can score detections individually.
func (c *AnomalyRankCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Sort detector names for deterministic ordering across calls.
	detectors := make([]string, 0, len(c.perDetector))
	for d := range c.perDetector {
		detectors = append(detectors, d)
	}
	sort.Strings(detectors)

	var result []observer.ActiveCorrelation
	for _, det := range detectors {
		rb := c.perDetector[det]
		if len(rb.items) == 0 {
			continue
		}

		// Copy then sort descending by score (stable on ties for determinism).
		sorted := make([]rankedAnomaly, len(rb.items))
		copy(sorted, rb.items)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].score > sorted[j].score
		})

		// Quantile gate: only emit items whose score falls in the top
		// QuantileFloor fraction of in-window scores.
		threshold := -math.MaxFloat64
		if c.cfg.QuantileFloor < 1 && len(sorted) > 0 {
			// q is the index such that scores[0..q] are the top
			// QuantileFloor fraction. floor(len * QuantileFloor) gives the
			// count of "top" items; clamp to [1, len].
			count := int(math.Floor(float64(len(sorted)) * c.cfg.QuantileFloor))
			if count < 1 {
				count = 1
			}
			if count > len(sorted) {
				count = len(sorted)
			}
			threshold = sorted[count-1].score
		}

		// Take the top min(TopKPerDetector, len) items whose score >= threshold.
		limit := c.cfg.TopKPerDetector
		if limit > len(sorted) {
			limit = len(sorted)
		}
		kept := make([]rankedAnomaly, 0, limit)
		for i := 0; i < limit; i++ {
			if sorted[i].score < threshold {
				break
			}
			kept = append(kept, sorted[i])
		}

		// Sort kept items by timestamp ascending so the per-detector emission
		// order is stable and chronological — easier for reporters to consume.
		sort.SliceStable(kept, func(i, j int) bool {
			return kept[i].ts < kept[j].ts
		})

		for i, ra := range kept {
			a := ra.anomaly
			result = append(result, observer.ActiveCorrelation{
				Pattern:     fmt.Sprintf("anomaly_rank_%s_%d", det, i),
				Title:       fmt.Sprintf("Ranked[%s]: %s", det, a.Source),
				Members:     []observer.SeriesDescriptor{a.Source},
				Anomalies:   []observer.Anomaly{a},
				FirstSeen:   a.Timestamp,
				LastUpdated: a.Timestamp,
			})
		}
	}

	return result
}

// Reset clears all internal state.
func (c *AnomalyRankCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perDetector = make(map[string]*rankBuffer)
	c.currentDataTs = 0
}
