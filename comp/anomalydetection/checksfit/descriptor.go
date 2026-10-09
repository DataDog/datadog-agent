// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checksfit carries the DDCHECKS v1 application protocol over the FIT
// shared-memory transport.
//
// The wire format mirrors the Rust `datadog-checks-protocol` crate that the
// isolated anomaly detection process consumes: the application identity is the
// eight bytes `DDCHECKS`, the protocol version is 1, and the record type IDs
// are metric 1, log 2, service check 3, and event 4. Only the metric codec is
// implemented here, because the anomaly detection forwarder sends metrics.
//
// See the contract documents in the checks-protocol crate for the payload
// layout and the FIT setup handshake.
package checksfit

import "github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"

// Record type identifiers from the Checks protocol.
const (
	// TypeMetric is the record type of a scalar metric payload.
	TypeMetric uint32 = 1
	// TypeLog is the record type of a check-produced log payload.
	TypeLog uint32 = 2
	// TypeServiceCheck is the record type of a service-check payload.
	TypeServiceCheck uint32 = 3
	// TypeEvent is the record type of a submitted event payload.
	TypeEvent uint32 = 4
)

// Numeric MetricType values, matching `metric.proto`.
const (
	// MetricTypeUnspecified is an unset metric type.
	MetricTypeUnspecified int32 = 0
	// MetricTypeCounter is a monotonic counter sample.
	MetricTypeCounter int32 = 1
	// MetricTypeRate is a per-second rate sample.
	MetricTypeRate int32 = 2
	// MetricTypeGauge is an instantaneous value.
	MetricTypeGauge int32 = 3
	// MetricTypeHistogram is a histogram sample.
	MetricTypeHistogram int32 = 4
)

// Descriptor identifies the DDCHECKS protocol during setup. Its version must be
// bumped for incompatible codec or type-registry changes, and its identity and
// message types must stay in lockstep with the Rust consumer.
var Descriptor = fitcore.ProtocolDescriptor{
	ID:           id8("DDCHECKS"),
	Version:      1,
	MessageTypes: []uint32{TypeMetric, TypeLog, TypeServiceCheck, TypeEvent},
}

// id8 converts an exact eight-byte string constant to a fixed-size identifier.
func id8(s string) [8]byte {
	var out [8]byte
	copy(out[:], s)
	return out
}
