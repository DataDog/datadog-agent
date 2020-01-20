// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Gauge implementation using Prometheus.
type promGauge struct {
	pg *prometheus.GaugeVec
}

// Set stores the value for the given tags.
func (g *promGauge) Set(value float64, tagsValue ...string) {
	g.pg.WithLabelValues(tagsValue...).Set(value)
}

// Inc increments the Gauge value.
func (g *promGauge) Inc(tagsValue ...string) {
	g.pg.WithLabelValues(tagsValue...).Inc()
}

// Dec decrements the Gauge value.
func (g *promGauge) Dec(tagsValue ...string) {
	g.pg.WithLabelValues(tagsValue...).Dec()
}

// Delete deletes the value for the Gauge with the given tags.
func (g *promGauge) Delete(tagsValue ...string) {
	g.pg.DeleteLabelValues(tagsValue...)
}

// Add adds the value to the Gauge value.
func (g *promGauge) Add(value float64, tagsValue ...string) {
	g.pg.WithLabelValues(tagsValue...).Add(value)
}

// Sub subtracts the value to the Gauge value.
func (g *promGauge) Sub(value float64, tagsValue ...string) {
	g.pg.WithLabelValues(tagsValue...).Sub(value)
}
