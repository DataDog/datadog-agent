// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
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

// TracerPayloadsV04 returns the tracer payloads of p in the legacy v04 shape, whichever
// format the agent sent: the legacy TracerPayloads as they are, followed by every idx
// payload converted with IdxTracerPayloadToV04. Tests that assert on what both formats
// carry can use it to run unchanged against either.
func TracerPayloadsV04(p *aggregator.TracePayload) []*pb.TracerPayload {
	out := append([]*pb.TracerPayload(nil), p.TracerPayloads...)
	for _, tp := range p.IdxTracerPayloads {
		out = append(out, IdxTracerPayloadToV04(tp))
	}
	return out
}

// IdxTracerPayloadToV04 converts an idx tracer payload to the legacy v04 shape, resolving
// every string reference:
//   - payload attributes become payload Tags, and chunk attributes chunk Tags, plus
//     _dd.p.dm from the chunk sampling mechanism;
//   - span string and bool attributes become Meta, numeric ones Metrics, and bytes,
//     arrays and maps a JSON string in Meta;
//   - span env, version, component and kind become the Meta keys they were promoted
//     from, and the upper 64 bits of the chunk trace ID become _dd.p.tid.
//
// Span events and links are not converted.
func IdxTracerPayloadToV04(tp *idx.TracerPayload) *pb.TracerPayload {
	strs := tp.GetStrings()
	out := &pb.TracerPayload{
		ContainerID:     IdxStr(strs, tp.GetContainerIDRef()),
		LanguageName:    IdxStr(strs, tp.GetLanguageNameRef()),
		LanguageVersion: IdxStr(strs, tp.GetLanguageVersionRef()),
		TracerVersion:   IdxStr(strs, tp.GetTracerVersionRef()),
		RuntimeID:       IdxStr(strs, tp.GetRuntimeIDRef()),
		Env:             IdxStr(strs, tp.GetEnvRef()),
		Hostname:        IdxStr(strs, tp.GetHostnameRef()),
		AppVersion:      IdxStr(strs, tp.GetAppVersionRef()),
		Tags:            idxAttrsAsTags(strs, tp.GetAttributes()),
	}
	for _, c := range tp.GetChunks() {
		chunk := &pb.TraceChunk{
			Priority:     c.GetPriority(),
			Origin:       IdxStr(strs, c.GetOriginRef()),
			Tags:         idxAttrsAsTags(strs, c.GetAttributes()),
			DroppedTrace: c.GetDroppedTrace(),
		}
		if m := c.GetSamplingMechanism(); m != 0 {
			if chunk.Tags == nil {
				chunk.Tags = map[string]string{}
			}
			chunk.Tags["_dd.p.dm"] = fmt.Sprintf("-%d", m)
		}
		var traceID [16]byte
		if tid := c.GetTraceID(); len(tid) <= 16 {
			copy(traceID[16-len(tid):], tid)
		} else {
			copy(traceID[:], tid[len(tid)-16:])
		}
		for _, s := range c.GetSpans() {
			chunk.Spans = append(chunk.Spans, idxSpanToV04(strs, s, traceID))
		}
		out.Chunks = append(out.Chunks, chunk)
	}
	return out
}

func idxSpanToV04(strs []string, s *idx.Span, traceID [16]byte) *pb.Span {
	span := &pb.Span{
		Service:  IdxStr(strs, s.GetServiceRef()),
		Name:     IdxStr(strs, s.GetNameRef()),
		Resource: IdxStr(strs, s.GetResourceRef()),
		Type:     IdxStr(strs, s.GetTypeRef()),
		TraceID:  binary.BigEndian.Uint64(traceID[8:]),
		SpanID:   s.GetSpanID(),
		ParentID: s.GetParentID(),
		Start:    int64(s.GetStart()),
		Duration: int64(s.GetDuration()),
		Meta:     map[string]string{},
		Metrics:  map[string]float64{},
	}
	if s.GetError() {
		span.Error = 1
	}
	for k, v := range s.GetAttributes() {
		key := IdxStr(strs, k)
		switch val := v.GetValue().(type) {
		case *idx.AnyValue_DoubleValue:
			span.Metrics[key] = val.DoubleValue
		case *idx.AnyValue_IntValue:
			span.Metrics[key] = float64(val.IntValue)
		default:
			span.Meta[key] = idxValueString(strs, v)
		}
	}
	for key, ref := range map[string]uint32{
		"env":       s.GetEnvRef(),
		"version":   s.GetVersionRef(),
		"component": s.GetComponentRef(),
	} {
		if v := IdxStr(strs, ref); v != "" {
			span.Meta[key] = v
		}
	}
	if kind, ok := idxSpanKinds[s.GetKind()]; ok {
		span.Meta["span.kind"] = kind
	}
	if hi := binary.BigEndian.Uint64(traceID[:8]); hi != 0 {
		span.Meta["_dd.p.tid"] = fmt.Sprintf("%016x", hi)
	}
	return span
}

var idxSpanKinds = map[idx.SpanKind]string{
	idx.SpanKind_SPAN_KIND_INTERNAL: "internal",
	idx.SpanKind_SPAN_KIND_SERVER:   "server",
	idx.SpanKind_SPAN_KIND_CLIENT:   "client",
	idx.SpanKind_SPAN_KIND_PRODUCER: "producer",
	idx.SpanKind_SPAN_KIND_CONSUMER: "consumer",
}

func idxAttrsAsTags(strs []string, attrs map[uint32]*idx.AnyValue) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	tags := make(map[string]string, len(attrs))
	for k, v := range attrs {
		tags[IdxStr(strs, k)] = idxValueString(strs, v)
	}
	return tags
}

// idxValueString returns the string form of v: scalars as text, and bytes, arrays and maps
// as JSON.
func idxValueString(strs []string, v *idx.AnyValue) string {
	switch val := v.GetValue().(type) {
	case *idx.AnyValue_StringValueRef:
		return IdxStr(strs, val.StringValueRef)
	case *idx.AnyValue_BoolValue:
		return strconv.FormatBool(val.BoolValue)
	case *idx.AnyValue_DoubleValue:
		return strconv.FormatFloat(val.DoubleValue, 'g', -1, 64)
	case *idx.AnyValue_IntValue:
		return strconv.FormatInt(val.IntValue, 10)
	}
	b, err := json.Marshal(idxValueJSON(strs, v))
	if err != nil {
		return ""
	}
	return string(b)
}

func idxValueJSON(strs []string, v *idx.AnyValue) any {
	switch val := v.GetValue().(type) {
	case *idx.AnyValue_StringValueRef:
		return IdxStr(strs, val.StringValueRef)
	case *idx.AnyValue_BoolValue:
		return val.BoolValue
	case *idx.AnyValue_DoubleValue:
		return val.DoubleValue
	case *idx.AnyValue_IntValue:
		return val.IntValue
	case *idx.AnyValue_BytesValue:
		return val.BytesValue
	case *idx.AnyValue_ArrayValue:
		out := []any{}
		for _, e := range val.ArrayValue.GetValues() {
			out = append(out, idxValueJSON(strs, e))
		}
		return out
	case *idx.AnyValue_KeyValueList:
		out := map[string]any{}
		for _, kv := range val.KeyValueList.GetKeyValues() {
			out[IdxStr(strs, kv.GetKey())] = idxValueJSON(strs, kv.GetValue())
		}
		return out
	}
	return nil
}
