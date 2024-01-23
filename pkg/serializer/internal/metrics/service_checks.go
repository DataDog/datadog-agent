// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"expvar"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	serviceCheckExpvar = expvar.NewMap("ServiceCheck")

	tlmServiceCheck = telemetry.NewCounter("metrics", "service_check_split",
		[]string{"action"}, "Service check split")
)

// ServiceChecks represents a list of service checks ready to be serialize
type ServiceChecks []*servicecheck.ServiceCheck

// MarshalJSON serializes service checks to JSON so it can be sent to V1 endpoints
// FIXME(olivier): to be removed when v2 endpoints are available
func (sc ServiceChecks) MarshalJSON() ([]byte, error) {
	panic("not called")
}

// SplitPayload breaks the payload into times number of pieces
func (sc ServiceChecks) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	panic("not called")
}

//// The following methods implement the StreamJSONMarshaler interface
//// for support of the enable_service_checks_stream_payload_serialization option.

// WriteHeader writes the payload header for this type
func (sc ServiceChecks) WriteHeader(stream *jsoniter.Stream) error {
	panic("not called")
}

// WriteFooter writes the payload footer for this type
func (sc ServiceChecks) WriteFooter(stream *jsoniter.Stream) error {
	panic("not called")
}

// WriteItem writes the json representation of an item
func (sc ServiceChecks) WriteItem(stream *jsoniter.Stream, i int) error {
	panic("not called")
}

// Len returns the number of items to marshal
func (sc ServiceChecks) Len() int {
	panic("not called")
}

// DescribeItem returns a text description for logs
func (sc ServiceChecks) DescribeItem(i int) string {
	panic("not called")
}

func writeServiceCheck(sc *servicecheck.ServiceCheck, stream *jsoniter.Stream) error {
	panic("not called")
}
