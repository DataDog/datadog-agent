// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"errors"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// IterableSeries is a serializer for metrics.IterableSeries
type IterableSeries struct {
	*metrics.IterableSeries
}

// WriteHeader writes the payload header for this type
func (series IterableSeries) WriteHeader(stream *jsoniter.Stream) error {
	return writeHeader(stream)
}

// WriteFooter writes the payload footer for this type
func (series IterableSeries) WriteFooter(stream *jsoniter.Stream) error {
	return writeFooter(stream)
}

// WriteCurrentItem writes the json representation of an item
func (series IterableSeries) WriteCurrentItem(stream *jsoniter.Stream) error {
	current := series.Current()
	if current == nil {
		return errors.New("nil serie")
	}
	return writeItem(stream, current)
}

// DescribeCurrentItem returns a text description for logs
func (series IterableSeries) DescribeCurrentItem() string {
	current := series.Current()
	if current == nil {
		return "nil serie"
	}
	return describeItem(current)
}

// MarshalSplitCompress uses the stream compressor to marshal and compress series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled MetricPayload objects.
func (series IterableSeries) MarshalSplitCompress(bufferContext *marshaler.BufferContext) ([]*[]byte, error) {
	return marshalSplitCompress(series, bufferContext)
}
