// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"

// Telemetry counters for the stateful metrics path, prefixed
// "serializer.v3_stateful_*". The "lane" label is the destination address; the
// "kind" label is the datum kind (name_define, tag_string_define,
// source_type_name_define, resource_string_define, resource_define,
// origin_define, tagset_define, series_batch).
var (
	tlmStreamsOpened = telemetryimpl.GetCompatComponent().NewCounter(
		"serializer", "v3_stateful_streams_opened",
		[]string{"lane"},
		"Stream creation attempts on the stateful metrics gRPC sender",
	)
	tlmStreamErrors = telemetryimpl.GetCompatComponent().NewCounter(
		"serializer", "v3_stateful_stream_errors",
		[]string{"lane", "reason"},
		"Stream failures on the stateful metrics gRPC sender",
	)
	tlmRotationCount = telemetryimpl.GetCompatComponent().NewCounter(
		"serializer", "v3_stateful_rotation_count",
		[]string{"lane", "reason"},
		"Stream rotations on the stateful metrics gRPC sender",
	)

	tlmBytesSent = telemetryimpl.GetCompatComponent().NewCounter(
		"serializer", "v3_stateful_bytes_sent",
		[]string{"lane"},
		"Compressed bytes sent on the stateful metrics gRPC stream",
	)
	tlmPreCompressionBytesSent = telemetryimpl.GetCompatComponent().NewCounter(
		"serializer", "v3_stateful_pre_compression_bytes_sent",
		[]string{"lane"},
		"Pre-compression bytes serialized on the stateful metrics gRPC stream",
	)

	// v3_stateful_datum_count and v3_stateful_datum_bytes are declared in the
	// encoder (internal/metrics/iterable_series_v3_stateful.go) where they're
	// incremented; declaring them here too would conflict at registration.

	tlmInflightSize = telemetryimpl.GetCompatComponent().NewGauge(
		"serializer", "v3_stateful_inflight_size",
		[]string{"lane"},
		"Current inflight ring size in bytes (compressed) on the stateful metrics gRPC sender",
	)
	tlmDictSize = telemetryimpl.GetCompatComponent().NewGauge(
		"serializer", "v3_stateful_dict_size",
		[]string{"lane", "kind"},
		"Current dict-entry count per kind on the stateful metrics gRPC sender",
	)
)
