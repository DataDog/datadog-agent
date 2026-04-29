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
}

func buildRateKey(metric *Metric) rateKey {
	sortedTags := slices.Clone(metric.Tags)
	slices.Sort(sortedTags)

	return rateKey{
		metricName: metric.Name,
		tagsKey:    strings.Join(sortedTags, ","),
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

func (r *RateCalculator) processMetric(metric *Metric, timestamp time.Time) {
	if metric.RateCalculationMode == NoRateCalculation {
		return
	}

	key := buildRateKey(metric)
	previous, ok := r.previousValues[key]

	r.previousValues[key] = previousValue{
		value:     metric.Value,
		timestamp: timestamp,
	}

	if !ok {
		// No previous value, so no rate at all.
		metric.Value = 0
		return
	}

	delta := metric.Value - previous.value
	if delta < 0 {
		delta = 0
	}

	if metric.RateCalculationMode == AbsoluteDeltaRateCalculation {
		metric.Value = delta
		return
	}

	timeDiff := timestamp.Sub(previous.timestamp).Seconds()
	if timeDiff <= 0 {
		// Time difference is negative or zero, so no rate at all.
		metric.Value = 0
		return
	}

	metric.Value = delta / timeDiff
}

// ProcessMetrics processes a list of metrics and calculates the rate for each
// metric if appropriate. Leaves unmodified the metrics that do not require rate
// calculation
func (r *RateCalculator) ProcessMetrics(metrics []*Metric, timestamp time.Time) {
	for _, metric := range metrics {
		r.processMetric(metric, timestamp)
	}
}
