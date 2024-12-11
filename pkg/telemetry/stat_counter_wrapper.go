// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides types and functions for internal telemetry
package telemetry

import (
	"go.uber.org/atomic"
)

// StatCounterWrapper is a convenience type that allows for migrating telemetry to
// prometheus Counters while continuing to make the underlying values available for reading
type StatCounterWrapper struct {
	stat    *atomic.Int64
	counter Counter
}

// Inc increments the counter with the given tags value.
func (sgw *StatCounterWrapper) Inc(tags ...string) {
	sgw.stat.Inc()
	sgw.counter.Inc(tags...)
}

// IncWithTags increments the counter with the given tags value.
func (sgw *StatCounterWrapper) IncWithTags(tags map[string]string) {
	sgw.stat.Inc()
	sgw.counter.IncWithTags(tags)
}

// Delete deletes the value for the counter with the given tags value.
func (sgw *StatCounterWrapper) Delete() {
	sgw.stat.Store(0)
	sgw.counter.Delete()
}

// Add adds the given value to the counter with the given tags value.
func (sgw *StatCounterWrapper) Add(v int64, tags ...string) {
	sgw.stat.Add(v)
	sgw.counter.Add(float64(v), tags...)
}

// Load atomically loads the wrapped value.
func (sgw *StatCounterWrapper) Load() int64 {
	return sgw.stat.Load()
}

// NewStatCounterWrapper returns a new StatCounterWrapper
func NewStatCounterWrapper(subsystem string, statName string, tags []string, description string) *StatCounterWrapper {
	return &StatCounterWrapper{
		stat:    atomic.NewInt64(0),
		counter: NewCounter(subsystem, statName, tags, description),
	}
}
