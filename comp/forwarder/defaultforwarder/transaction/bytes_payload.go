// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package transaction

import (
	"sync/atomic"
	"time"
)

// CaptureObserver receives initial routing decisions only. Implementations must
// not block delivery and must copy any metadata they retain before returning.
type CaptureObserver interface {
	ObserveRoute(payloadID uint64, ordinals []uint64, endpoint, protocol, destination string, enqueuedAt time.Time)
}

// CaptureMetadata correlates an owned semantic observation with its serialized
// payload. It is local-only: it is never a header, body, or retry-storage field.
// The metadata and its ordinals must remain immutable while attached.
type CaptureMetadata struct {
	SessionID string
	CycleID   uint64
	PayloadID uint64
	Ordinals  []uint64
	Observer  CaptureObserver
}

// BytesPayload is a payload stored as bytes.
// It contains metadata about the payload.
type BytesPayload struct {
	content     []byte
	pointCount  int
	Destination Destination
	capture     atomic.Pointer[CaptureMetadata]
}

// SetCapture attaches local correlation data for the synchronous submission.
// The owner must ClearCapture when that submission returns, so queued retries
// cannot retain the observation's state.
func (p *BytesPayload) SetCapture(metadata *CaptureMetadata) { p.capture.Store(metadata) }

// Capture returns immutable local correlation data, or nil when disarmed.
func (p *BytesPayload) Capture() *CaptureMetadata { return p.capture.Load() }

// ClearCapture detaches local observation state without changing the payload.
func (p *BytesPayload) ClearCapture() { p.capture.Store(nil) }

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
