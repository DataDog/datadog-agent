// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookback provides the disk-based lookback ring buffer component.
// It subscribes to the metrics pipeline hook and stores pre-aggregation
// metric samples on disk as append-only WAL files, enabling callers to
// replay any past time window via Flush.
package lookback

// team: agent-metric-pipelines

import (
	"context"
	"errors"
	"time"
)

// ErrDisabled is returned by Flush when lookback.enabled is false.
var ErrDisabled = errors.New("lookback: component is disabled")

// ErrNoData is returned when no sealed WAL files cover the requested range.
var ErrNoData = errors.New("lookback: no data in range")

// Bucket is one aggregation interval of samples for a single metric+tag combination.
type Bucket struct {
	Name string
	Tags []string
	// Ts is the Unix nanosecond timestamp floored to the aggregation interval boundary.
	Ts    int64
	Count int64
	Sum   float64
	Min   float64
	Max   float64
}

// Component is the lookback ring buffer component interface.
type Component interface {
	// Flush returns Buckets aggregated at interval granularity for all metrics
	// matching name+tags within the half-open interval [start, stop).
	//
	// # name
	//
	// Exact name or a glob pattern (*, ?, [...] as in path.Match).
	// Examples:
	//   "system.cpu.user"   — exact match, single metric
	//   "kubernetes.*"      — all metrics in the kubernetes.* namespace
	//   "cilium.bpf.*"      — all cilium BPF metrics
	//   "*.memory.rss"      — memory.rss across all namespaces
	//
	// Exact names resolve in O(context_store_size / num_shards).
	// Glob patterns scan the full context store: O(context_store_size).
	//
	// # tags
	//
	// Optional inclusion filter. When non-nil, only contexts whose tag set
	// contains ALL of the requested tags are returned. This is a subset
	// match, not an exclusive match: a context tagged [env:prod, region:us]
	// IS returned by tags=["env:prod"], even though it carries extra tags.
	//
	// Examples:
	//   nil              — return all tag combinations for the metric name
	//   ["env:prod"]     — return only contexts that include env:prod
	//   ["env:prod",
	//    "region:us"]    — return only contexts that include both tags
	//
	// # interval
	//
	// Aggregation bucket width. Zero defaults to 1 second.
	// Each Bucket aggregates all raw samples whose timestamp falls within
	// one interval-width window: [floor(ts, interval), floor(ts, interval)+interval).
	//
	// # time range
	//
	// start and stop are Unix nanoseconds, half-open: [start, stop).
	// Only sealed WAL windows overlapping the range are queried; the active
	// (current) window is not included.
	//
	// Returns ErrDisabled if lookback.enabled is false.
	// Returns ErrNoData if no contexts match or no sealed WAL data covers the range.
	Flush(ctx context.Context, name string, tags []string, start, stop int64, interval time.Duration) ([]Bucket, error)
}
