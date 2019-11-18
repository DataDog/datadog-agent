// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

// Stub implementatoin of a Gauge.
// It is used when the telemetry is disabled.
type noopGauge struct {
}

// Set does nothing.
func (g *noopGauge) Set(value float64, tags ...string) {
}

// Inc does nothing.
func (g *noopGauge) Inc(tags ...string) {
}

// Dec does nothing.
func (g *noopGauge) Dec(tags ...string) {
}

// Delete does nothing.
func (g *noopGauge) Delete(tags ...string) {
}

// Add does nothing.
func (g *noopGauge) Add(value float64, tags ...string) {
}

// Sub does nothing.
func (g *noopGauge) Sub(value float64, tags ...string) {
}
