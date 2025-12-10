// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Config holds the metric history configuration.
type Config struct {
	Enabled         bool
	IncludePrefixes []string
	ExcludePrefixes []string // metric prefixes to exclude from tracking
	RecentDuration  time.Duration
	MediumDuration  time.Duration
	LongDuration    time.Duration
	ExpiryDuration  time.Duration

	// Anomaly detection configuration
	AnomalyDetectionEnabled  bool
	DetectionIntervalFlushes int      // run detection every N flushes (default: 1, ~15 seconds)
	DetectorType             string   // "robust_zscore" (default), "bayesian_changepoint", or "mean_change"
	RobustZScoreThreshold    float64  // M-score threshold for robust_zscore (default: 3.5)
	BayesianHazard           float64  // changepoint hazard rate for bayesian (default: 0.01)
	BayesianThreshold        float64  // probability threshold for bayesian (default: 0.5)
	MinDataPoints            int      // minimum points before detection (default: 10)
	MinSeverity              float64  // minimum severity (0-1) to report anomaly (default: 0)
	ExcludeAnomalyPrefixes   []string // metric prefixes to exclude from anomaly detection

	// Debug server configuration (for local testing)
	DebugServerEnabled bool   // start HTTP server for snapshot capture
	DebugServerAddr    string // address for debug server (default: localhost:6063)
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		IncludePrefixes: []string{"system."},
		ExcludePrefixes: []string{},
		RecentDuration:  5 * time.Minute,
		MediumDuration:  1 * time.Hour,
		LongDuration:    24 * time.Hour,
		ExpiryDuration:  25 * time.Minute, // 100 flush cycles * 15s

		// Anomaly detection defaults
		AnomalyDetectionEnabled:  true,
		DetectionIntervalFlushes: 1,               // ~15 seconds at 15s flush interval
		DetectorType:             "robust_zscore", // robust_zscore, bayesian_changepoint, or mean_change
		RobustZScoreThreshold:    3.5,             // M-score threshold (3.0-3.5 typical)
		BayesianHazard:           0.01,            // expect changepoint every ~100 observations
		BayesianThreshold:        0.5,             // 50% probability to trigger
		MinDataPoints:            10,              // minimum points before detection
		MinSeverity:              0.0,             // report all anomalies by default
		ExcludeAnomalyPrefixes:   []string{},      // exclude no metrics by default

		// Debug server defaults
		DebugServerEnabled: false,
		DebugServerAddr:    "localhost:6063",
	}
}

// LoadConfig reads configuration from the agent config.
func LoadConfig(cfg model.Reader) Config {
	result := DefaultConfig()

	if cfg.IsSet("metric_history.enabled") {
		result.Enabled = cfg.GetBool("metric_history.enabled")
	}

	if cfg.IsSet("metric_history.include_metrics") {
		result.IncludePrefixes = cfg.GetStringSlice("metric_history.include_metrics")
	}

	if cfg.IsSet("metric_history.exclude_metrics") {
		result.ExcludePrefixes = cfg.GetStringSlice("metric_history.exclude_metrics")
	}

	if cfg.IsSet("metric_history.retention.recent_duration") {
		if duration, err := time.ParseDuration(cfg.GetString("metric_history.retention.recent_duration")); err == nil {
			result.RecentDuration = duration
		}
	}

	if cfg.IsSet("metric_history.retention.medium_duration") {
		if duration, err := time.ParseDuration(cfg.GetString("metric_history.retention.medium_duration")); err == nil {
			result.MediumDuration = duration
		}
	}

	if cfg.IsSet("metric_history.retention.long_duration") {
		if duration, err := time.ParseDuration(cfg.GetString("metric_history.retention.long_duration")); err == nil {
			result.LongDuration = duration
		}
	}

	if cfg.IsSet("metric_history.expiry_duration") {
		if duration, err := time.ParseDuration(cfg.GetString("metric_history.expiry_duration")); err == nil {
			result.ExpiryDuration = duration
		}
	}

	// Load anomaly detection configuration
	if cfg.IsSet("metric_history.anomaly_detection.enabled") {
		result.AnomalyDetectionEnabled = cfg.GetBool("metric_history.anomaly_detection.enabled")
	}

	if cfg.IsSet("metric_history.anomaly_detection.detection_interval_flushes") {
		result.DetectionIntervalFlushes = cfg.GetInt("metric_history.anomaly_detection.detection_interval_flushes")
	}

	if cfg.IsSet("metric_history.anomaly_detection.detector_type") {
		result.DetectorType = cfg.GetString("metric_history.anomaly_detection.detector_type")
	}

	if cfg.IsSet("metric_history.anomaly_detection.robust_zscore_threshold") {
		result.RobustZScoreThreshold = cfg.GetFloat64("metric_history.anomaly_detection.robust_zscore_threshold")
	}

	if cfg.IsSet("metric_history.anomaly_detection.bayesian_hazard") {
		result.BayesianHazard = cfg.GetFloat64("metric_history.anomaly_detection.bayesian_hazard")
	}

	if cfg.IsSet("metric_history.anomaly_detection.bayesian_threshold") {
		result.BayesianThreshold = cfg.GetFloat64("metric_history.anomaly_detection.bayesian_threshold")
	}

	if cfg.IsSet("metric_history.anomaly_detection.min_data_points") {
		result.MinDataPoints = cfg.GetInt("metric_history.anomaly_detection.min_data_points")
	}

	if cfg.IsSet("metric_history.anomaly_detection.min_severity") {
		result.MinSeverity = cfg.GetFloat64("metric_history.anomaly_detection.min_severity")
	}

	if cfg.IsSet("metric_history.anomaly_detection.exclude_metrics") {
		result.ExcludeAnomalyPrefixes = cfg.GetStringSlice("metric_history.anomaly_detection.exclude_metrics")
	}

	// Load debug server configuration
	if cfg.IsSet("metric_history.debug_server.enabled") {
		result.DebugServerEnabled = cfg.GetBool("metric_history.debug_server.enabled")
	}

	if cfg.IsSet("metric_history.debug_server.addr") {
		result.DebugServerAddr = cfg.GetString("metric_history.debug_server.addr")
	}

	return result
}

// RecentCapacity returns the number of points to store at flush resolution.
// Assumes 15-second flush interval.
func (c Config) RecentCapacity() int {
	return int(c.RecentDuration / (15 * time.Second))
}

// MediumCapacity returns the number of 1-minute rollup points to store.
func (c Config) MediumCapacity() int {
	return int(c.MediumDuration / time.Minute)
}

// LongCapacity returns the number of 1-hour rollup points to store.
func (c Config) LongCapacity() int {
	return int(c.LongDuration / time.Hour)
}
