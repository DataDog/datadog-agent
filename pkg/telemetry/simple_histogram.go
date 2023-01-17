// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SimpleHistogram tracks how many times something is happening.
type SimpleHistogram interface {
	// Observe the value to the Histogram value.
	Observe(value float64)
}

// NewSimpleHistogram creates a new SimpleHistogram with default options.
func NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram {
	return NewSimpleHistogramWithOpts(subsystem, name, help, buckets, DefaultOptions)
}

// NewSimpleHistogramWithOpts creates a new SimpleHistogram.
func NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram {
	name = opts.NameWithSeparator(subsystem, name)

	pc := prometheus.NewHistogram(prometheus.HistogramOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
		Buckets:   buckets,
	})

	telemetryRegistry.MustRegister(pc)

	return pc
}
