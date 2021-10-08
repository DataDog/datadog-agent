// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshaler

import (
	"bytes"

	jsoniter "github.com/json-iterator/go"
)

// Marshaler is an interface for metrics that are able to serialize themselves to JSON and protobuf
type Marshaler interface {
	MarshalJSON() ([]byte, error)
	Marshal() ([]byte, error)
	SplitPayload(int) ([]Marshaler, error)
	MarshalSplitCompress(*BufferContext) ([]*[]byte, error)
}

// StreamJSONMarshaler is an interface for metrics that are able to serialize themselves in a stream
type StreamJSONMarshaler interface {
	Marshaler
	WriteHeader(*jsoniter.Stream) error
	WriteFooter(*jsoniter.Stream) error
	WriteItem(*jsoniter.Stream, int) error
	Len() int
	DescribeItem(i int) string
}

// BufferContext contains the buffers used for MarshalSplitCompress so they can be shared between invocations
type BufferContext struct {
	CompressorInput   *bytes.Buffer
	CompressorOutput  *bytes.Buffer
	PrecompressionBuf *bytes.Buffer
}

// DefaultBufferContext initialize the default compression buffers
func DefaultBufferContext() *BufferContext {
	return &BufferContext{
		bytes.NewBuffer(make([]byte, 0, 1024)),
		bytes.NewBuffer(make([]byte, 0, 1024)),
		bytes.NewBuffer(make([]byte, 0, 1024)),
	}
}
