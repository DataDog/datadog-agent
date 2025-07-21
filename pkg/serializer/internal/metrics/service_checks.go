// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"errors"
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	utiljson "github.com/DataDog/datadog-agent/pkg/util/json"
)

// ServiceChecks represents a list of service checks ready to be serialize
type ServiceChecks []*servicecheck.ServiceCheck

//// The following methods implement the StreamJSONMarshaler interface,

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

func writeServiceCheck(sc *servicecheck.ServiceCheck, stream *jsoniter.Stream) error {
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
