// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

// Stub implementation of a counter.
// It is used when the telemetry is disabled.
type noopCounter struct {
}

// Set does nothing.
func (g *noopCounter) Set(value float64, tags ...string) {
}

// Inc does nothing.
func (g *noopCounter) Inc(tags ...string) {
}

// Dec does nothing.
func (g *noopCounter) Dec(tags ...string) {
}

// Delete does nothing.
func (g *noopCounter) Delete(tags ...string) {
}

// Add does nothing.
func (g *noopCounter) Add(value float64, tags ...string) {
}

// Sub does nothing.
func (g *noopCounter) Sub(value float64, tags ...string) {
}
