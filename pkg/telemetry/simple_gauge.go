// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SimpleGauge tracks how many times something is happening.
type SimpleGauge interface {
	// Inc increments the gaguge.
	Inc()
	// Dec decrements the gauge.
	Dec()
	// Add increments the gauge by given amount.
	Add(float64)
	// Sub decrements the gauge by given amount.
	Sub(float64)
	// Set sets the value of the gauge.
	Set(float64)
}

// NewSimpleGauge creates a new SimpleGauge with default options.
func NewSimpleGauge(subsystem, name, help string) SimpleGauge {
	return NewSimpleGaugeWithOpts(subsystem, name, help, DefaultOptions)
}

// NewSimpleGaugeWithOpts creates a new SimpleGauge.
func NewSimpleGaugeWithOpts(subsystem, name, help string, opts Options) SimpleGauge {
	name = opts.NameWithSeparator(subsystem, name)

	pc := prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})

	telemetryRegistry.MustRegister(pc)

	return pc
}
