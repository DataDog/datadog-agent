// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	telemetryDef "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// Gauge tracks the value of one health metric of the Agent.
type Gauge = telemetryDef.Gauge

// NewGauge creates a Gauge with default options for telemetry purpose.
// Current implementation used: Prometheus Gauge
func NewGauge(subsystem, name string, tags []string, help string) Gauge {
	return GetCompatComponent().NewGauge(subsystem, name, tags, help)
}
