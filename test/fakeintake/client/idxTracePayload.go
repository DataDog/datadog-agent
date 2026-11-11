// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"encoding/binary"
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

// LegacyTracerPayloads returns every tracer payload carried by the agent payload in
// the legacy (non-indexed) shape: the payloads in AgentPayload.TracerPayloads as-is,
// followed by those in AgentPayload.IdxTracerPayloads converted with
// IdxToLegacyTracerPayload. Tests that assert on span, chunk or payload metadata can
// use it to accept both formats without duplicating their assertions.
func LegacyTracerPayloads(p *pb.AgentPayload) []*pb.TracerPayload {
	if p == nil {
		return nil
	}
	tps := make([]*pb.TracerPayload, 0, len(p.TracerPayloads)+len(p.IdxTracerPayloads))
	tps = append(tps, p.TracerPayloads...)
	for _, tp := range p.IdxTracerPayloads {
		tps = append(tps, IdxToLegacyTracerPayload(tp))
	}
	return tps
}

// IdxToLegacyTracerPayload converts an indexed tracer payload into the legacy
// tracer payload shape, resolving every string-table reference. It mirrors how the
// agent maps legacy fields onto the indexed format so the result reads like the
// payload the agent would have produced without convert-traces:
//   - string attributes become Meta (span) or Tags (chunk and payload) entries, and
//     double/int attributes become span Metrics; bool attributes become "true"/"false"
//     Meta entries, and other attribute types are dropped;
//   - the promoted span fields env, version, component and span.kind are restored as
//     Meta entries, and the chunk's 128-bit trace ID is split back into the span
//     TraceID (low 64 bits) and the "_dd.p.tid" Meta entry (high 64 bits, hex);
//   - the chunk SamplingMechanism is restored as the "_dd.p.dm" chunk tag.
//
// Promoted values never override an attribute of the same key. Span links, span
// events and meta_struct are not converted.
func IdxToLegacyTracerPayload(tp *idx.TracerPayload) *pb.TracerPayload {
	strs := tp.GetStrings()
	str := func(ref uint32) string { return IdxStr(strs, ref) }
	out := &pb.TracerPayload{
		ContainerID:     str(tp.GetContainerIDRef()),
		LanguageName:    str(tp.GetLanguageNameRef()),
		LanguageVersion: str(tp.GetLanguageVersionRef()),
		TracerVersion:   str(tp.GetTracerVersionRef()),
		RuntimeID:       str(tp.GetRuntimeIDRef()),
		Env:             str(tp.GetEnvRef()),
		Hostname:        str(tp.GetHostnameRef()),
		AppVersion:      str(tp.GetAppVersionRef()),
		Tags:            idxStringAttrs(strs, tp.GetAttributes()),
	}
	for _, chunk := range tp.GetChunks() {
		lc := &pb.TraceChunk{
			Priority:     chunk.GetPriority(),
			Origin:       str(chunk.GetOriginRef()),
			Tags:         idxStringAttrs(strs, chunk.GetAttributes()),
			DroppedTrace: chunk.GetDroppedTrace(),
		}
		if sm := chunk.GetSamplingMechanism(); sm != 0 {
			setIfAbsent(lc.Tags, "_dd.p.dm", "-"+strconv.FormatUint(uint64(sm), 10))
		}
		traceID := IdxChunkTraceID(chunk)
		traceIDUpper := idxChunkTraceIDUpper(chunk)
		for _, sp := range chunk.GetSpans() {
			ls := &pb.Span{
				Service:  str(sp.GetServiceRef()),
				Name:     str(sp.GetNameRef()),
				Resource: str(sp.GetResourceRef()),
				Type:     str(sp.GetTypeRef()),
				TraceID:  traceID,
				SpanID:   sp.GetSpanID(),
				ParentID: sp.GetParentID(),
				Start:    int64(sp.GetStart()),
				Duration: int64(sp.GetDuration()),
				Meta:     map[string]string{},
				Metrics:  map[string]float64{},
			}
			if sp.GetError() {
				ls.Error = 1
			}
			for k, v := range sp.GetAttributes() {
				switch val := v.GetValue().(type) {
				case *idx.AnyValue_StringValueRef:
					ls.Meta[str(k)] = str(val.StringValueRef)
				case *idx.AnyValue_BoolValue:
					ls.Meta[str(k)] = strconv.FormatBool(val.BoolValue)
				case *idx.AnyValue_DoubleValue:
					ls.Metrics[str(k)] = val.DoubleValue
				case *idx.AnyValue_IntValue:
					ls.Metrics[str(k)] = float64(val.IntValue)
				}
			}
			setIfAbsent(ls.Meta, "env", str(sp.GetEnvRef()))
			setIfAbsent(ls.Meta, "version", str(sp.GetVersionRef()))
			setIfAbsent(ls.Meta, "component", str(sp.GetComponentRef()))
			setIfAbsent(ls.Meta, "span.kind", idxSpanKindName(sp.GetKind()))
			if traceIDUpper != 0 {
				setIfAbsent(ls.Meta, "_dd.p.tid", fmt.Sprintf("%016x", traceIDUpper))
			}
			lc.Spans = append(lc.Spans, ls)
		}
		out.Chunks = append(out.Chunks, lc)
	}
	return out
}

// idxStringAttrs returns the string-valued attributes as a map keyed by name.
func idxStringAttrs(strs []string, attrs map[uint32]*idx.AnyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for k, v := range attrs {
		if sv, ok := v.GetValue().(*idx.AnyValue_StringValueRef); ok {
			m[IdxStr(strs, k)] = IdxStr(strs, sv.StringValueRef)
		}
	}
	return m
}

// idxChunkTraceIDUpper returns the high 64 bits of a chunk's 128-bit trace ID.
func idxChunkTraceIDUpper(chunk *idx.TraceChunk) uint64 {
	tid := chunk.GetTraceID()
	if len(tid) < 16 {
		return 0
	}
	return binary.BigEndian.Uint64(tid[:8])
}

// idxSpanKindName returns the legacy "span.kind" value for an indexed span kind, or
// "" when the kind is unspecified.
func idxSpanKindName(kind idx.SpanKind) string {
	switch kind {
	case idx.SpanKind_SPAN_KIND_INTERNAL:
		return "internal"
	case idx.SpanKind_SPAN_KIND_SERVER:
		return "server"
	case idx.SpanKind_SPAN_KIND_CLIENT:
		return "client"
	case idx.SpanKind_SPAN_KIND_PRODUCER:
		return "producer"
	case idx.SpanKind_SPAN_KIND_CONSUMER:
		return "consumer"
	}
	return ""
}

func setIfAbsent(m map[string]string, key, value string) {
	if value == "" {
		return
	}
	if _, ok := m[key]; !ok {
		m[key] = value
	}
}
