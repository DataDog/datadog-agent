// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// AnchorTrustConfig configures the AnchorTrustCorrelator. It embeds
// TimeClusterConfig so all proximity/window tuning applies to the inner
// clusterer, and adds two cheap post-filter knobs applied at emission time.
type AnchorTrustConfig struct {
	// TimeClusterConfig controls the embedded TimeClusterCorrelator
	// (proximity, window, min cluster size). Inlined so JSON config files can
	// keep their existing time_cluster shape.
	TimeClusterConfig

	// AnchorDetectors is the list of DetectorName values whose anomalies are
	// treated as "trusted anchors". When a cluster contains an anomaly from
	// any anchor detector, the emitted FirstSeen / LastUpdated are the min /
	// max timestamps of the anchor anomalies only — preserving the narrow
	// time window of the high-precision detector even when adjacent
	// lower-precision detectors fire.
	//
	// Default: ["bocpd_detector"]. Note: BOCPDDetector.Name() returns
	// "bocpd_detector" (not "bocpd"); we match Anomaly.DetectorName, which
	// is set from Name(), not from the catalog key.
	AnchorDetectors []string `json:"anchor_detectors"`

	// MinDistinctDetectors is the quorum threshold applied to clusters that
	// contain NO anchor anomalies. A non-anchor cluster is only emitted if it
	// contains anomalies from at least this many distinct DetectorName
	// values; otherwise it is dropped (a single non-anchor detector is
	// considered too noisy to alert on).
	// Default: 2.
	MinDistinctDetectors int `json:"min_distinct_detectors"`
}

// DefaultAnchorTrustConfig returns the default config: TimeClusterConfig
// defaults, bocpd as the only anchor, and a 2-detector quorum.
func DefaultAnchorTrustConfig() AnchorTrustConfig {
	return AnchorTrustConfig{
		TimeClusterConfig:    DefaultTimeClusterConfig(),
		AnchorDetectors:      []string{"bocpd_detector"},
		MinDistinctDetectors: 2,
	}
}

// AnchorTrustCorrelator wraps a TimeClusterCorrelator and adds two emission-
// time gates:
//
//  1. Anchor preservation. If a cluster contains an anomaly from any of the
//     configured anchor detectors, the emitted FirstSeen / LastUpdated are the
//     min / max timestamps of the anchor anomalies only — not the cluster's
//     full span. The full anomaly list passes through unchanged so downstream
//     scoring still sees every contributing anomaly.
//  2. Quorum. If a cluster contains NO anchor anomalies, it is only emitted if
//     it contains anomalies from at least MinDistinctDetectors distinct
//     DetectorName values. Single-detector non-anchor clusters are dropped.
//
// Clustering itself (proximity grouping, eviction, sampling-interval-aware
// proximity widening) is delegated verbatim to the embedded
// TimeClusterCorrelator — this struct only post-processes the cluster list at
// ActiveCorrelations() time.
type AnchorTrustCorrelator struct {
	inner          *TimeClusterCorrelator
	anchorSet      map[string]struct{}
	minDistinctDet int
}

// NewAnchorTrustCorrelator constructs an AnchorTrustCorrelator. A zero-value
// AnchorTrustConfig is valid: anchor defaults to bocpd_detector and quorum
// defaults to 2.
func NewAnchorTrustCorrelator(cfg AnchorTrustConfig) *AnchorTrustCorrelator {
	anchors := cfg.AnchorDetectors
	if len(anchors) == 0 {
		anchors = []string{"bocpd_detector"}
	}
	anchorSet := make(map[string]struct{}, len(anchors))
	for _, name := range anchors {
		anchorSet[name] = struct{}{}
	}
	minDistinct := cfg.MinDistinctDetectors
	if minDistinct == 0 {
		minDistinct = 2
	}
	return &AnchorTrustCorrelator{
		inner:          NewTimeClusterCorrelator(cfg.TimeClusterConfig),
		anchorSet:      anchorSet,
		minDistinctDet: minDistinct,
	}
}

// Name returns the catalog name. Matches the catalog entry key so testbench
// --only and config readers stay consistent.
func (c *AnchorTrustCorrelator) Name() string { return "anchor_trust_correlator" }

// ProcessAnomaly delegates clustering to the inner TimeClusterCorrelator.
func (c *AnchorTrustCorrelator) ProcessAnomaly(a observer.Anomaly) {
	c.inner.ProcessAnomaly(a)
}

// Advance delegates time-based maintenance to the inner correlator.
func (c *AnchorTrustCorrelator) Advance(dataTime int64) { c.inner.Advance(dataTime) }

// Reset clears all internal state for reanalysis.
func (c *AnchorTrustCorrelator) Reset() { c.inner.Reset() }

// ActiveCorrelations returns the inner clusters after applying the anchor and
// quorum gates. Members and Anomalies pass through unchanged so the FP/TP
// scorer still sees every contributing anomaly; only the cluster-span fields
// (FirstSeen / LastUpdated) are narrowed when an anchor is present.
//
// Complexity: O(K) where K is the total anomaly count across active clusters.
// Allocations: one slice for the result, one small per-cluster map (bounded
// by the catalog detector count, ≤ 16 entries in practice).
func (c *AnchorTrustCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	raw := c.inner.ActiveCorrelations()
	if len(raw) == 0 {
		return nil
	}
	out := make([]observer.ActiveCorrelation, 0, len(raw))

	// Reuse a single distinct-detector map across clusters to avoid per-cluster
	// allocation in the steady state. The map is cleared between iterations.
	distinctDet := make(map[string]struct{}, 8)

	for _, cluster := range raw {
		// Clear the map (Go optimises this loop into a runtime call).
		for k := range distinctDet {
			delete(distinctDet, k)
		}

		var anchorMin, anchorMax int64
		var anchorCount int
		for _, a := range cluster.Anomalies {
			distinctDet[a.DetectorName] = struct{}{}
			if _, isAnchor := c.anchorSet[a.DetectorName]; isAnchor {
				if anchorCount == 0 || a.Timestamp < anchorMin {
					anchorMin = a.Timestamp
				}
				if anchorCount == 0 || a.Timestamp > anchorMax {
					anchorMax = a.Timestamp
				}
				anchorCount++
			}
		}

		switch {
		case anchorCount > 0:
			// Anchor preservation: narrow the emitted span to the anchor
			// anomalies' min / max. Members and Anomalies pass through.
			emitted := cluster
			emitted.FirstSeen = anchorMin
			emitted.LastUpdated = anchorMax
			out = append(out, emitted)
		case len(distinctDet) >= c.minDistinctDet:
			// Quorum reached: emit cluster unchanged.
			out = append(out, cluster)
		default:
			// Single non-anchor detector: drop.
		}
	}
	return out
}

// GetClusters proxies to the inner correlator's diagnostic view. Reporters
// that surface per-cluster info via ComponentDataProvider continue to see the
// raw clusters; the anchor / quorum gates only shape ActiveCorrelations().
func (c *AnchorTrustCorrelator) GetClusters() []TimeClusterInfo {
	return c.inner.GetClusters()
}

// GetExtraData implements ComponentDataProvider. Returns a small overview
// containing both the inner cluster diagnostics and the anchor configuration
// so the testbench UI can show why a cluster was kept or dropped.
func (c *AnchorTrustCorrelator) GetExtraData() interface{} {
	anchors := make([]string, 0, len(c.anchorSet))
	for name := range c.anchorSet {
		anchors = append(anchors, name)
	}
	sort.Strings(anchors)
	return map[string]interface{}{
		"clusters":               c.inner.GetClusters(),
		"anchor_detectors":       anchors,
		"min_distinct_detectors": c.minDistinctDet,
	}
}

// String implements fmt.Stringer for diagnostic logging.
func (c *AnchorTrustCorrelator) String() string {
	return fmt.Sprintf("AnchorTrustCorrelator(anchors=%d, min_distinct=%d)", len(c.anchorSet), c.minDistinctDet)
}
