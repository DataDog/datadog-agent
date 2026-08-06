// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noisyneighbor contains the Noisy Neighbor check.
//
// EXPERIMENTAL — this module is unstable and under active development. Metric
// names, configuration keys, BPF map layout, and on-CPU overhead may all
// change without notice between releases, and the check may be removed
// outright. Use at your own risk; do not build production dashboards,
// monitors, or alerting on top of these metrics yet.
package noisyneighbor

const (
	// CheckName is the name of the check
	CheckName = "noisy_neighbor"
)
