// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	telemetryDef "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// Counter tracks how many times something is happening.
type Counter = telemetryDef.Counter

// NewCounter creates a Counter with default options for telemetry purpose.
// Current implementation used: Prometheus Counter
func NewCounter(subsystem, name string, tags []string, help string) Counter {
	return GetCompatComponent().NewCounter(subsystem, name, tags, help)
}
