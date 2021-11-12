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
	JSONMarshaler
	ProtoMarshaler
}

// JSONMarshaler is a AbstractMarshaler that implement JSON marshaling.
type JSONMarshaler interface {
	AbstractMarshaler

	// MarshalJSON serialization a Payload to JSON
	MarshalJSON() ([]byte, error)
}

// ProtoMarshaler is a AbstractMarshaler that implement proto marshaling.
type ProtoMarshaler interface {
	AbstractMarshaler

	// Marshal serialize objects using agent-payload definition.
	Marshal() ([]byte, error)
}

// AbstractMarshaler is an abstract marshaler.
type AbstractMarshaler interface {
	// SplitPayload breaks the payload into times number of pieces
	SplitPayload(int) ([]AbstractMarshaler, error)

	// MarshalSplitCompress uses the stream compressor to marshal and compress payloads.
	MarshalSplitCompress(*BufferContext) ([]*[]byte, error)
}

// StreamJSONMarshaler is an interface for metrics that are able to serialize themselves in a stream
type StreamJSONMarshaler interface {
	JSONMarshaler
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
