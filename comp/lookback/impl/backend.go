// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"context"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	lookback "github.com/DataDog/datadog-agent/comp/lookback/def"
)

// timeSeriesBackend is the storage abstraction for the lookback component.
// Both the custom WAL and the Prometheus TSDB backend implement this interface.
type timeSeriesBackend interface {
	// writeSample records one metric sample. Called from onSamples and walSender.Commit.
	writeSample(name string, tags []string, tsUs int64, value float64)

	// flush returns aggregated Buckets for the name/tag pattern over [startUs, stopUs).
	// name may contain glob wildcards (*, ?, [...]).
	// intervalUs is the bucket width in microseconds; 0 defaults to 1 000 000 µs (1 s).
	flush(ctx context.Context, name string, tags []string,
		startUs, stopUs, intervalUs int64) ([]lookback.Bucket, error)

	// startRotationTimer starts the WAL rotation goroutine. No-op for TSDB.
	startRotationTimer()

	// stop flushes pending data and closes all file handles.
	stop(ctx context.Context) error
}

// newBackend selects the storage backend at compile time.
// To switch: comment the active line and uncomment the other.
func newBackend(cfg storeConfig, l log.Component) (timeSeriesBackend, error) {
	return newWALBackend(cfg, l)      // ← custom WAL (default)
	// return newTSDBBackend(cfg, l)  // ← Prometheus TSDB
}
