// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

// Gauge tracks the value of one health metric of the Agent.
type Gauge interface {
	// Set stores the value for the given tags.
	Set(value float64, tags ...string)
}
