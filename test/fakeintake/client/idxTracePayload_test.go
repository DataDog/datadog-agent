// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

func strRef(ref uint32) *idx.AnyValue {
	return &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: ref}}
}

func TestIdxStr(t *testing.T) {
	strings := []string{"", "service", "env"}
	assert.Equal(t, "service", IdxStr(strings, 1))
	// Reference 0 is the empty-string sentinel.
	assert.Equal(t, "", IdxStr(strings, 0))
	// An out-of-range reference must not panic.
	assert.Equal(t, "", IdxStr(strings, 42))
}

func TestIdxAttrs(t *testing.T) {
	strings := []string{"", "http.method", "GET", "_sampling_priority_v1", "retries"}
	attrs := map[uint32]*idx.AnyValue{
		1: strRef(2),
		3: {Value: &idx.AnyValue_DoubleValue{DoubleValue: 1}},
		4: {Value: &idx.AnyValue_IntValue{IntValue: 3}},
	}

	v, ok := IdxStrAttr(strings, attrs, "http.method")
	assert.True(t, ok)
	assert.Equal(t, "GET", v)

	_, ok = IdxStrAttr(strings, attrs, "missing")
	assert.False(t, ok)

	// A numeric attribute is not a string attribute.
	_, ok = IdxStrAttr(strings, attrs, "retries")
	assert.False(t, ok)

	n, ok := IdxNumAttr(strings, attrs, "_sampling_priority_v1")
	assert.True(t, ok)
	assert.Equal(t, float64(1), n)

	n, ok = IdxNumAttr(strings, attrs, "retries")
	assert.True(t, ok)
	assert.Equal(t, float64(3), n)

	_, ok = IdxNumAttr(strings, attrs, "http.method")
	assert.False(t, ok)

	assert.True(t, IdxHasAttr(strings, attrs, "http.method"))
	assert.True(t, IdxHasAttr(strings, attrs, "retries"))
	assert.False(t, IdxHasAttr(strings, attrs, "missing"))
}

func TestIdxChunkTraceID(t *testing.T) {
	full := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
		0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
	}
	assert.Equal(t, uint64(0x8899aabbccddeeff), IdxChunkTraceID(&idx.TraceChunk{TraceID: full}))

	// A trace ID may be serialized with its leading zero bytes trimmed; the
	// bytes present are the least-significant ones.
	assert.Equal(t, uint64(0xAF), IdxChunkTraceID(&idx.TraceChunk{TraceID: []byte{0xAF}}))
	assert.Equal(t, uint64(0x0102030405060708), IdxChunkTraceID(&idx.TraceChunk{
		TraceID: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
	}))
	assert.Equal(t, uint64(0), IdxChunkTraceID(&idx.TraceChunk{}))
}

func TestIdxPayloadHasService(t *testing.T) {
	payload := &aggregator.TracePayload{AgentPayload: &pb.AgentPayload{
		IdxTracerPayloads: []*idx.TracerPayload{{
			Strings: []string{"", "tracegen", "other"},
			Chunks: []*idx.TraceChunk{{
				Spans: []*idx.Span{{ServiceRef: 1}},
			}},
		}},
	}}

	assert.True(t, IdxPayloadHasService(payload, "tracegen"))
	assert.False(t, IdxPayloadHasService(payload, "other"))
	// A payload carrying only the legacy tracer payloads has no idx service.
	assert.False(t, IdxPayloadHasService(&aggregator.TracePayload{AgentPayload: &pb.AgentPayload{
		TracerPayloads: []*pb.TracerPayload{{Chunks: []*pb.TraceChunk{{
			Spans: []*pb.Span{{Service: "tracegen"}},
		}}}},
	}}, "tracegen"))
}
