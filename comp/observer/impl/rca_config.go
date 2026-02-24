// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

// RCAScoringWeights control the temporal root ranking function.
type RCAScoringWeights struct {
	Onset              float64
	DownstreamCoverage float64
	Persistence        float64
	Severity           float64
	IncomingPenalty    float64
	SpreadPenalty      float64
}

// RCAConfig controls RCA behavior.
type RCAConfig struct {
	Enabled bool

	// Correlator selects which builder family to use.
	// Current supported value: "time_cluster".
	Correlator string

	MaxRootCandidates int
	MaxEvidencePaths  int

	// Time tolerance for deciding direction from onset ordering.
	OnsetEpsilonSeconds int64
	// Maximum lag considered for temporal-proximity edges.
	MaxEdgeLagSeconds int64

	// Confidence heuristics.
	MinDataNodes                int
	WeakDirectionalityThreshold float64
	AmbiguousRootMargin         float64

	// MinConfidence suppresses RCA results below this confidence score.
	MinConfidence float64

	Weights RCAScoringWeights
}

// DefaultRCAConfig returns conservative defaults.
func DefaultRCAConfig() RCAConfig {
	return RCAConfig{
		Enabled:                     false,
		Correlator:                  "time_cluster",
		MaxRootCandidates:           3,
		MaxEvidencePaths:            3,
		OnsetEpsilonSeconds:         1,
		MaxEdgeLagSeconds:           10,
		MinDataNodes:                3,
		WeakDirectionalityThreshold: 0.45,
		AmbiguousRootMargin:         0.08,
		MinConfidence:               0.5,
		Weights: RCAScoringWeights{
			Onset:              0.35,
			DownstreamCoverage: 0.25,
			Persistence:        0.15,
			Severity:           0.15,
			IncomingPenalty:    0.25,
			SpreadPenalty:      0.15,
		},
	}
}

func (c RCAConfig) normalized() RCAConfig {
	defaults := DefaultRCAConfig()

	if c.Correlator == "" {
		c.Correlator = defaults.Correlator
	}
	if c.MaxRootCandidates <= 0 {
		c.MaxRootCandidates = defaults.MaxRootCandidates
	}
	if c.MaxEvidencePaths <= 0 {
		c.MaxEvidencePaths = defaults.MaxEvidencePaths
	}
	if c.OnsetEpsilonSeconds < 0 {
		c.OnsetEpsilonSeconds = defaults.OnsetEpsilonSeconds
	}
	if c.MaxEdgeLagSeconds <= 0 {
		c.MaxEdgeLagSeconds = defaults.MaxEdgeLagSeconds
	}
	if c.MinDataNodes <= 0 {
		c.MinDataNodes = defaults.MinDataNodes
	}
	if c.WeakDirectionalityThreshold <= 0 {
		c.WeakDirectionalityThreshold = defaults.WeakDirectionalityThreshold
	}
	if c.AmbiguousRootMargin <= 0 {
		c.AmbiguousRootMargin = defaults.AmbiguousRootMargin
	}
	if c.MinConfidence <= 0 {
		c.MinConfidence = defaults.MinConfidence
	}

	if c.Weights.Onset <= 0 {
		c.Weights.Onset = defaults.Weights.Onset
	}
	if c.Weights.DownstreamCoverage <= 0 {
		c.Weights.DownstreamCoverage = defaults.Weights.DownstreamCoverage
	}
	if c.Weights.Persistence <= 0 {
		c.Weights.Persistence = defaults.Weights.Persistence
	}
	if c.Weights.Severity <= 0 {
		c.Weights.Severity = defaults.Weights.Severity
	}
	if c.Weights.IncomingPenalty <= 0 {
		c.Weights.IncomingPenalty = defaults.Weights.IncomingPenalty
	}
	if c.Weights.SpreadPenalty <= 0 {
		c.Weights.SpreadPenalty = defaults.Weights.SpreadPenalty
	}

	return c
}

func loadRCAConfig(cfg pkgconfigmodel.Reader) RCAConfig {
	config := DefaultRCAConfig()

	config.Enabled = cfg.GetBool("observer.rca.enabled")
	config.Correlator = cfg.GetString("observer.rca.correlator")
	config.MaxRootCandidates = cfg.GetInt("observer.rca.max_root_candidates")
	config.MaxEvidencePaths = cfg.GetInt("observer.rca.max_evidence_paths")
	config.OnsetEpsilonSeconds = cfg.GetInt64("observer.rca.onset_epsilon_seconds")
	config.MaxEdgeLagSeconds = cfg.GetInt64("observer.rca.max_edge_lag_seconds")
	config.MinDataNodes = cfg.GetInt("observer.rca.min_data_nodes")
	config.WeakDirectionalityThreshold = cfg.GetFloat64("observer.rca.weak_directionality_threshold")
	config.AmbiguousRootMargin = cfg.GetFloat64("observer.rca.ambiguous_root_margin")
	config.MinConfidence = cfg.GetFloat64("observer.rca.min_confidence")

	config.Weights.Onset = cfg.GetFloat64("observer.rca.weights.onset")
	config.Weights.DownstreamCoverage = cfg.GetFloat64("observer.rca.weights.downstream_coverage")
	config.Weights.Persistence = cfg.GetFloat64("observer.rca.weights.persistence")
	config.Weights.Severity = cfg.GetFloat64("observer.rca.weights.severity")
	config.Weights.IncomingPenalty = cfg.GetFloat64("observer.rca.weights.incoming_penalty")
	config.Weights.SpreadPenalty = cfg.GetFloat64("observer.rca.weights.spread_penalty")

	return config.normalized()
}
