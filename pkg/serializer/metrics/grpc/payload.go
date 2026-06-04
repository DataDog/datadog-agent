// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpc implements the stateful gRPC transport for v3 metrics.
//
// This package is intentionally self-contained and does NOT import from
// pkg/logs/* (per contract.md and the implementation plan in
// phase3-proto-proposal.md §11). It mirrors selected pieces of the logs
// stateful sender (pkg/logs/sender/grpc/) but is a separate codebase so
// that the branch can be cherry-picked onto main without depending on
// jsaf/fix-delta-encoding landing first.
package grpc

import (
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

// Payload represents one stateful metric batch ready to be shipped over a
// gRPC stream. Carries the wire bytes for the MetricStatefulBatch.data
// field (a serialized + optionally compressed MetricDatumSequence) plus
// the state-changes mutations the receiver will apply on ack.
//
// Compared to the logs equivalent (pkg/logs/message.Payload), this type
// is minimal: no per-message metadata (the series content is opaque
// inside Encoded), no WireDatums (the PoC does not implement the lazy
// snapshot mechanism — see contract.md D6 snapshot semantics and the
// rationale in phase3-proto-proposal.md §9).
type Payload struct {
	// Encoded is the wire-ready bytes of a serialized + compressed
	// MetricDatumSequence. Goes directly into MetricStatefulBatch.data.
	Encoded []byte

	// Encoding is the content encoding string used to compress Encoded
	// (e.g. "zstd", "gzip", "identity"). Set by the encoder/batch
	// strategy; observable for telemetry but not transmitted in the
	// batch envelope (the gRPC channel-level header carries it instead).
	Encoding string

	// UnencodedSize is the size of the raw serialized MetricDatumSequence
	// before compression. Used for compression-ratio telemetry.
	UnencodedSize int

	// PreCompressionBytes is identical to UnencodedSize today, retained
	// as a separate field to mirror the logs Payload shape and for forward
	// compatibility (in case we ever introduce a stage between
	// serialization and compression).
	PreCompressionBytes int

	// PointCount is the number of metric points represented in this
	// payload's MetricSeriesBatch, for the metrics.MetricsSent-style
	// counters reported on ack. Zero for snapshot-only payloads.
	PointCount int

	// StateChanges is the set of dict-define datums newly introduced by
	// this payload. On ack, the inflight tracker walks this slice and
	// applies each define to the per-stream snapshotState. This is how
	// the snapshot accumulates only "definitely-delivered" state — if a
	// payload is dropped, its StateChanges are dropped with it.
	//
	// StateChanges contains ONLY the define datums (Metric*Define) — NOT
	// the MetricSeriesBatch datum itself, which has no snapshot effect.
	StateChanges []*statefulpb.MetricDatum
}
