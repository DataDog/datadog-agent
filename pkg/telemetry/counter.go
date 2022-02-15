// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Counter tracks how many times something is happening.
type Counter interface {
	// Initialize creates the counter with the given tags and initializes it to 0.
	// This method is intended to be used when the counter value is important to
	// send even before any incrementing/addition is done on it.
	Initialize(tagsValue ...string)
	// Inc increments the counter with the given tags value.
	Inc(tagsValue ...string)
	// Add adds the given value to the counter with the given tags value.
	Add(value float64, tagsValue ...string)
	// Delete deletes the value for the counter with the given tags value.
	Delete(tagsValue ...string)
	// IncWithTags increments the counter with the given tags.
	// Even if less convenient, this signature could be used in hot path
	// instead of Inc(...string) to avoid escaping the parameters on the heap.
	IncWithTags(tags map[string]string)
	// AddWithTags adds the given value to the counter with the given tags.
	// Even if less convenient, this signature could be used in hot path
	// instead of Add(float64, ...string) to avoid escaping the parameters on the heap.
	AddWithTags(value float64, tags map[string]string)
	// DeleteWithTags deletes the value for the counter with the given tags.
	// Even if less convenient, this signature could be used in hot path
	// instead of Delete(...string) to avoid escaping the parameters on the heap.
	DeleteWithTags(tags map[string]string)
	// WithValues returns SimpleCounter for this metric with the given tag values.
	WithValues(tagsValue ...string) SimpleCounter
	// WithTags returns SimpleCounter for this metric with the given tqg values.
	WithTags(tags map[string]string) SimpleCounter
}

// NewCounter creates a Counter with default options for telemetry purpose.
// Current implementation used: Prometheus Counter
func NewCounter(subsystem, name string, tags []string, help string) Counter {
	return NewCounterWithOpts(subsystem, name, tags, help, DefaultOptions)
}

// NewCounterWithOpts creates a Counter with the given options for telemetry purpose.
// See NewCounter()
func NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter {
	name = opts.NameWithSeparator(subsystem, name)

	c := &promCounter{
		pc: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
			},
			tags,
		),
	}
	telemetryRegistry.MustRegister(c.pc)
	return c
}
