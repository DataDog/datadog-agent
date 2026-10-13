// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metricsevaluatorimpl

import (
	"slices"
	"time"

	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// sample is the value of a monitor's metric at one evaluation. ok is false when no series matched.
type sample struct {
	value float64
	ok    bool
}

// monitorState holds a monitor's samples in a ring buffer covering its window.
type monitorState struct {
	monitor metricsevaluator.Monitor
	state   metricsevaluator.State

	samples []sample
	next    int // index the next sample is written to
	filled  int // number of samples written, capped at len(samples)
}

func newMonitorState(m metricsevaluator.Monitor, interval time.Duration) *monitorState {
	// A window of N intervals is bounded by N+1 samples, which a delta needs to span the whole window.
	intervals := int((m.Window + interval - 1) / interval)
	return &monitorState{
		monitor: m,
		state:   metricsevaluator.StateNoData,
		samples: make([]sample, intervals+1),
	}
}

func (ms *monitorState) push(s sample) {
	ms.samples[ms.next] = s
	ms.next = (ms.next + 1) % len(ms.samples)
	if ms.filled < len(ms.samples) {
		ms.filled++
	}
}

// aggregate reduces the window to a single value. It returns false until the window is full, or when no sample
// of the window has a value.
func (ms *monitorState) aggregate() (float64, bool) {
	if ms.filled < len(ms.samples) {
		return 0, false
	}

	// The ring is full, so the oldest sample is the one about to be overwritten.
	var values []float64
	for i := range ms.samples {
		if s := ms.samples[(ms.next+i)%len(ms.samples)]; s.ok {
			values = append(values, s.value)
		}
	}
	if len(values) == 0 {
		return 0, false
	}

	switch ms.monitor.Aggregation {
	case metricsevaluator.AggregationAvg:
		var sum float64
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values)), true
	case metricsevaluator.AggregationMax:
		return slices.Max(values), true
	case metricsevaluator.AggregationMin:
		return slices.Min(values), true
	case metricsevaluator.AggregationDelta:
		// Sum the increase between consecutive samples. A decrease means the counter was reset
		// (telemetry.Component.Reset) and restarted from zero, so the sample after it counts in full.
		var delta float64
		for i := 1; i < len(values); i++ {
			if values[i] >= values[i-1] {
				delta += values[i] - values[i-1]
			} else {
				delta += values[i]
			}
		}
		return delta, true
	}
	return 0, false
}

// monitorSample computes the current sample of m from the gathered metric families.
func monitorSample(m metricsevaluator.Monitor, families map[string]*telemetry.MetricFamily) sample {
	value, ok := seriesValue(families[m.Metric], m.Tags)
	if !ok || m.DivideBy == "" {
		return sample{value: value, ok: ok}
	}

	divisor, ok := seriesValue(families[m.DivideBy], m.Tags)
	if !ok || divisor == 0 {
		return sample{}
	}
	return sample{value: value / divisor, ok: true}
}

// seriesValue sums the series of mf whose labels contain all tags.
// It returns false when mf is nil or no series matches.
func seriesValue(mf *telemetry.MetricFamily, tags map[string]string) (float64, bool) {
	if mf == nil {
		return 0, false
	}

	var sum float64
	var found bool
	for _, m := range mf.GetMetric() {
		matched := 0
		for _, lp := range m.GetLabel() {
			if v, ok := tags[lp.GetName()]; ok && v == lp.GetValue() {
				matched++
			}
		}
		if matched != len(tags) {
			continue
		}

		switch {
		case m.Gauge != nil:
			sum += m.GetGauge().GetValue()
		case m.Counter != nil:
			sum += m.GetCounter().GetValue()
		case m.Untyped != nil:
			sum += m.GetUntyped().GetValue()
		default:
			// Histograms and summaries have no single value.
			continue
		}
		found = true
	}
	return sum, found
}
