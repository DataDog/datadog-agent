// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	utiljson "github.com/DataDog/datadog-agent/pkg/util/json"
)

var (
	serviceCheckExpvar = expvar.NewMap("ServiceCheck")

	tlmServiceCheck = telemetry.NewCounter("metrics", "service_check_split",
		[]string{"action"}, "Service check split")
)

// ServiceChecks represents a list of service checks ready to be serialize
type ServiceChecks []*metrics.ServiceCheck

// MarshalJSON serializes service checks to JSON so it can be sent to V1 endpoints
// FIXME(olivier): to be removed when v2 endpoints are available
func (sc ServiceChecks) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type ServiceChecksAlias ServiceChecks

	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(ServiceChecksAlias(sc))
	return reqBody.Bytes(), err
}

// SplitPayload breaks the payload into times number of pieces
func (sc ServiceChecks) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	serviceCheckExpvar.Add("TimesSplit", 1)
	tlmServiceCheck.Inc("times_split")
	// only split it up as much as possible
	if len(sc) < times {
		serviceCheckExpvar.Add("ServiceChecksShorter", 1)
		tlmServiceCheck.Inc("shorter")
		times = len(sc)
	}
	splitPayloads := make([]marshaler.AbstractMarshaler, times)
	batchSize := len(sc) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// the batch size will not be perfect, only split it as much as possible
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(sc)
		}
		newSC := sc[n:end]
		splitPayloads[i] = newSC
		n += batchSize
	}
	return splitPayloads, nil
}

//// The following methods implement the StreamJSONMarshaler interface
//// for support of the enable_service_checks_stream_payload_serialization option.

// WriteHeader writes the payload header for this type
func (sc ServiceChecks) WriteHeader(stream *jsoniter.Stream) error {
	stream.WriteArrayStart()
	return stream.Flush()
}

// WriteFooter writes the payload footer for this type
func (sc ServiceChecks) WriteFooter(stream *jsoniter.Stream) error {
	stream.WriteArrayEnd()
	return stream.Flush()
}

// WriteItem writes the json representation of an item
func (sc ServiceChecks) WriteItem(stream *jsoniter.Stream, i int) error {
	if i < 0 || i > len(sc)-1 {
		return errors.New("out of range")
	}

	if err := writeServiceCheck(sc[i], stream); err != nil {
		return err
	}
	return stream.Flush()
}

// Len returns the number of items to marshal
func (sc ServiceChecks) Len() int {
	return len(sc)
}

// DescribeItem returns a text description for logs
func (sc ServiceChecks) DescribeItem(i int) string {
	if i < 0 || i > len(sc)-1 {
		return "out of range"
	}
	return fmt.Sprintf("CheckName:%q, Message:%q", sc[i].CheckName, sc[i].Message)
}

func writeServiceCheck(sc *metrics.ServiceCheck, stream *jsoniter.Stream) error {
	writer := utiljson.NewRawObjectWriter(stream)

	if err := writer.StartObject(); err != nil {
		return err
	}
	writer.AddStringField("check", sc.CheckName, utiljson.AllowEmpty)
	writer.AddStringField("host_name", sc.Host, utiljson.AllowEmpty)
	writer.AddInt64Field("timestamp", sc.Ts)
	writer.AddInt64Field("status", int64(sc.Status))
	writer.AddStringField("message", sc.Message, utiljson.AllowEmpty)

	tagsField := "tags"

	if len(sc.Tags) == 0 {
		stream.WriteMore()
		stream.WriteObjectField(tagsField)
		stream.WriteNil()
	} else {
		if err := writer.StartArrayField(tagsField); err != nil {
			return err
		}
		for _, tag := range sc.Tags {
			writer.AddStringValue(tag)
		}
		if err := writer.FinishArrayField(); err != nil {
			return err
		}
	}
	if err := writer.FinishObject(); err != nil {
		return err
	}
	return writer.Flush()
}
