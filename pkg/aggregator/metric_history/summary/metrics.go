// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

// ExtractMetricPattern finds the common prefix among metric names
// and returns the varying suffixes.
//
// Examples:
//
//	[system.disk.free, system.disk.used] -> Family: "system.disk", Variants: ["free", "used"]
//	[system.cpu.user.total, system.cpu.system.total] -> Family: "system.cpu", Variants: ["user.total", "system.total"]
//	[system.load.1, system.load.1] -> Family: "system.load.1", Variants: []
func ExtractMetricPattern(events []AnomalyEvent) MetricPattern {
	// TODO: implement
	return MetricPattern{}
}
