// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetryimpl

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
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

// DeletePartialMatch deletes the values for the Gauges that match the subset of given tags
func (g *promGauge) DeletePartialMatch(tagsValue map[string]string) {
	g.pg.DeletePartialMatch(tagsValue)
}

// Add adds the value to the Gauge value.
func (g *promGauge) Add(value float64, tagsValue ...string) {
	g.pg.WithLabelValues(tagsValue...).Add(value)
}

// Sub subtracts the value to the Gauge value.
func (g *promGauge) Sub(value float64, tagsValue ...string) {
	g.pg.WithLabelValues(tagsValue...).Sub(value)
}

// WithValues returns SimpleGauge for this metric with the given tag values.
func (g *promGauge) WithValues(tagsValue ...string) telemetry.SimpleGauge {
	return &simplePromGauge{g: g.pg.WithLabelValues(tagsValue...)}
}

// Withtags returns SimpleGauge for this metric with the given tag values.
func (g *promGauge) WithTags(tags map[string]string) telemetry.SimpleGauge {
	return &simplePromGauge{g: g.pg.With(tags)}
}
