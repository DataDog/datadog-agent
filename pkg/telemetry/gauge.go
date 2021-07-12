// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package telemetry

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Gauge tracks the value of one health metric of the Agent.
type Gauge interface {
	// Set stores the value for the given tags.
	Set(value float64, tagsValue ...string)
	// Inc increments the Gauge value.
	Inc(tagsValue ...string)
	// Dec decrements the Gauge value.
	Dec(tagsValue ...string)
	// Add adds the value to the Gauge value.
	Add(value float64, tagsValue ...string)
	// Sub subtracts the value to the Gauge value.
	Sub(value float64, tagsValue ...string)
	// Delete deletes the value for the Gauge with the given tags.
	Delete(tagsValue ...string)
}

// NewGauge creates a Gauge with default options for telemetry purpose.
// Current implementation used: Prometheus Gauge
func NewGauge(subsystem, name string, tags []string, help string) Gauge {
	return NewGaugeWithOpts(subsystem, name, tags, help, DefaultOptions)
}

// NewGaugeWithOpts creates a Gauge with the given options for telemetry purpose.
// See NewGauge()
func NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge {
	// subsystem is optional
	if subsystem != "" && !opts.NoDoubleUnderscoreSep {
		// Prefix metrics with a _, prometheus will add a second _
		// It will create metrics with a custom separator and
		// will let us replace it to a dot later in the process.
		name = fmt.Sprintf("_%s", name)
	}

	g := &promGauge{
		pg: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
			},
			tags,
		),
	}
	telemetryRegistry.MustRegister(g.pg)
	return g
}
