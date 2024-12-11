// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"go.uber.org/atomic"
)

// StatGaugeWrapper is a convenience type that allows for migrating telemetry to
// prometheus Gauges while continuing to make the underlying values available for reading
type StatGaugeWrapper struct {
	stat  *atomic.Int64
	gauge Gauge
}

// Inc increments the Gauge value.
func (sgw *StatGaugeWrapper) Inc() {
	sgw.stat.Inc()
	sgw.gauge.Inc()
}

// Dec decrements the Gauge value.
func (sgw *StatGaugeWrapper) Dec() {
	sgw.stat.Dec()
	sgw.gauge.Dec()
}

// Add adds the value to the Gauge value.
func (sgw *StatGaugeWrapper) Add(v int64) {
	sgw.stat.Add(v)
	sgw.gauge.Add(float64(v))
}

// Set stores the value for the given tags.
func (sgw *StatGaugeWrapper) Set(v int64) {
	sgw.stat.Store(v)
	sgw.gauge.Set(float64(v))
}

// Load atomically loads the wrapped value.
func (sgw *StatGaugeWrapper) Load() int64 {
	return sgw.stat.Load()
}

// NewStatGaugeWrapper returns a new StatGaugeWrapper
func NewStatGaugeWrapper(subsystem string, statName string, tags []string, description string) *StatGaugeWrapper {
	return &StatGaugeWrapper{
		stat:  atomic.NewInt64(0),
		gauge: NewGauge(subsystem, statName, tags, description),
	}
}
