// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package telemetry

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Counter tracks how many times something is happening.
type Counter interface {
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
}

// NewCounter creates a Counter with default options for telemetry purpose.
// Current implementation used: Prometheus Counter
func NewCounter(subsystem, name string, tags []string, help string) Counter {
	return NewCounterWithOpts(subsystem, name, tags, help, DefaultOptions)
}

// NewCounterWithOpts creates a Counter with the given options for telemetry purpose.
// See NewCounter()
func NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter {
	// subsystem is optional
	if subsystem != "" && !opts.NoDoubleUnderscoreSep {
		// Prefix metrics with a _, prometheus will add a second _
		// It will create metrics with a custom separator and
		// will let us replace it to a dot later in the process.
		name = fmt.Sprintf("_%s", name)
	}

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
