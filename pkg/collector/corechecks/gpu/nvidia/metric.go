// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvidia

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name  string   // Name holds the name of the metric.
	Value float64  // Value holds the value of the metric.
	Tags  []string // Tags holds the tags associated with the metric.
}
