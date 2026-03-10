// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nccl

import "time"

const (
	// ncclMetricsNs is the namespace for all NCCL metrics
	ncclMetricsNs = "nccl."

	// defaultSocketPath is where the agent listens for inspector plugin connections.
	// Training pods must mount the host's /var/run/datadog/ directory to reach it.
	defaultSocketPath = "/var/run/datadog/nccl.socket"

	// hangDetectionMetric is the metric emitted for each known rank.
	// Value is the number of seconds since that rank last produced an event.
	// Use for hang detection (e.g. alert when > 30s).
	hangDetectionMetric = "rank.seconds_since_last_event"

	// networkMaxTransferTimeMetric is the maximum proxy operation network time
	// observed for a rank in a check interval, aggregated across all proxy operations.
	// Tags: rank, direction (send/recv). Cardinality: 2N.
	networkMaxTransferTimeMetric = "proxy_op.max_network_time_us"

	// intraNodeDivergenceMetric is the intra-node rank divergence metric.
	// Value is max(exec_time_us) − min(exec_time_us) across ranks observed on the same node
	// for a single collective. Only emitted when 2+ ranks are seen in one check run.
	intraNodeDivergenceMetric = "intra_node_rank_divergence_us"
)

// rankStalenessMaxAge is the TTL for lastSeenRank entries. A rank that has not
// produced events for this long is evicted from the map so that stale entries from
// finished jobs (or ranks that migrated to a different node) do not generate
// indefinitely-growing hang-detection signals.
const rankStalenessMaxAge = 5 * time.Minute
