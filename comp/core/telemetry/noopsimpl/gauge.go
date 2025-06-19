// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noopsimpl

import "github.com/DataDog/datadog-agent/comp/core/telemetry"

// Gauge implementation using Prometheus.
type slsGauge struct{}

// Set stores the value for the given tags.
func (g *slsGauge) Set(float64, ...string) {}

// Inc increments the Gauge value.
func (g *slsGauge) Inc(...string) {}

// Dec decrements the Gauge value.
func (g *slsGauge) Dec(...string) {}

// Delete deletes the value for the Gauge with the given tags.
func (g *slsGauge) Delete(...string) {}

// DeletePartialMatch deletes the values for the Gauges that match the subset of given tags
func (g *slsGauge) DeletePartialMatch(map[string]string) {}

// Add adds the value to the Gauge value.
func (g *slsGauge) Add(float64, ...string) {}

// Sub subtracts the value to the Gauge value.
func (g *slsGauge) Sub(float64, ...string) {}

// WithValues returns SimpleGauge for this metric with the given tag values.
func (g *slsGauge) WithValues(...string) telemetry.SimpleGauge {
	return &simpleNoOpGauge{}
}

// Withtags returns SimpleGauge for this metric with the given tag values.
func (g *slsGauge) WithTags(map[string]string) telemetry.SimpleGauge {
	return &simpleNoOpGauge{}
}
