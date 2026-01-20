// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"encoding/binary"
	"math"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// Protobuf wire types
const (
	wireVarint      = 0
	wireFixed64     = 1
	wireLengthDelim = 2
)

// Field numbers for AgentPayload (from agent_payload.proto)
const (
	fieldHostName           = 1  // string
	fieldEnv                = 2  // string
	fieldTracerPayloads     = 5  // repeated TracerPayload (ignored for now)
	fieldTags               = 6  // map<string, string>
	fieldAgentVersion       = 7  // string
	fieldTargetTPS          = 8  // double (fixed64)
	fieldErrorTPS           = 9  // double (fixed64)
	fieldRareSamplerEnabled = 10 // bool
	fieldIdxTracerPayloads  = 11 // repeated idx.TracerPayload
)

// Field numbers for idx.TracerPayload (from idx/tracer_payload.proto)
const (
	fieldTPStrings            = 1  // repeated string
	fieldTPContainerIDRef     = 2  // uint32
	fieldTPLanguageNameRef    = 3  // uint32
	fieldTPLanguageVersionRef = 4  // uint32
	fieldTPTracerVersionRef   = 5  // uint32
	fieldTPRuntimeIDRef       = 6  // uint32
	fieldTPEnvRef             = 7  // uint32
	fieldTPHostnameRef        = 8  // uint32
	fieldTPAppVersionRef      = 9  // uint32
	fieldTPAttributes         = 10 // map<uint32, AnyValue>
	fieldTPChunks             = 11 // repeated TraceChunk
)

// Field numbers for idx.TraceChunk (from idx/tracer_payload.proto)
const (
	fieldTCPriority          = 1 // int32
	fieldTCOriginRef         = 2 // uint32
	fieldTCAttributes        = 3 // map<uint32, AnyValue>
	fieldTCSpans             = 4 // repeated Span
	fieldTCDroppedTrace      = 5 // bool
	fieldTCTraceID           = 6 // bytes
	fieldTCSamplingMechanism = 7 // uint32
)

// Field numbers for idx.Span (from idx/span.proto)
const (
	fieldSpanServiceRef   = 1  // uint32
	fieldSpanNameRef      = 2  // uint32
	fieldSpanResourceRef  = 3  // uint32
	fieldSpanSpanID       = 4  // fixed64
	fieldSpanParentID     = 5  // uint64 (varint)
	fieldSpanStart        = 6  // fixed64
	fieldSpanDuration     = 7  // uint64 (varint)
	fieldSpanError        = 8  // bool
	fieldSpanAttributes   = 9  // map<uint32, AnyValue>
	fieldSpanTypeRef      = 10 // uint32
	fieldSpanLinks        = 11 // repeated SpanLink
	fieldSpanEvents       = 12 // repeated SpanEvent
	fieldSpanEnvRef       = 13 // uint32
	fieldSpanVersionRef   = 14 // uint32
	fieldSpanComponentRef = 15 // uint32
	fieldSpanKind         = 16 // SpanKind enum (varint)
)

// Field numbers for idx.SpanLink (from idx/span.proto)
const (
	fieldSpanLinkTraceID       = 1 // bytes
	fieldSpanLinkSpanID        = 2 // fixed64
	fieldSpanLinkAttributes    = 3 // map<uint32, AnyValue>
	fieldSpanLinkTracestateRef = 4 // uint32
	fieldSpanLinkFlags         = 5 // uint32
)

// Field numbers for idx.SpanEvent (from idx/span.proto)
const (
	fieldSpanEventTime       = 1 // fixed64
	fieldSpanEventNameRef    = 2 // uint32
	fieldSpanEventAttributes = 3 // map<uint32, AnyValue>
)

// Field numbers for idx.AnyValue (from idx/span.proto)
const (
	fieldAnyValueStringValueRef = 1 // uint32 (oneof)
	fieldAnyValueBoolValue      = 2 // bool (oneof)
	fieldAnyValueDoubleValue    = 3 // double (oneof)
	fieldAnyValueIntValue       = 4 // int64 (oneof)
	fieldAnyValueBytesValue     = 5 // bytes (oneof)
	fieldAnyValueArrayValue     = 6 // ArrayValue (oneof)
	fieldAnyValueKeyValueList   = 7 // KeyValueList (oneof)
)

// Field numbers for idx.ArrayValue (from idx/span.proto)
const (
	fieldArrayValueValues = 1 // repeated AnyValue
)

// Field numbers for idx.KeyValueList (from idx/span.proto)
const (
	fieldKeyValueListKeyValues = 1 // repeated KeyValue
)

// Field numbers for idx.KeyValue (from idx/span.proto)
const (
	fieldKeyValueKey   = 1 // uint32
	fieldKeyValueValue = 2 // AnyValue
)

// notAssigned is a sentinel value indicating a string ref has not been assigned a new index
const notAssigned = ^uint32(0) // math.MaxUint32

// stringRefMapper tracks used string references and builds a compact mapping
type stringRefMapper struct {
	// oldToNew maps old string indices to new compact indices
	// A value of notAssigned means the index has not been seen/assigned yet
	oldToNew []uint32
	// newStrings is the compacted string table
	newStrings []string
	// nextIndex is the next index to assign
	nextIndex uint32
}

// newStringRefMapper creates a new string reference mapper for a given string table size
func newStringRefMapper(stringTableSize int) *stringRefMapper {
	oldToNew := make([]uint32, stringTableSize)
	// Initialize with sentinel value to indicate "not assigned"
	for i := range oldToNew {
		oldToNew[i] = notAssigned
	}
	return &stringRefMapper{
		oldToNew:   oldToNew,
		newStrings: make([]string, 0, stringTableSize),
		nextIndex:  0,
	}
}

// markRef marks a string reference as used and assigns it a new index if not already assigned
func (m *stringRefMapper) markRef(oldRef uint32) {
	if int(oldRef) >= len(m.oldToNew) {
		return // Invalid ref, ignore
	}
	if m.oldToNew[oldRef] == notAssigned {
		m.oldToNew[oldRef] = m.nextIndex
		m.nextIndex++
	}
}

// remap returns the new index for an old reference
func (m *stringRefMapper) remap(oldRef uint32) uint32 {
	if int(oldRef) >= len(m.oldToNew) {
		return 0 // Invalid ref, return 0
	}
	return m.oldToNew[oldRef]
}

// buildCompactStrings builds the compacted string table from the original strings
func (m *stringRefMapper) buildCompactStrings(originalStrings []string) []string {
	m.newStrings = make([]string, m.nextIndex)
	for oldIdx, newIdx := range m.oldToNew {
		if newIdx != notAssigned && oldIdx < len(originalStrings) {
			m.newStrings[newIdx] = originalStrings[oldIdx]
		}
	}
	return m.newStrings
}

// collectStringRefs walks through a TracerPayload and marks all used string references
func collectStringRefs(tp *idx.TracerPayload, mapper *stringRefMapper) {
	if tp == nil {
		return
	}

	// TracerPayload level refs
	mapper.markRef(tp.ContainerIDRef)
	mapper.markRef(tp.LanguageNameRef)
	mapper.markRef(tp.LanguageVersionRef)
	mapper.markRef(tp.TracerVersionRef)
	mapper.markRef(tp.RuntimeIDRef)
	mapper.markRef(tp.EnvRef)
	mapper.markRef(tp.HostnameRef)
	mapper.markRef(tp.AppVersionRef)

	// TracerPayload attributes
	collectAttributeRefs(tp.Attributes, mapper)

	// Chunks
	for _, chunk := range tp.Chunks {
		collectChunkRefs(chunk, mapper)
	}
}

// collectChunkRefs collects string refs from a TraceChunk
func collectChunkRefs(chunk *idx.TraceChunk, mapper *stringRefMapper) {
	if chunk == nil {
		return
	}

	mapper.markRef(chunk.OriginRef)
	collectAttributeRefs(chunk.Attributes, mapper)

	for _, span := range chunk.Spans {
		collectSpanRefs(span, mapper)
	}
}

// collectSpanRefs collects string refs from a Span
func collectSpanRefs(span *idx.Span, mapper *stringRefMapper) {
	if span == nil {
		return
	}

	mapper.markRef(span.ServiceRef)
	mapper.markRef(span.NameRef)
	mapper.markRef(span.ResourceRef)
	mapper.markRef(span.TypeRef)
	mapper.markRef(span.EnvRef)
	mapper.markRef(span.VersionRef)
	mapper.markRef(span.ComponentRef)

	collectAttributeRefs(span.Attributes, mapper)

	for _, link := range span.Links {
		collectSpanLinkRefs(link, mapper)
	}

	for _, event := range span.Events {
		collectSpanEventRefs(event, mapper)
	}
}

// collectSpanLinkRefs collects string refs from a SpanLink
func collectSpanLinkRefs(link *idx.SpanLink, mapper *stringRefMapper) {
	if link == nil {
		return
	}

	mapper.markRef(link.TracestateRef)
	collectAttributeRefs(link.Attributes, mapper)
}

// collectSpanEventRefs collects string refs from a SpanEvent
func collectSpanEventRefs(event *idx.SpanEvent, mapper *stringRefMapper) {
	if event == nil {
		return
	}

	mapper.markRef(event.NameRef)
	collectAttributeRefs(event.Attributes, mapper)
}

// collectAttributeRefs collects string refs from an attributes map
func collectAttributeRefs(attrs map[uint32]*idx.AnyValue, mapper *stringRefMapper) {
	for keyRef, value := range attrs {
		mapper.markRef(keyRef)
		collectAnyValueRefs(value, mapper)
	}
}

// collectAnyValueRefs collects string refs from an AnyValue
func collectAnyValueRefs(av *idx.AnyValue, mapper *stringRefMapper) {
	if av == nil {
		return
	}

	switch v := av.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		mapper.markRef(v.StringValueRef)
	case *idx.AnyValue_ArrayValue:
		if v.ArrayValue != nil {
			for _, elem := range v.ArrayValue.Values {
				collectAnyValueRefs(elem, mapper)
			}
		}
	case *idx.AnyValue_KeyValueList:
		if v.KeyValueList != nil {
			for _, kv := range v.KeyValueList.KeyValues {
				if kv != nil {
					mapper.markRef(kv.Key)
					collectAnyValueRefs(kv.Value, mapper)
				}
			}
		}
	}
}

// MarshalAgentPayload serializes an AgentPayload to protobuf binary format.
// This is a custom serializer that ignores the TracerPayloads field and only
// serializes the IdxTracerPayloads field along with the other fields.
// It also compacts the string table by removing unused strings and remapping references.
func MarshalAgentPayload(ap *AgentPayload) ([]byte, error) {
	if ap == nil {
		return nil, nil
	}
	// We can't pre-calculate exact size due to string compaction, so we estimate
	buf := make([]byte, 0, 4096)
	return AppendAgentPayload(buf, ap)
}

// SizeAgentPayload calculates the size of the serialized AgentPayload.
func SizeAgentPayload(ap *AgentPayload) int {
	if ap == nil {
		return 0
	}
	size := 0

	// Field 1: hostName (string)
	if len(ap.HostName) > 0 {
		size += sizeTag(fieldHostName, wireLengthDelim)
		size += sizeVarint(uint64(len(ap.HostName)))
		size += len(ap.HostName)
	}

	// Field 2: env (string)
	if len(ap.Env) > 0 {
		size += sizeTag(fieldEnv, wireLengthDelim)
		size += sizeVarint(uint64(len(ap.Env)))
		size += len(ap.Env)
	}

	// Field 5: tracerPayloads - IGNORED

	// Field 6: tags (map<string, string>)
	for k, v := range ap.Tags {
		mapEntrySize := sizeMapEntry(k, v)
		size += sizeTag(fieldTags, wireLengthDelim)
		size += sizeVarint(uint64(mapEntrySize))
		size += mapEntrySize
	}

	// Field 7: agentVersion (string)
	if len(ap.AgentVersion) > 0 {
		size += sizeTag(fieldAgentVersion, wireLengthDelim)
		size += sizeVarint(uint64(len(ap.AgentVersion)))
		size += len(ap.AgentVersion)
	}

	// Field 8: targetTPS (double/fixed64)
	if ap.TargetTPS != 0 {
		size += sizeTag(fieldTargetTPS, wireFixed64)
		size += 8
	}

	// Field 9: errorTPS (double/fixed64)
	if ap.ErrorTPS != 0 {
		size += sizeTag(fieldErrorTPS, wireFixed64)
		size += 8
	}

	// Field 10: rareSamplerEnabled (bool)
	if ap.RareSamplerEnabled {
		size += sizeTag(fieldRareSamplerEnabled, wireVarint)
		size++ // bool is always 1 byte when true
	}

	// Field 11: idxTracerPayloads (repeated idx.TracerPayload)
	for _, tp := range ap.IdxTracerPayloads {
		payloadSize := tp.SizeVT()
		size += sizeTag(fieldIdxTracerPayloads, wireLengthDelim)
		size += sizeVarint(uint64(payloadSize))
		size += payloadSize
	}

	return size
}

// AppendAgentPayload appends the serialized AgentPayload to the given buffer.
func AppendAgentPayload(buf []byte, ap *AgentPayload) ([]byte, error) {
	if ap == nil {
		return buf, nil
	}

	// Field 1: hostName (string)
	if len(ap.HostName) > 0 {
		buf = appendTag(buf, fieldHostName, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(ap.HostName)))
		buf = append(buf, ap.HostName...)
	}

	// Field 2: env (string)
	if len(ap.Env) > 0 {
		buf = appendTag(buf, fieldEnv, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(ap.Env)))
		buf = append(buf, ap.Env...)
	}

	// Field 5: tracerPayloads - IGNORED

	// Field 6: tags (map<string, string>)
	// Sort keys for deterministic output
	if len(ap.Tags) > 0 {
		keys := make([]string, 0, len(ap.Tags))
		for k := range ap.Tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := ap.Tags[k]
			buf = appendTag(buf, fieldTags, wireLengthDelim)
			mapEntrySize := sizeMapEntry(k, v)
			buf = appendVarint(buf, uint64(mapEntrySize))
			buf = appendMapEntry(buf, k, v)
		}
	}

	// Field 7: agentVersion (string)
	if len(ap.AgentVersion) > 0 {
		buf = appendTag(buf, fieldAgentVersion, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(ap.AgentVersion)))
		buf = append(buf, ap.AgentVersion...)
	}

	// Field 8: targetTPS (double/fixed64)
	if ap.TargetTPS != 0 {
		buf = appendTag(buf, fieldTargetTPS, wireFixed64)
		buf = appendFixed64(buf, math.Float64bits(ap.TargetTPS))
	}

	// Field 9: errorTPS (double/fixed64)
	if ap.ErrorTPS != 0 {
		buf = appendTag(buf, fieldErrorTPS, wireFixed64)
		buf = appendFixed64(buf, math.Float64bits(ap.ErrorTPS))
	}

	// Field 10: rareSamplerEnabled (bool)
	if ap.RareSamplerEnabled {
		buf = appendTag(buf, fieldRareSamplerEnabled, wireVarint)
		buf = append(buf, 1)
	}

	// Field 11: idxTracerPayloads (repeated idx.TracerPayload)
	for _, tp := range ap.IdxTracerPayloads {
		buf = appendIdxTracerPayload(buf, tp)
	}

	return buf, nil
}

// appendIdxTracerPayload serializes an idx.TracerPayload and appends it to the buffer.
// It compacts the string table by removing unused strings and remapping all references.
func appendIdxTracerPayload(buf []byte, tp *idx.TracerPayload) []byte {
	if tp == nil {
		return buf
	}

	// Step 1: Create mapper and collect all used string refs
	mapper := newStringRefMapper(len(tp.Strings))
	collectStringRefs(tp, mapper)

	// Step 2: Build compact string table
	compactStrings := mapper.buildCompactStrings(tp.Strings)

	// Step 3: Calculate exact size needed for the TracerPayload
	payloadSize := sizeTracerPayloadWithMapper(tp, compactStrings, mapper)

	// Step 4: Write tag + length prefix, then serialize directly
	buf = appendTag(buf, fieldIdxTracerPayloads, wireLengthDelim)
	buf = appendVarint(buf, uint64(payloadSize))

	// Ensure buffer has enough capacity
	if cap(buf)-len(buf) < payloadSize {
		newBuf := make([]byte, len(buf), len(buf)+payloadSize)
		copy(newBuf, buf)
		buf = newBuf
	}

	buf = appendTracerPayloadWithMapper(buf, tp, compactStrings, mapper)
	return buf
}

// =============================================================================
// Size calculation functions (for pre-allocation optimization)
// =============================================================================

// sizeTracerPayloadWithMapper calculates the serialized size of a TracerPayload
func sizeTracerPayloadWithMapper(tp *idx.TracerPayload, compactStrings []string, mapper *stringRefMapper) int {
	size := 0

	// Field 1: strings (repeated string)
	for _, s := range compactStrings {
		size += sizeTag(fieldTPStrings, wireLengthDelim)
		size += sizeVarint(uint64(len(s)))
		size += len(s)
	}

	// Field 2-9: various refs
	if tp.ContainerIDRef != 0 {
		size += sizeTag(fieldTPContainerIDRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.ContainerIDRef)))
	}
	if tp.LanguageNameRef != 0 {
		size += sizeTag(fieldTPLanguageNameRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.LanguageNameRef)))
	}
	if tp.LanguageVersionRef != 0 {
		size += sizeTag(fieldTPLanguageVersionRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.LanguageVersionRef)))
	}
	if tp.TracerVersionRef != 0 {
		size += sizeTag(fieldTPTracerVersionRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.TracerVersionRef)))
	}
	if tp.RuntimeIDRef != 0 {
		size += sizeTag(fieldTPRuntimeIDRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.RuntimeIDRef)))
	}
	if tp.EnvRef != 0 {
		size += sizeTag(fieldTPEnvRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.EnvRef)))
	}
	if tp.HostnameRef != 0 {
		size += sizeTag(fieldTPHostnameRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.HostnameRef)))
	}
	if tp.AppVersionRef != 0 {
		size += sizeTag(fieldTPAppVersionRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(tp.AppVersionRef)))
	}

	// Field 10: attributes
	size += sizeAttributesWithMapper(fieldTPAttributes, tp.Attributes, mapper)

	// Field 11: chunks
	for _, chunk := range tp.Chunks {
		chunkSize := sizeTraceChunkWithMapper(chunk, mapper)
		size += sizeTag(fieldTPChunks, wireLengthDelim)
		size += sizeVarint(uint64(chunkSize))
		size += chunkSize
	}

	return size
}

// sizeTraceChunkWithMapper calculates the serialized size of a TraceChunk
func sizeTraceChunkWithMapper(chunk *idx.TraceChunk, mapper *stringRefMapper) int {
	if chunk == nil {
		return 0
	}
	size := 0

	if chunk.Priority != 0 {
		size += sizeTag(fieldTCPriority, wireVarint)
		size += sizeVarint(uint64(uint32(chunk.Priority)))
	}
	if chunk.OriginRef != 0 {
		size += sizeTag(fieldTCOriginRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(chunk.OriginRef)))
	}

	size += sizeAttributesWithMapper(fieldTCAttributes, chunk.Attributes, mapper)

	for _, span := range chunk.Spans {
		spanSize := sizeSpanWithMapper(span, mapper)
		size += sizeTag(fieldTCSpans, wireLengthDelim)
		size += sizeVarint(uint64(spanSize))
		size += spanSize
	}

	if chunk.DroppedTrace {
		size += sizeTag(fieldTCDroppedTrace, wireVarint)
		size++
	}
	if len(chunk.TraceID) > 0 {
		size += sizeTag(fieldTCTraceID, wireLengthDelim)
		size += sizeVarint(uint64(len(chunk.TraceID)))
		size += len(chunk.TraceID)
	}
	if chunk.SamplingMechanism != 0 {
		size += sizeTag(fieldTCSamplingMechanism, wireVarint)
		size += sizeVarint(uint64(chunk.SamplingMechanism))
	}

	return size
}

// sizeSpanWithMapper calculates the serialized size of a Span
func sizeSpanWithMapper(span *idx.Span, mapper *stringRefMapper) int {
	if span == nil {
		return 0
	}
	size := 0

	if span.ServiceRef != 0 {
		size += sizeTag(fieldSpanServiceRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(span.ServiceRef)))
	}
	if span.NameRef != 0 {
		size += sizeTag(fieldSpanNameRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(span.NameRef)))
	}
	if span.ResourceRef != 0 {
		size += sizeTag(fieldSpanResourceRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(span.ResourceRef)))
	}
	if span.SpanID != 0 {
		size += sizeTag(fieldSpanSpanID, wireFixed64) + 8
	}
	if span.ParentID != 0 {
		size += sizeTag(fieldSpanParentID, wireVarint)
		size += sizeVarint(span.ParentID)
	}
	if span.Start != 0 {
		size += sizeTag(fieldSpanStart, wireFixed64) + 8
	}
	if span.Duration != 0 {
		size += sizeTag(fieldSpanDuration, wireVarint)
		size += sizeVarint(span.Duration)
	}
	if span.Error {
		size += sizeTag(fieldSpanError, wireVarint) + 1
	}

	size += sizeAttributesWithMapper(fieldSpanAttributes, span.Attributes, mapper)

	if span.TypeRef != 0 {
		size += sizeTag(fieldSpanTypeRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(span.TypeRef)))
	}

	for _, link := range span.Links {
		linkSize := sizeSpanLinkWithMapper(link, mapper)
		size += sizeTag(fieldSpanLinks, wireLengthDelim)
		size += sizeVarint(uint64(linkSize))
		size += linkSize
	}

	for _, event := range span.Events {
		eventSize := sizeSpanEventWithMapper(event, mapper)
		size += sizeTag(fieldSpanEvents, wireLengthDelim)
		size += sizeVarint(uint64(eventSize))
		size += eventSize
	}

	if span.EnvRef != 0 {
		size += sizeTag(fieldSpanEnvRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(span.EnvRef)))
	}
	if span.VersionRef != 0 {
		size += sizeTag(fieldSpanVersionRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(span.VersionRef)))
	}
	if span.ComponentRef != 0 {
		size += sizeTag(fieldSpanComponentRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(span.ComponentRef)))
	}
	if span.Kind != 0 {
		size += sizeTag(fieldSpanKind, wireVarint)
		size += sizeVarint(uint64(span.Kind))
	}

	return size
}

// sizeSpanLinkWithMapper calculates the serialized size of a SpanLink
func sizeSpanLinkWithMapper(link *idx.SpanLink, mapper *stringRefMapper) int {
	if link == nil {
		return 0
	}
	size := 0

	if len(link.TraceID) > 0 {
		size += sizeTag(fieldSpanLinkTraceID, wireLengthDelim)
		size += sizeVarint(uint64(len(link.TraceID)))
		size += len(link.TraceID)
	}
	if link.SpanID != 0 {
		size += sizeTag(fieldSpanLinkSpanID, wireFixed64) + 8
	}
	size += sizeAttributesWithMapper(fieldSpanLinkAttributes, link.Attributes, mapper)
	if link.TracestateRef != 0 {
		size += sizeTag(fieldSpanLinkTracestateRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(link.TracestateRef)))
	}
	if link.Flags != 0 {
		size += sizeTag(fieldSpanLinkFlags, wireVarint)
		size += sizeVarint(uint64(link.Flags))
	}

	return size
}

// sizeSpanEventWithMapper calculates the serialized size of a SpanEvent
func sizeSpanEventWithMapper(event *idx.SpanEvent, mapper *stringRefMapper) int {
	if event == nil {
		return 0
	}
	size := 0

	if event.Time != 0 {
		size += sizeTag(fieldSpanEventTime, wireFixed64) + 8
	}
	if event.NameRef != 0 {
		size += sizeTag(fieldSpanEventNameRef, wireVarint)
		size += sizeVarint(uint64(mapper.remap(event.NameRef)))
	}
	size += sizeAttributesWithMapper(fieldSpanEventAttributes, event.Attributes, mapper)

	return size
}

// sizeAttributesWithMapper calculates the serialized size of an attributes map
func sizeAttributesWithMapper(fieldNum int, attrs map[uint32]*idx.AnyValue, mapper *stringRefMapper) int {
	if len(attrs) == 0 {
		return 0
	}
	size := 0

	for oldKey, value := range attrs {
		newKey := mapper.remap(oldKey)

		// Entry size: key field + value field
		entrySize := sizeTag(1, wireVarint) + sizeVarint(uint64(newKey))
		valueSize := sizeAnyValueWithMapper(value, mapper)
		entrySize += sizeTag(2, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize

		// Map entry wrapper
		size += sizeTag(fieldNum, wireLengthDelim)
		size += sizeVarint(uint64(entrySize))
		size += entrySize
	}

	return size
}

// sizeAnyValueWithMapper calculates the serialized size of an AnyValue
func sizeAnyValueWithMapper(av *idx.AnyValue, mapper *stringRefMapper) int {
	if av == nil {
		return 0
	}

	switch v := av.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return sizeTag(fieldAnyValueStringValueRef, wireVarint) + sizeVarint(uint64(mapper.remap(v.StringValueRef)))

	case *idx.AnyValue_BoolValue:
		return sizeTag(fieldAnyValueBoolValue, wireVarint) + 1

	case *idx.AnyValue_DoubleValue:
		return sizeTag(fieldAnyValueDoubleValue, wireFixed64) + 8

	case *idx.AnyValue_IntValue:
		return sizeTag(fieldAnyValueIntValue, wireVarint) + sizeVarint(uint64(v.IntValue))

	case *idx.AnyValue_BytesValue:
		return sizeTag(fieldAnyValueBytesValue, wireLengthDelim) + sizeVarint(uint64(len(v.BytesValue))) + len(v.BytesValue)

	case *idx.AnyValue_ArrayValue:
		if v.ArrayValue == nil {
			return 0
		}
		arraySize := 0
		for _, elem := range v.ArrayValue.Values {
			elemSize := sizeAnyValueWithMapper(elem, mapper)
			arraySize += sizeTag(fieldArrayValueValues, wireLengthDelim) + sizeVarint(uint64(elemSize)) + elemSize
		}
		return sizeTag(fieldAnyValueArrayValue, wireLengthDelim) + sizeVarint(uint64(arraySize)) + arraySize

	case *idx.AnyValue_KeyValueList:
		if v.KeyValueList == nil {
			return 0
		}
		kvListSize := 0
		for _, kv := range v.KeyValueList.KeyValues {
			if kv != nil {
				kvSize := sizeTag(fieldKeyValueKey, wireVarint) + sizeVarint(uint64(mapper.remap(kv.Key)))
				valueSize := sizeAnyValueWithMapper(kv.Value, mapper)
				kvSize += sizeTag(fieldKeyValueValue, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize
				kvListSize += sizeTag(fieldKeyValueListKeyValues, wireLengthDelim) + sizeVarint(uint64(kvSize)) + kvSize
			}
		}
		return sizeTag(fieldAnyValueKeyValueList, wireLengthDelim) + sizeVarint(uint64(kvListSize)) + kvListSize
	}

	return 0
}

// appendTracerPayloadWithMapper serializes a TracerPayload with remapped string references
func appendTracerPayloadWithMapper(buf []byte, tp *idx.TracerPayload, compactStrings []string, mapper *stringRefMapper) []byte {
	// Field 1: strings (repeated string)
	for _, s := range compactStrings {
		buf = appendTag(buf, fieldTPStrings, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(s)))
		buf = append(buf, s...)
	}

	// Field 2: containerIDRef
	if tp.ContainerIDRef != 0 {
		buf = appendTag(buf, fieldTPContainerIDRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.ContainerIDRef)))
	}

	// Field 3: languageNameRef
	if tp.LanguageNameRef != 0 {
		buf = appendTag(buf, fieldTPLanguageNameRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.LanguageNameRef)))
	}

	// Field 4: languageVersionRef
	if tp.LanguageVersionRef != 0 {
		buf = appendTag(buf, fieldTPLanguageVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.LanguageVersionRef)))
	}

	// Field 5: tracerVersionRef
	if tp.TracerVersionRef != 0 {
		buf = appendTag(buf, fieldTPTracerVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.TracerVersionRef)))
	}

	// Field 6: runtimeIDRef
	if tp.RuntimeIDRef != 0 {
		buf = appendTag(buf, fieldTPRuntimeIDRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.RuntimeIDRef)))
	}

	// Field 7: envRef
	if tp.EnvRef != 0 {
		buf = appendTag(buf, fieldTPEnvRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.EnvRef)))
	}

	// Field 8: hostnameRef
	if tp.HostnameRef != 0 {
		buf = appendTag(buf, fieldTPHostnameRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.HostnameRef)))
	}

	// Field 9: appVersionRef
	if tp.AppVersionRef != 0 {
		buf = appendTag(buf, fieldTPAppVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(tp.AppVersionRef)))
	}

	// Field 10: attributes (map<uint32, AnyValue>)
	buf = appendAttributesWithMapper(buf, fieldTPAttributes, tp.Attributes, mapper)

	// Field 11: chunks (repeated TraceChunk)
	for _, chunk := range tp.Chunks {
		buf = appendTraceChunkWithMapper(buf, chunk, mapper)
	}

	return buf
}

// appendTraceChunkWithMapper serializes a TraceChunk with remapped string references
func appendTraceChunkWithMapper(buf []byte, chunk *idx.TraceChunk, mapper *stringRefMapper) []byte {
	if chunk == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	chunkSize := sizeTraceChunkWithMapper(chunk, mapper)
	buf = appendTag(buf, fieldTPChunks, wireLengthDelim)
	buf = appendVarint(buf, uint64(chunkSize))

	// Write content directly to buf (no intermediate buffer)
	if chunk.Priority != 0 {
		buf = appendTag(buf, fieldTCPriority, wireVarint)
		buf = appendVarint(buf, uint64(uint32(chunk.Priority)))
	}
	if chunk.OriginRef != 0 {
		buf = appendTag(buf, fieldTCOriginRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(chunk.OriginRef)))
	}
	buf = appendAttributesWithMapper(buf, fieldTCAttributes, chunk.Attributes, mapper)
	for _, span := range chunk.Spans {
		buf = appendSpanWithMapper(buf, span, mapper)
	}
	if chunk.DroppedTrace {
		buf = appendTag(buf, fieldTCDroppedTrace, wireVarint)
		buf = append(buf, 1)
	}
	if len(chunk.TraceID) > 0 {
		buf = appendTag(buf, fieldTCTraceID, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(chunk.TraceID)))
		buf = append(buf, chunk.TraceID...)
	}
	if chunk.SamplingMechanism != 0 {
		buf = appendTag(buf, fieldTCSamplingMechanism, wireVarint)
		buf = appendVarint(buf, uint64(chunk.SamplingMechanism))
	}

	return buf
}

// appendSpanWithMapper serializes a Span with remapped string references
func appendSpanWithMapper(buf []byte, span *idx.Span, mapper *stringRefMapper) []byte {
	if span == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	spanSize := sizeSpanWithMapper(span, mapper)
	buf = appendTag(buf, fieldTCSpans, wireLengthDelim)
	buf = appendVarint(buf, uint64(spanSize))

	// Write content directly to buf (no intermediate buffer)
	if span.ServiceRef != 0 {
		buf = appendTag(buf, fieldSpanServiceRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(span.ServiceRef)))
	}
	if span.NameRef != 0 {
		buf = appendTag(buf, fieldSpanNameRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(span.NameRef)))
	}
	if span.ResourceRef != 0 {
		buf = appendTag(buf, fieldSpanResourceRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(span.ResourceRef)))
	}
	if span.SpanID != 0 {
		buf = appendTag(buf, fieldSpanSpanID, wireFixed64)
		buf = appendFixed64(buf, span.SpanID)
	}
	if span.ParentID != 0 {
		buf = appendTag(buf, fieldSpanParentID, wireVarint)
		buf = appendVarint(buf, span.ParentID)
	}
	if span.Start != 0 {
		buf = appendTag(buf, fieldSpanStart, wireFixed64)
		buf = appendFixed64(buf, span.Start)
	}
	if span.Duration != 0 {
		buf = appendTag(buf, fieldSpanDuration, wireVarint)
		buf = appendVarint(buf, span.Duration)
	}
	if span.Error {
		buf = appendTag(buf, fieldSpanError, wireVarint)
		buf = append(buf, 1)
	}
	buf = appendAttributesWithMapper(buf, fieldSpanAttributes, span.Attributes, mapper)
	if span.TypeRef != 0 {
		buf = appendTag(buf, fieldSpanTypeRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(span.TypeRef)))
	}
	for _, link := range span.Links {
		buf = appendSpanLinkWithMapper(buf, link, mapper)
	}
	for _, event := range span.Events {
		buf = appendSpanEventWithMapper(buf, event, mapper)
	}
	if span.EnvRef != 0 {
		buf = appendTag(buf, fieldSpanEnvRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(span.EnvRef)))
	}
	if span.VersionRef != 0 {
		buf = appendTag(buf, fieldSpanVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(span.VersionRef)))
	}
	if span.ComponentRef != 0 {
		buf = appendTag(buf, fieldSpanComponentRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(span.ComponentRef)))
	}
	if span.Kind != 0 {
		buf = appendTag(buf, fieldSpanKind, wireVarint)
		buf = appendVarint(buf, uint64(span.Kind))
	}

	return buf
}

// appendSpanLinkWithMapper serializes a SpanLink with remapped string references
func appendSpanLinkWithMapper(buf []byte, link *idx.SpanLink, mapper *stringRefMapper) []byte {
	if link == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	linkSize := sizeSpanLinkWithMapper(link, mapper)
	buf = appendTag(buf, fieldSpanLinks, wireLengthDelim)
	buf = appendVarint(buf, uint64(linkSize))

	// Write content directly
	if len(link.TraceID) > 0 {
		buf = appendTag(buf, fieldSpanLinkTraceID, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(link.TraceID)))
		buf = append(buf, link.TraceID...)
	}
	if link.SpanID != 0 {
		buf = appendTag(buf, fieldSpanLinkSpanID, wireFixed64)
		buf = appendFixed64(buf, link.SpanID)
	}
	buf = appendAttributesWithMapper(buf, fieldSpanLinkAttributes, link.Attributes, mapper)
	if link.TracestateRef != 0 {
		buf = appendTag(buf, fieldSpanLinkTracestateRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(link.TracestateRef)))
	}
	if link.Flags != 0 {
		buf = appendTag(buf, fieldSpanLinkFlags, wireVarint)
		buf = appendVarint(buf, uint64(link.Flags))
	}

	return buf
}

// appendSpanEventWithMapper serializes a SpanEvent with remapped string references
func appendSpanEventWithMapper(buf []byte, event *idx.SpanEvent, mapper *stringRefMapper) []byte {
	if event == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	eventSize := sizeSpanEventWithMapper(event, mapper)
	buf = appendTag(buf, fieldSpanEvents, wireLengthDelim)
	buf = appendVarint(buf, uint64(eventSize))

	// Write content directly
	if event.Time != 0 {
		buf = appendTag(buf, fieldSpanEventTime, wireFixed64)
		buf = appendFixed64(buf, event.Time)
	}
	if event.NameRef != 0 {
		buf = appendTag(buf, fieldSpanEventNameRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(event.NameRef)))
	}
	buf = appendAttributesWithMapper(buf, fieldSpanEventAttributes, event.Attributes, mapper)

	return buf
}

// appendAttributesWithMapper serializes an attributes map with remapped string references
func appendAttributesWithMapper(buf []byte, fieldNum int, attrs map[uint32]*idx.AnyValue, mapper *stringRefMapper) []byte {
	if len(attrs) == 0 {
		return buf
	}

	// Sort keys for deterministic output
	keys := make([]uint32, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, oldKey := range keys {
		value := attrs[oldKey]
		newKey := mapper.remap(oldKey)

		// Calculate entry size
		valueSize := sizeAnyValueWithMapper(value, mapper)
		entrySize := sizeTag(1, wireVarint) + sizeVarint(uint64(newKey))
		entrySize += sizeTag(2, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize

		// Write map entry directly
		buf = appendTag(buf, fieldNum, wireLengthDelim)
		buf = appendVarint(buf, uint64(entrySize))

		// Key field
		buf = appendTag(buf, 1, wireVarint)
		buf = appendVarint(buf, uint64(newKey))

		// Value field
		buf = appendTag(buf, 2, wireLengthDelim)
		buf = appendVarint(buf, uint64(valueSize))
		buf = appendAnyValueWithMapper(buf, value, mapper)
	}

	return buf
}

// appendAnyValueWithMapper serializes an AnyValue with remapped string references
func appendAnyValueWithMapper(buf []byte, av *idx.AnyValue, mapper *stringRefMapper) []byte {
	if av == nil {
		return buf
	}

	switch v := av.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		buf = appendTag(buf, fieldAnyValueStringValueRef, wireVarint)
		buf = appendVarint(buf, uint64(mapper.remap(v.StringValueRef)))

	case *idx.AnyValue_BoolValue:
		buf = appendTag(buf, fieldAnyValueBoolValue, wireVarint)
		if v.BoolValue {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}

	case *idx.AnyValue_DoubleValue:
		buf = appendTag(buf, fieldAnyValueDoubleValue, wireFixed64)
		buf = appendFixed64(buf, math.Float64bits(v.DoubleValue))

	case *idx.AnyValue_IntValue:
		buf = appendTag(buf, fieldAnyValueIntValue, wireVarint)
		buf = appendVarint(buf, uint64(v.IntValue))

	case *idx.AnyValue_BytesValue:
		buf = appendTag(buf, fieldAnyValueBytesValue, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(v.BytesValue)))
		buf = append(buf, v.BytesValue...)

	case *idx.AnyValue_ArrayValue:
		if v.ArrayValue != nil {
			// Calculate array size
			arraySize := 0
			for _, elem := range v.ArrayValue.Values {
				elemSize := sizeAnyValueWithMapper(elem, mapper)
				arraySize += sizeTag(fieldArrayValueValues, wireLengthDelim) + sizeVarint(uint64(elemSize)) + elemSize
			}

			// Write directly
			buf = appendTag(buf, fieldAnyValueArrayValue, wireLengthDelim)
			buf = appendVarint(buf, uint64(arraySize))
			for _, elem := range v.ArrayValue.Values {
				elemSize := sizeAnyValueWithMapper(elem, mapper)
				buf = appendTag(buf, fieldArrayValueValues, wireLengthDelim)
				buf = appendVarint(buf, uint64(elemSize))
				buf = appendAnyValueWithMapper(buf, elem, mapper)
			}
		}

	case *idx.AnyValue_KeyValueList:
		if v.KeyValueList != nil {
			// Calculate kvList size
			kvListSize := 0
			for _, kv := range v.KeyValueList.KeyValues {
				if kv != nil {
					valueSize := sizeAnyValueWithMapper(kv.Value, mapper)
					kvSize := sizeTag(fieldKeyValueKey, wireVarint) + sizeVarint(uint64(mapper.remap(kv.Key)))
					kvSize += sizeTag(fieldKeyValueValue, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize
					kvListSize += sizeTag(fieldKeyValueListKeyValues, wireLengthDelim) + sizeVarint(uint64(kvSize)) + kvSize
				}
			}

			// Write directly
			buf = appendTag(buf, fieldAnyValueKeyValueList, wireLengthDelim)
			buf = appendVarint(buf, uint64(kvListSize))
			for _, kv := range v.KeyValueList.KeyValues {
				if kv != nil {
					valueSize := sizeAnyValueWithMapper(kv.Value, mapper)
					kvSize := sizeTag(fieldKeyValueKey, wireVarint) + sizeVarint(uint64(mapper.remap(kv.Key)))
					kvSize += sizeTag(fieldKeyValueValue, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize

					buf = appendTag(buf, fieldKeyValueListKeyValues, wireLengthDelim)
					buf = appendVarint(buf, uint64(kvSize))
					buf = appendTag(buf, fieldKeyValueKey, wireVarint)
					buf = appendVarint(buf, uint64(mapper.remap(kv.Key)))
					buf = appendTag(buf, fieldKeyValueValue, wireLengthDelim)
					buf = appendVarint(buf, uint64(valueSize))
					buf = appendAnyValueWithMapper(buf, kv.Value, mapper)
				}
			}
		}
	}

	return buf
}

// sizeTag returns the size of a protobuf tag.
func sizeTag(fieldNum int, wireType int) int {
	return sizeVarint(uint64(fieldNum<<3 | wireType))
}

// appendTag appends a protobuf tag to the buffer.
func appendTag(buf []byte, fieldNum int, wireType int) []byte {
	return appendVarint(buf, uint64(fieldNum<<3|wireType))
}

// sizeVarint returns the size of a varint-encoded uint64.
func sizeVarint(v uint64) int {
	switch {
	case v < 1<<7:
		return 1
	case v < 1<<14:
		return 2
	case v < 1<<21:
		return 3
	case v < 1<<28:
		return 4
	case v < 1<<35:
		return 5
	case v < 1<<42:
		return 6
	case v < 1<<49:
		return 7
	case v < 1<<56:
		return 8
	case v < 1<<63:
		return 9
	default:
		return 10
	}
}

// appendVarint appends a varint-encoded uint64 to the buffer.
func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// appendFixed64 appends a fixed64 (little-endian) to the buffer.
func appendFixed64(buf []byte, v uint64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	return append(buf, b[:]...)
}

// sizeMapEntry calculates the size of a map entry (key: string, value: string).
// Map entries are encoded as: field1 (key) + field2 (value)
func sizeMapEntry(key, value string) int {
	// key is field 1, string type
	size := sizeTag(1, wireLengthDelim)
	size += sizeVarint(uint64(len(key)))
	size += len(key)
	// value is field 2, string type
	size += sizeTag(2, wireLengthDelim)
	size += sizeVarint(uint64(len(value)))
	size += len(value)
	return size
}

// appendMapEntry appends a map entry to the buffer.
func appendMapEntry(buf []byte, key, value string) []byte {
	// key is field 1, string type
	buf = appendTag(buf, 1, wireLengthDelim)
	buf = appendVarint(buf, uint64(len(key)))
	buf = append(buf, key...)
	// value is field 2, string type
	buf = appendTag(buf, 2, wireLengthDelim)
	buf = appendVarint(buf, uint64(len(value)))
	buf = append(buf, value...)
	return buf
}
