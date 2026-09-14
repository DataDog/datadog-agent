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

// ProcessMetrics updates timestamps for metrics with a StrictInterval.
func (p *StrictIntervalProcessor) ProcessMetrics(metrics []*Metric, timestamp time.Time, gpuUUID string) []*Metric {
	processed := make([]*Metric, 0, len(metrics))
	for _, metric := range metrics {
		processed = p.processMetric(processed, metric, timestamp, gpuUUID)
	}
	return processed
}

func (p *StrictIntervalProcessor) processMetric(out []*Metric, metric *Metric, timestamp time.Time, gpuUUID string) []*Metric {
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

		point := *metric
		point.Timestamp = last
		out = append(out, &point)

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
