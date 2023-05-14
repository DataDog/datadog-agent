// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SimpleCounter tracks how many times something is happening.
type SimpleCounter interface {
	// Inc increments the counter.
	Inc()
	// Add increments the counter by given amount.
	Add(float64)
}

// NewSimpleCounter creates a new SimpleCounter with default options.
func NewSimpleCounter(subsystem, name, help string) SimpleCounter {
	return NewSimpleCounterWithOpts(subsystem, name, help, DefaultOptions)
}

// NewSimpleCounterWithOpts creates a new SimpleCounter.
func NewSimpleCounterWithOpts(subsystem, name, help string, opts Options) SimpleCounter {
	name = opts.NameWithSeparator(subsystem, name)

	pc := prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})

	telemetryRegistry.MustRegister(pc)

	return pc
}
