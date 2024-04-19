// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noopsimpl

import "github.com/DataDog/datadog-agent/comp/core/telemetry"

// Gauge implementation using Prometheus.
type slsGauge struct{}

// Set stores the value for the given tags.
func (g *slsGauge) Set(value float64, tagsValue ...string) {}

// Inc increments the Gauge value.
func (g *slsGauge) Inc(tagsValue ...string) {}

// Dec decrements the Gauge value.
func (g *slsGauge) Dec(tagsValue ...string) {}

// Delete deletes the value for the Gauge with the given tags.
func (g *slsGauge) Delete(tagsValue ...string) {}

// Add adds the value to the Gauge value.
func (g *slsGauge) Add(value float64, tagsValue ...string) {}

// Sub subtracts the value to the Gauge value.
func (g *slsGauge) Sub(value float64, tagsValue ...string) {}

// WithValues returns SimpleGauge for this metric with the given tag values.
func (g *slsGauge) WithValues(tagsValue ...string) telemetry.SimpleGauge {
	return &simpleNoOpGauge{}
}

// Withtags returns SimpleGauge for this metric with the given tag values.
func (g *slsGauge) WithTags(tags map[string]string) telemetry.SimpleGauge {
	return &simpleNoOpGauge{}
}
