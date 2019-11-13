// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

// Gauge tracks the value of one health metric of the Agent.
type Gauge interface {
	// Set stores the value for the given tags.
	Set(value float64, tags ...string)
	// Inc increments the Gauge value.
	Inc(tags ...string)
	// Dec decrements the Gauge value.
	Dec(tags ...string)
	// Add adds the value to the Gauge value.
	Add(value float64, tags ...string)
	// Sub subtracts the value to the Gauge value.
	Sub(value float64, tags ...string)
	// Delete deletes the value for the Gauge with the given tags.
	Delete(tags ...string)
}
