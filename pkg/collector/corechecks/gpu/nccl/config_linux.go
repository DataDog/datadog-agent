// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nccl

import "time"

const (
	// ncclMetricsNs is the namespace for all NCCL metrics. Prefixed with "gpu."
	// to align with the parent GPU check (gpuMetricsNs = "gpu.") and this check's
	// own config namespace (gpu.nccl.*) → metrics are gpu.nccl.collective.* etc.
	ncclMetricsNs = "gpu.nccl."

	// hangDetectionMetric is the metric emitted for each known rank.
	// Value is the number of seconds since that rank last produced an event.
	// Use for hang detection (e.g. alert when > 30s).
	hangDetectionMetric = "rank.seconds_since_last_event"
)

// rankStalenessMaxAge is the TTL for lastSeenRank entries. A rank that has not
// produced events for this long is evicted from the map so that stale entries from
// finished jobs (or ranks that migrated to a different node) do not generate
// indefinitely-growing hang-detection signals.
const rankStalenessMaxAge = 5 * time.Minute
