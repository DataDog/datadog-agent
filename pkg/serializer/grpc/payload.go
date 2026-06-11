// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpc implements the stateful gRPC transport for v3 metrics.
package grpc

import (
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

// Payload is one stateful metric batch ready to ship over a gRPC stream: the
// wire bytes for MetricStatefulBatch.data plus the dictionary state-changes the
// receiver applies on ack.
type Payload struct {
	// Encoded is the wire-ready serialized + compressed MetricDatumSequence,
	// placed directly into MetricStatefulBatch.data.
	Encoded []byte

	// Encoding is the content encoding of Encoded ("zstd", "gzip", "identity"),
	// for telemetry; the wire encoding is carried by the gRPC channel header.
	Encoding string

	// UnencodedSize is the serialized MetricDatumSequence size before
	// compression, for compression-ratio telemetry.
	UnencodedSize int

	// PreCompressionBytes is the byte count before compression, for telemetry.
	PreCompressionBytes int

	// PointCount is the number of metric points in this payload's
	// MetricSeriesBatch (0 for a snapshot-only payload).
	PointCount int

	// StateChanges are the dict-define datums introduced by this payload (the
	// Metric*Define datums only, not the MetricSeriesBatch). On ack the
	// inflight tracker applies them to the per-stream snapshot, so the snapshot
	// accumulates only delivered state.
	StateChanges []*statefulpb.MetricDatum
}
