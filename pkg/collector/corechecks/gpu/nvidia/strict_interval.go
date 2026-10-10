// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import "time"

const (
	strictIntervalTolerance = 0.1

	// Prevent a long gap from generating unbounded samples.
	maxStrictIntervalPoints = 10
)

var staticMetricNames = map[string]struct{}{
	"core.limit":        {},
	"device.total":      {},
	"memory.bar1.total": {},
	"memory.limit":      {},
}

type strictIntervalKey struct {
	metricName string
	gpuUUID    string
}

// StrictIntervalProcessor emits metrics at their StrictInterval.
type StrictIntervalProcessor struct {
	staticMetricsInterval time.Duration
	lastEmitted           map[strictIntervalKey]time.Time
}

func NewStrictIntervalProcessor(staticMetricsInterval time.Duration) *StrictIntervalProcessor {
	return &StrictIntervalProcessor{
		staticMetricsInterval: staticMetricsInterval,
		lastEmitted:           make(map[strictIntervalKey]time.Time),
	}
}

// ProcessSamples updates timestamps for metrics with a StrictInterval.
func (p *StrictIntervalProcessor) ProcessSamples(samples []Sample, timestamp time.Time, gpuUUID string) []Sample {
	processed := make([]Sample, 0, len(samples))
	for _, sample := range samples {
		processed = p.processSample(processed, sample, timestamp, gpuUUID)
	}
	return processed
}

func (p *StrictIntervalProcessor) processSample(out []Sample, sample Sample, timestamp time.Time, gpuUUID string) []Sample {
	metric, ok := sample.(*Metric)
	if !ok {
		return append(out, sample)
	}

	if _, isStatic := staticMetricNames[metric.Name]; isStatic && metric.StrictInterval == 0 {
		metric.StrictInterval = p.staticMetricsInterval
	}
	if metric.StrictInterval <= 0 {
		return append(out, metric)
	}

	key := strictIntervalKey{metricName: metric.Name, gpuUUID: gpuUUID}
	last, seen := p.lastEmitted[key]
	if !seen {
		metric.Timestamp = timestamp
		p.lastEmitted[key] = timestamp
		return append(out, metric)
	}

	interval := metric.StrictInterval
	tolerance := time.Duration(float64(interval) * strictIntervalTolerance)

	caughtUp := false
	for points := 0; points < maxStrictIntervalPoints; points++ {
		diff := timestamp.Sub(last)
		if diff < interval-tolerance {
			caughtUp = true
			break
		}

		last = last.Add(interval)

		point := metric
		if points > 0 {
			point = metric.Clone().(*Metric)
		}
		point.Timestamp = last
		out = append(out, point)

		if diff <= interval+tolerance {
			caughtUp = true
			break
		}
	}

	if !caughtUp {
		last = timestamp
	}

	p.lastEmitted[key] = last
	return out
}
