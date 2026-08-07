// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package metricresolution defines the validated configuration contract for the
// global metric-resolution experiment.
package metricresolution

import (
	"fmt"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	// EnabledKey enables the global metric-resolution experiment.
	EnabledKey = "metric_resolution_experiment.enabled"
	// CheckIntervalKey controls the effective interval of normal recurring checks.
	CheckIntervalKey = "metric_resolution_experiment.check_interval"
	// DogStatsDAggregationIntervalKey controls ordinary DogStatsD bucket width.
	DogStatsDAggregationIntervalKey = "metric_resolution_experiment.dogstatsd_aggregation_interval"
	// SerializerFlushIntervalKey controls metric serializer flush cadence.
	SerializerFlushIntervalKey = "metric_resolution_experiment.serializer_flush_interval"
)

// ExperimentConfig is the validated runtime contract for the experiment.
type ExperimentConfig struct {
	Enabled                      bool
	CheckInterval                time.Duration
	DogStatsDAggregationInterval time.Duration
	SerializerFlushInterval      time.Duration
}

// Read reads and validates the experiment configuration. Dormant interval
// values are not validated so disabled mode retains existing behavior.
func Read(config pkgconfigmodel.Reader) (ExperimentConfig, error) {
	result := ExperimentConfig{
		Enabled:                      config.GetBool(EnabledKey),
		CheckInterval:                config.GetDuration(CheckIntervalKey),
		DogStatsDAggregationInterval: config.GetDuration(DogStatsDAggregationIntervalKey),
		SerializerFlushInterval:      config.GetDuration(SerializerFlushIntervalKey),
	}
	if !result.Enabled {
		return result, nil
	}

	intervals := []struct {
		key      string
		interval time.Duration
	}{
		{CheckIntervalKey, result.CheckInterval},
		{DogStatsDAggregationIntervalKey, result.DogStatsDAggregationInterval},
		{SerializerFlushIntervalKey, result.SerializerFlushInterval},
	}
	for _, configured := range intervals {
		if configured.interval < time.Second || configured.interval%time.Second != 0 {
			return ExperimentConfig{}, fmt.Errorf("%s must be a whole-second duration of at least 1s, got %s", configured.key, configured.interval)
		}
	}

	return result, nil
}
