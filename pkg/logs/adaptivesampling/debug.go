// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package adaptivesampling

// DebugLogPrefix is shared by the anomaly-driven adaptive sampling POC logs.
const DebugLogPrefix = "[logs/adaptive-sampling-poc]"

// ShouldLogDebugSample bounds high-volume POC logs while still making startup
// and steady-state flow visible in Agent logs.
func ShouldLogDebugSample(count uint64) bool {
	return count <= 10 || count%1000 == 0
}

// TruncateDebugString trims high-cardinality log samples for Agent log output.
func TruncateDebugString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
