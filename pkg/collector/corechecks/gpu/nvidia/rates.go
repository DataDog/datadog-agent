// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package nvidia

import (
	"slices"
	"strings"
	"time"
)

type RateCalculationMode int

const (
	NoRateCalculation RateCalculationMode = iota
	AbsoluteDeltaRateCalculation
	PerSecondRateCalculation
)

type rateKey struct {
	metricName string
	tagsKey    string
	gpuUUID    string
}

func buildRateKey(metric *Metric, gpuUUID string) rateKey {
	sortedTags := slices.Clone(metric.Tags)
	slices.Sort(sortedTags)

	sortedWorkloads := make([]string, 0, len(metric.AssociatedWorkloads))
	for _, workload := range metric.AssociatedWorkloads {
		sortedWorkloads = append(sortedWorkloads, string(workload.Kind)+":"+workload.ID)
	}
	slices.Sort(sortedWorkloads)

	return rateKey{
		metricName: metric.Name,
		tagsKey:    strings.Join(sortedTags, ",") + "|workloads:" + strings.Join(sortedWorkloads, ","),
		gpuUUID:    gpuUUID,
	}
}

type previousValue struct {
	value     float64
	timestamp time.Time
}

// RateCalculator is a struct that calculates the rate of metrics based on previous values
type RateCalculator struct {
	previousValues map[rateKey]previousValue
}

// NewRateCalculator creates a new RateCalculator
func NewRateCalculator() *RateCalculator {
	return &RateCalculator{
		previousValues: make(map[rateKey]previousValue),
	}
}

// processMetric processes a single metric and returns true if the metric should be included in the output, false if it should be dropped.
func (r *RateCalculator) processMetric(metric *Metric, timestamp time.Time, gpuUUID string) bool {
	if metric.RateCalculationMode == NoRateCalculation {
		return true
	}

	key := buildRateKey(metric, gpuUUID)
	previous, ok := r.previousValues[key]

	r.previousValues[key] = previousValue{
		value:     metric.Value,
		timestamp: timestamp,
	}

	if !ok {
		// No previous value, so no rate yet.
		return false
	}

	delta := metric.Value - previous.value
	if delta < 0 {
		delta = 0
	}

	if metric.RateCalculationMode == AbsoluteDeltaRateCalculation {
		metric.Value = delta
		return true
	}

	timeDiff := timestamp.Sub(previous.timestamp).Seconds()
	if timeDiff <= 0 {
		// Time difference is negative or zero, so no rate at all.
		metric.Value = 0
		return true
	}

	metric.Value = delta / timeDiff
	return true
}

// ProcessMetrics processes a list of metrics and calculates the rate for each
// metric if appropriate. Metrics that require rate calculation and don't have a
// previous value are dropped.
func (r *RateCalculator) ProcessMetrics(metrics []*Metric, timestamp time.Time, gpuUUID string) []*Metric {
	filteredMetrics := make([]*Metric, 0, len(metrics))
	for _, metric := range metrics {
		if r.processMetric(metric, timestamp, gpuUUID) {
			filteredMetrics = append(filteredMetrics, metric)
		}
	}
	return filteredMetrics
}
