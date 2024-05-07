// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
)

// Gauge tracks the value of one health metric of the Agent.
type Gauge interface {
	telemetryComponent.Gauge
}

// NewGauge creates a Gauge with default options for telemetry purpose.
// Current implementation used: Prometheus Gauge
func NewGauge(subsystem, name string, tags []string, help string) Gauge {
	return NewGaugeWithOpts(subsystem, name, tags, help, DefaultOptions)
}

// NewGaugeWithOpts creates a Gauge with the given options for telemetry purpose.
// See NewGauge()
func NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge {
	return GetCompatComponent().NewGaugeWithOpts(subsystem, name, tags, help, telemetryComponent.Options(opts))
}
