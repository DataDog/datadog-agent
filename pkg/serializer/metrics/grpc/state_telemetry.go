// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import "github.com/DataDog/datadog-agent/pkg/telemetry"

// Telemetry counters for the stateful metrics path. Names follow the
// "serializer.v3_stateful_*" prefix per contract.md D10 and the
// phase3-proto-proposal.md §7 list.
//
// kind label values:
//   name_define, tag_string_define, source_type_name_define,
//   resource_string_define, resource_define, origin_define,
//   tagset_define, series_batch
//
// lane label values: numeric ("0", "1", ...). Always "0" at PoC N=1.
//
// reason label values for stream errors / rotation:
//   stream_creation_failed, recv_error_*, send_err_*, server_eof,
//   transport_error, batch_id_mismatch, received_ack_but_no_sent_payloads,
//   irrecoverable_error  (mirrored from logs grpc sender; the exact set
//   may diverge slightly as the metric path is implemented).

var (
	tlmStreamsOpened = telemetry.NewCounter(
		"serializer", "v3_stateful_streams_opened",
		[]string{"lane"},
		"Stream creation attempts on the stateful metrics gRPC sender",
	)
	tlmStreamErrors = telemetry.NewCounter(
		"serializer", "v3_stateful_stream_errors",
		[]string{"lane", "reason"},
		"Stream failures on the stateful metrics gRPC sender",
	)
	tlmRotationCount = telemetry.NewCounter(
		"serializer", "v3_stateful_rotation_count",
		[]string{"lane", "reason"},
		"Stream rotations on the stateful metrics gRPC sender",
	)

	tlmBytesSent = telemetry.NewCounter(
		"serializer", "v3_stateful_bytes_sent",
		[]string{"lane"},
		"Compressed bytes sent on the stateful metrics gRPC stream",
	)
	tlmPreCompressionBytesSent = telemetry.NewCounter(
		"serializer", "v3_stateful_pre_compression_bytes_sent",
		[]string{"lane"},
		"Pre-compression bytes serialized on the stateful metrics gRPC stream",
	)

	// Note: v3_stateful_datum_count and v3_stateful_datum_bytes are declared
	// in pkg/serializer/internal/metrics/iterable_series_v3_stateful.go,
	// where they're actually incremented (in the encoder's submit method).
	// Declaring them here as duplicates would cause a registration conflict.

	tlmInflightSize = telemetry.NewGauge(
		"serializer", "v3_stateful_inflight_size",
		[]string{"lane"},
		"Current inflight ring size in bytes (compressed) on the stateful metrics gRPC sender",
	)
	tlmDictSize = telemetry.NewGauge(
		"serializer", "v3_stateful_dict_size",
		[]string{"lane", "kind"},
		"Current dict-entry count per kind on the stateful metrics gRPC sender",
	)
)
