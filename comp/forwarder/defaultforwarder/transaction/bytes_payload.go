// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package transaction

// BytesPayload is a payload stored as bytes.
// It contains metadata about the payload.
type BytesPayload struct {
	content     []byte
	pointCount  int
	Destination Destination
}

// NewBytesPayload creates a new instance of BytesPayload.
func NewBytesPayload(payload []byte, pointCount int) *BytesPayload {
	return &BytesPayload{
		content:    payload,
		pointCount: pointCount,
	}
}

// NewBytesPayloadWithoutMetaData creates a new instance of BytesPayload without metadata.
func NewBytesPayloadWithoutMetaData(payload []byte) *BytesPayload {
	return &BytesPayload{content: payload}
}

// Len returns the length as bytes of the payload
func (p *BytesPayload) Len() int {
	return len(p.content)
}

// GetContent returns the content of the payload
func (p *BytesPayload) GetContent() []byte {
	return p.content
}

// GetPointCount returns the number of points in this payload
func (p *BytesPayload) GetPointCount() int {
	return p.pointCount
}

// BytesPayloads is a collection of BytesPayload
type BytesPayloads []*BytesPayload

// NewBytesPayloadsWithoutMetaData creates BytesPayloads without metadata.
func NewBytesPayloadsWithoutMetaData(payloads []*[]byte) BytesPayloads {
	var bytesPayloads BytesPayloads
	for _, payload := range payloads {
		if payload != nil {
			bytesPayloads = append(bytesPayloads, NewBytesPayloadWithoutMetaData(*payload))
		}
	}
	return bytesPayloads
}
