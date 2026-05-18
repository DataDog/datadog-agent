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
	// Flush returns Buckets aggregated at interval granularity for the metric
	// identified by name+tags within the half-open interval [start, stop).
	//
	// start and stop are Unix nanoseconds.
	// tags is optional: nil matches all tag combinations for the metric name.
	// interval is the aggregation bucket width; zero defaults to 1 second.
	//
	// Returns ErrDisabled if lookback.enabled is false.
	// Returns ErrNoData if no sealed WAL files overlap [start, stop).
	Flush(ctx context.Context, name string, tags []string, start, stop int64, interval time.Duration) ([]Bucket, error)
}
