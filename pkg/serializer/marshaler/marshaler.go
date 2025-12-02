// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshaler

import (
	"bytes"

	jsoniter "github.com/json-iterator/go"
)

// JSONMarshaler is a AbstractMarshaler that implement JSON marshaling.
type JSONMarshaler interface {
	// MarshalJSON serialization a Payload to JSON
	MarshalJSON() ([]byte, error)
}

// AbstractMarshaler is deprecated.
type AbstractMarshaler JSONMarshaler

// StreamJSONMarshaler is an interface for metrics that are able to serialize themselves in a stream
type StreamJSONMarshaler interface {
	WriteHeader(*jsoniter.Stream) error
	WriteFooter(*jsoniter.Stream) error
	WriteItem(*jsoniter.Stream, int) error
	Len() int
	DescribeItem(i int) string
}

// IterableStreamJSONMarshaler is an interface for iterable metrics that are able to
// serialize themselves in a stream.
// Expected usage:
//
//		m.WriteHeader(stream)
//		for m.MoveNext() {
//			m.WriteCurrentItem(stream)
//	 }
//		m.WriteFooter(stream)
type IterableStreamJSONMarshaler interface {
	WriteHeader(*jsoniter.Stream) error
	WriteFooter(*jsoniter.Stream) error
	WriteCurrentItem(*jsoniter.Stream) error
	DescribeCurrentItem() string
	MoveNext() bool
	GetCurrentItemPointCount() int
}

// BufferContext contains the buffers used for MarshalSplitCompress so they can be shared between invocations
type BufferContext struct {
	CompressorInput   *bytes.Buffer
	CompressorOutput  *bytes.Buffer
	PrecompressionBuf *bytes.Buffer
}

// NewBufferContext initialize the default compression buffers
func NewBufferContext() *BufferContext {
	return &BufferContext{
		bytes.NewBuffer(make([]byte, 0, 1024)),
		bytes.NewBuffer(make([]byte, 0, 1024)),
		bytes.NewBuffer(make([]byte, 0, 1024)),
	}
}
