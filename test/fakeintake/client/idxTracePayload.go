// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"encoding/binary"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// The convert-traces feature is enabled by default, so the agent serializes tracer
// payloads in the v1 string-indexed idx format (AgentPayload.IdxTracerPayloads) and
// leaves the legacy AgentPayload.TracerPayloads field empty. In that format the
// payload, chunk and span metadata is carried as references into a per-payload
// string table, and meta and metrics live in a single attributes map keyed by
// string reference. The accessors below resolve those references so tests can
// assert on indexed payloads without each suite rebuilding the same lookups.

// IdxStr resolves a string-table reference to its value. Reference 0 is the
// empty-string sentinel.
func IdxStr(strings []string, ref uint32) string {
	if ref == 0 || int(ref) >= len(strings) {
		return ""
	}
	return strings[ref]
}

// IdxStrAttr returns the string value of the attribute named key, and whether a
// string-valued attribute with that key was present.
func IdxStrAttr(strings []string, attrs map[uint32]*idx.AnyValue, key string) (string, bool) {
	for k, v := range attrs {
		if IdxStr(strings, k) != key {
			continue
		}
		if sv, ok := v.GetValue().(*idx.AnyValue_StringValueRef); ok {
			return IdxStr(strings, sv.StringValueRef), true
		}
		return "", false
	}
	return "", false
}

// IdxNumAttr returns the numeric value of the attribute named key, and whether a
// numeric-valued attribute with that key was present.
func IdxNumAttr(strings []string, attrs map[uint32]*idx.AnyValue, key string) (float64, bool) {
	for k, v := range attrs {
		if IdxStr(strings, k) != key {
			continue
		}
		switch val := v.GetValue().(type) {
		case *idx.AnyValue_DoubleValue:
			return val.DoubleValue, true
		case *idx.AnyValue_IntValue:
			return float64(val.IntValue), true
		}
		return 0, false
	}
	return 0, false
}

// IdxHasAttr reports whether an attribute named key is present, regardless of type.
func IdxHasAttr(strings []string, attrs map[uint32]*idx.AnyValue, key string) bool {
	for k := range attrs {
		if IdxStr(strings, k) == key {
			return true
		}
	}
	return false
}

// IdxChunkTraceID returns the legacy 64-bit trace ID of a chunk: the trace ID is
// carried once per chunk as a big-endian 128-bit value, and its lowest 8 bytes are
// the legacy ID that used to live on every span. A trace ID shorter than 16 bytes
// is treated as if the missing high-order bytes were zero, matching
// idx.InternalTraceChunk.LegacyTraceID.
func IdxChunkTraceID(chunk *idx.TraceChunk) uint64 {
	tid := chunk.GetTraceID()
	if len(tid) >= 16 {
		return binary.BigEndian.Uint64(tid[8:])
	}
	var buf [16]byte
	copy(buf[16-len(tid):], tid)
	return binary.BigEndian.Uint64(buf[8:])
}

// IdxTracerPayloadHasService reports whether the tracer payload contains at least
// one span for the given service.
func IdxTracerPayloadHasService(tp *idx.TracerPayload, service string) bool {
	for _, chunk := range tp.GetChunks() {
		for _, sp := range chunk.GetSpans() {
			if IdxStr(tp.GetStrings(), sp.GetServiceRef()) == service {
				return true
			}
		}
	}
	return false
}

// IdxPayloadHasService reports whether any tracer payload in the agent payload
// contains a span for the given service.
func IdxPayloadHasService(p *aggregator.TracePayload, service string) bool {
	for _, tp := range p.IdxTracerPayloads {
		if IdxTracerPayloadHasService(tp, service) {
			return true
		}
	}
	return false
}
