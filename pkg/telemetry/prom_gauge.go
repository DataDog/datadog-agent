// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// NewGauge creates a Gauge for telemetry purpose.
func NewGauge(subsystem, name string, tags []string, help string) Gauge {
	g := &promGauge{
		pg: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
			},
			tags,
		),
	}
	prometheus.MustRegister(g.pg)
	return g
}

// Gauge implementation using Prometheus.
type promGauge struct {
	pg   *prometheus.GaugeVec
	once sync.Once
}

// Set stores the value for the given tags.
func (g *promGauge) Set(value float64, tags ...string) {
	g.pg.WithLabelValues(tags...).Set(value)
}

// Inc increments the Gauge value.
func (g *promGauge) Inc(tags ...string) {
	g.pg.WithLabelValues(tags...).Inc()
}

// Dec decrements the Gauge value.
func (g *promGauge) Dec(tags ...string) {
	g.pg.WithLabelValues(tags...).Dec()
}

// Delete deletes the value for the Gauge with the given tags.
func (g *promGauge) Delete(tags ...string) {
	g.pg.DeleteLabelValues(tags...)
}

// Add adds the value to the Gauge value.
func (g *promGauge) Add(value float64, tags ...string) {
	g.pg.WithLabelValues(tags...).Add(value)
}

// Sub subtracts the value to the Gauge value.
func (g *promGauge) Sub(value float64, tags ...string) {
	g.pg.WithLabelValues(tags...).Sub(value)
}
