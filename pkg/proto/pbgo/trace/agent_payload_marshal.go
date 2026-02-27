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

// notMapped is a sentinel value indicating an unassigned/unused string reference
const notMapped uint32 = ^uint32(0)

// stringCompactor collects used string references and creates a compact mapping
// during serialization without modifying the original payload.
type stringCompactor struct {
	originalStrings []string
	oldToNew        []uint32
	newStrings      []string
}

// newStringCompactor creates a compactor that scans the TracerPayload to identify
// all used string references and builds a compact remapping.
func newStringCompactor(tp *idx.TracerPayload) *stringCompactor {
	if tp == nil || len(tp.Strings) == 0 {
		return &stringCompactor{
			originalStrings: nil,
			oldToNew:        nil,
			newStrings:      []string{""},
		}
	}

	c := &stringCompactor{
		originalStrings: tp.Strings,
		oldToNew:        make([]uint32, len(tp.Strings)),
	}

	// Initialize all to notMapped
	for i := range c.oldToNew {
		c.oldToNew[i] = notMapped
	}

	nextIndex := uint32(0)

	// markRef marks a string reference as used and assigns it a new index
	markRef := func(oldRef uint32) {
		if int(oldRef) < len(c.oldToNew) && c.oldToNew[oldRef] == notMapped {
			c.oldToNew[oldRef] = nextIndex
			nextIndex++
		}
	}

	// Always mark index 0 first (empty string sentinel)
	markRef(0)

	// Collect TracerPayload level refs
	markRef(tp.ContainerIDRef)
	markRef(tp.LanguageNameRef)
	markRef(tp.LanguageVersionRef)
	markRef(tp.TracerVersionRef)
	markRef(tp.RuntimeIDRef)
	markRef(tp.EnvRef)
	markRef(tp.HostnameRef)
	markRef(tp.AppVersionRef)
	c.collectAttributeRefs(tp.Attributes, markRef)

	// Collect from chunks
	for _, chunk := range tp.Chunks {
		if chunk == nil {
			continue
		}
		markRef(chunk.OriginRef)
		c.collectAttributeRefs(chunk.Attributes, markRef)
		for _, span := range chunk.Spans {
			if span == nil {
				continue
			}
			markRef(span.ServiceRef)
			markRef(span.NameRef)
			markRef(span.ResourceRef)
			markRef(span.TypeRef)
			markRef(span.EnvRef)
			markRef(span.VersionRef)
			markRef(span.ComponentRef)
			c.collectAttributeRefs(span.Attributes, markRef)
			for _, link := range span.Links {
				if link != nil {
					markRef(link.TracestateRef)
					c.collectAttributeRefs(link.Attributes, markRef)
				}
			}
			for _, event := range span.Events {
				if event != nil {
					markRef(event.NameRef)
					c.collectAttributeRefs(event.Attributes, markRef)
				}
			}
		}
	}

	// Build the new compact strings table
	c.newStrings = make([]string, nextIndex)
	for oldIdx, newIdx := range c.oldToNew {
		if newIdx != notMapped {
			c.newStrings[newIdx] = c.originalStrings[oldIdx]
		}
	}

	return c
}

// remap returns the new compacted index for an old reference
func (c *stringCompactor) remap(oldRef uint32) uint32 {
	if c.oldToNew == nil || int(oldRef) >= len(c.oldToNew) {
		return 0
	}
	newRef := c.oldToNew[oldRef]
	if newRef == notMapped {
		return 0
	}
	return newRef
}

// collectAttributeRefs marks all string refs in an attribute map
func (c *stringCompactor) collectAttributeRefs(attrs map[uint32]*idx.AnyValue, markRef func(uint32)) {
	for keyRef, value := range attrs {
		markRef(keyRef)
		c.collectAnyValueRefs(value, markRef)
	}
}

// collectAnyValueRefs marks string refs in an AnyValue
func (c *stringCompactor) collectAnyValueRefs(value *idx.AnyValue, markRef func(uint32)) {
	if value == nil {
		return
	}
	switch v := value.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		markRef(v.StringValueRef)
	case *idx.AnyValue_ArrayValue:
		if v.ArrayValue != nil {
			for _, elem := range v.ArrayValue.Values {
				c.collectAnyValueRefs(elem, markRef)
			}
		}
	case *idx.AnyValue_KeyValueList:
		if v.KeyValueList != nil {
			for _, kv := range v.KeyValueList.KeyValues {
				if kv != nil {
					markRef(kv.Key)
					c.collectAnyValueRefs(kv.Value, markRef)
				}
			}
		}
	}
}

// =============================================================================
// PreparedTracerPayload - pre-computed compaction for efficient size calculation
// =============================================================================

// PreparedTracerPayload holds a TracerPayload along with its pre-computed string
// compactor and size. This allows accurate size calculations for buffer management
// and avoids recomputing the compaction during serialization.
type PreparedTracerPayload struct {
	payload   *idx.TracerPayload
	compactor *stringCompactor
	Size      int // compacted serialized size
}

// PrepareTracerPayload creates a PreparedTracerPayload by computing the string
// compaction and size upfront. The compactor is reused during serialization.
func PrepareTracerPayload(tp *idx.TracerPayload) *PreparedTracerPayload {
	if tp == nil {
		return &PreparedTracerPayload{payload: nil, compactor: nil, Size: 0}
	}
	c := newStringCompactor(tp)
	size := sizeTracerPayloadCompacting(tp, c)
	return &PreparedTracerPayload{
		payload:   tp,
		compactor: c,
		Size:      size,
	}
}

// PrepareTracerPayloadWithChunks creates a PreparedTracerPayload using only
// a subset of chunks from the source payload. This is useful for splitting
// large payloads while reusing the string table. PreparedTracerPayloads must not be modified to avoid race conditions.
func PrepareTracerPayloadWithChunks(source *idx.TracerPayload, chunks []*idx.TraceChunk) *PreparedTracerPayload {
	if source == nil {
		return &PreparedTracerPayload{payload: nil, compactor: nil, Size: 0}
	}
	// Create a new TracerPayload with the same metadata but only the specified chunks
	// The resulting TP must not have any strings modified after this point.
	tp := &idx.TracerPayload{
		Strings:            source.Strings,
		ContainerIDRef:     source.ContainerIDRef,
		LanguageNameRef:    source.LanguageNameRef,
		LanguageVersionRef: source.LanguageVersionRef,
		TracerVersionRef:   source.TracerVersionRef,
		RuntimeIDRef:       source.RuntimeIDRef,
		EnvRef:             source.EnvRef,
		HostnameRef:        source.HostnameRef,
		AppVersionRef:      source.AppVersionRef,
		Attributes:         source.Attributes,
		Chunks:             chunks,
	}
	return PrepareTracerPayload(tp)
}

// MarshalAgentPayload serializes an AgentPayload to protobuf binary format.
// This is a custom serializer that ignores the TracerPayloads field and only
// serializes the IdxTracerPayloads field along with the other fields.
// String compaction is performed during serialization without modifying the input payload.
func MarshalAgentPayload(ap *AgentPayload) ([]byte, error) {
	if ap == nil {
		return nil, nil
	}
	// We can't pre-calculate exact size due to string compaction, so we estimate
	buf := make([]byte, 0, 4096)
	return AppendAgentPayload(buf, ap)
}

// MarshalAgentPayloadPrepared serializes an AgentPayload using pre-computed
// PreparedTracerPayloads. This is more efficient when the compaction has already
// been computed for size calculations, as it avoids recomputing the string mapping.
// Note: The ap.IdxTracerPayloads field is ignored; only the prepared payloads are serialized.
func MarshalAgentPayloadPrepared(ap *AgentPayload, prepared []*PreparedTracerPayload) ([]byte, error) {
	if ap == nil {
		return nil, nil
	}
	// Calculate total size for better allocation
	size := sizeAgentPayloadBase(ap)
	for _, p := range prepared {
		if p != nil {
			size += sizeTag(fieldIdxTracerPayloads, wireLengthDelim)
			size += sizeVarint(uint64(p.Size))
			size += p.Size
		}
	}
	buf := make([]byte, 0, size)
	return appendAgentPayloadPrepared(buf, ap, prepared)
}

// sizeAgentPayloadBase calculates the size of AgentPayload fields excluding IdxTracerPayloads
func sizeAgentPayloadBase(ap *AgentPayload) int {
	if ap == nil {
		return 0
	}
	size := 0

	if len(ap.HostName) > 0 {
		size += sizeTag(fieldHostName, wireLengthDelim)
		size += sizeVarint(uint64(len(ap.HostName)))
		size += len(ap.HostName)
	}
	if len(ap.Env) > 0 {
		size += sizeTag(fieldEnv, wireLengthDelim)
		size += sizeVarint(uint64(len(ap.Env)))
		size += len(ap.Env)
	}
	for k, v := range ap.Tags {
		mapEntrySize := sizeMapEntry(k, v)
		size += sizeTag(fieldTags, wireLengthDelim)
		size += sizeVarint(uint64(mapEntrySize))
		size += mapEntrySize
	}
	if len(ap.AgentVersion) > 0 {
		size += sizeTag(fieldAgentVersion, wireLengthDelim)
		size += sizeVarint(uint64(len(ap.AgentVersion)))
		size += len(ap.AgentVersion)
	}
	if ap.TargetTPS != 0 {
		size += sizeTag(fieldTargetTPS, wireFixed64) + 8
	}
	if ap.ErrorTPS != 0 {
		size += sizeTag(fieldErrorTPS, wireFixed64) + 8
	}
	if ap.RareSamplerEnabled {
		size += sizeTag(fieldRareSamplerEnabled, wireVarint) + 1
	}
	return size
}

// appendAgentPayloadPrepared appends the serialized AgentPayload using prepared payloads.
func appendAgentPayloadPrepared(buf []byte, ap *AgentPayload, prepared []*PreparedTracerPayload) ([]byte, error) {
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

	// Field 6: tags (map<string, string>)
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

	// Field 11: idxTracerPayloads (using prepared payloads with pre-computed compactors)
	for _, p := range prepared {
		buf = appendPreparedIdxTracerPayload(buf, p)
	}

	return buf, nil
}

// appendPreparedIdxTracerPayload serializes a PreparedTracerPayload using its
// pre-computed compactor, avoiding the need to recompute the string mapping.
func appendPreparedIdxTracerPayload(buf []byte, p *PreparedTracerPayload) []byte {
	if p == nil || p.payload == nil {
		return buf
	}

	// Use pre-computed size
	payloadSize := p.Size

	// Write tag + length prefix, then serialize directly
	buf = appendTag(buf, fieldIdxTracerPayloads, wireLengthDelim)
	buf = appendVarint(buf, uint64(payloadSize))

	// Ensure buffer has enough capacity
	if cap(buf)-len(buf) < payloadSize {
		newBuf := make([]byte, len(buf), len(buf)+payloadSize)
		copy(newBuf, buf)
		buf = newBuf
	}

	// Use pre-computed compactor
	buf = appendTracerPayloadCompacting(buf, p.payload, p.compactor)
	return buf
}

// SizeAgentPayload calculates the size of the serialized AgentPayload.
// Note: This returns an estimate as exact size depends on string compaction
// which happens during serialization.
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
	// Use compacting size calculation for accurate size
	for _, tp := range ap.IdxTracerPayloads {
		compactor := newStringCompactor(tp)
		payloadSize := sizeTracerPayloadCompacting(tp, compactor)
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
// This function performs string compaction during serialization without modifying the
// input payload. The compacted strings table is serialized last.
func appendIdxTracerPayload(buf []byte, tp *idx.TracerPayload) []byte {
	if tp == nil {
		return buf
	}

	// Create a compactor that scans the payload and builds the compact string mapping
	compactor := newStringCompactor(tp)

	// Calculate exact size needed for the TracerPayload (using compacted refs)
	payloadSize := sizeTracerPayloadCompacting(tp, compactor)

	// Write tag + length prefix, then serialize directly
	buf = appendTag(buf, fieldIdxTracerPayloads, wireLengthDelim)
	buf = appendVarint(buf, uint64(payloadSize))

	// Ensure buffer has enough capacity
	if cap(buf)-len(buf) < payloadSize {
		newBuf := make([]byte, len(buf), len(buf)+payloadSize)
		copy(newBuf, buf)
		buf = newBuf
	}

	buf = appendTracerPayloadCompacting(buf, tp, compactor)
	return buf
}

// =============================================================================
// Compacting size calculation functions (for string compaction during serialization)
// =============================================================================

// sizeTracerPayloadCompacting calculates the serialized size using compacted string refs
func sizeTracerPayloadCompacting(tp *idx.TracerPayload, c *stringCompactor) int {
	size := 0

	// Field 1: strings (compacted) - will be serialized last but counted here
	for _, s := range c.newStrings {
		size += sizeTag(fieldTPStrings, wireLengthDelim)
		size += sizeVarint(uint64(len(s)))
		size += len(s)
	}

	// Field 2-9: various refs (using remapped indices)
	if ref := c.remap(tp.ContainerIDRef); ref != 0 {
		size += sizeTag(fieldTPContainerIDRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(tp.LanguageNameRef); ref != 0 {
		size += sizeTag(fieldTPLanguageNameRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(tp.LanguageVersionRef); ref != 0 {
		size += sizeTag(fieldTPLanguageVersionRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(tp.TracerVersionRef); ref != 0 {
		size += sizeTag(fieldTPTracerVersionRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(tp.RuntimeIDRef); ref != 0 {
		size += sizeTag(fieldTPRuntimeIDRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(tp.EnvRef); ref != 0 {
		size += sizeTag(fieldTPEnvRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(tp.HostnameRef); ref != 0 {
		size += sizeTag(fieldTPHostnameRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(tp.AppVersionRef); ref != 0 {
		size += sizeTag(fieldTPAppVersionRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}

	// Field 10: attributes
	size += sizeIdxAttributesCompacting(fieldTPAttributes, tp.Attributes, c)

	// Field 11: chunks
	for _, chunk := range tp.Chunks {
		chunkSize := sizeTraceChunkCompacting(chunk, c)
		size += sizeTag(fieldTPChunks, wireLengthDelim)
		size += sizeVarint(uint64(chunkSize))
		size += chunkSize
	}

	return size
}

// sizeTraceChunkCompacting calculates the serialized size of a TraceChunk with compaction
func sizeTraceChunkCompacting(chunk *idx.TraceChunk, c *stringCompactor) int {
	if chunk == nil {
		return 0
	}
	size := 0

	if chunk.Priority != 0 {
		size += sizeTag(fieldTCPriority, wireVarint)
		size += sizeVarint(uint64(uint32(chunk.Priority)))
	}
	if ref := c.remap(chunk.OriginRef); ref != 0 {
		size += sizeTag(fieldTCOriginRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}

	size += sizeIdxAttributesCompacting(fieldTCAttributes, chunk.Attributes, c)

	for _, span := range chunk.Spans {
		spanSize := sizeIdxSpanCompacting(span, c)
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

// sizeIdxSpanCompacting calculates the serialized size of a Span with compaction
func sizeIdxSpanCompacting(span *idx.Span, c *stringCompactor) int {
	if span == nil {
		return 0
	}
	size := 0

	if ref := c.remap(span.ServiceRef); ref != 0 {
		size += sizeTag(fieldSpanServiceRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(span.NameRef); ref != 0 {
		size += sizeTag(fieldSpanNameRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(span.ResourceRef); ref != 0 {
		size += sizeTag(fieldSpanResourceRef, wireVarint)
		size += sizeVarint(uint64(ref))
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

	size += sizeIdxAttributesCompacting(fieldSpanAttributes, span.Attributes, c)

	if ref := c.remap(span.TypeRef); ref != 0 {
		size += sizeTag(fieldSpanTypeRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}

	for _, link := range span.Links {
		linkSize := sizeSpanLinkCompacting(link, c)
		size += sizeTag(fieldSpanLinks, wireLengthDelim)
		size += sizeVarint(uint64(linkSize))
		size += linkSize
	}

	for _, event := range span.Events {
		eventSize := sizeSpanEventCompacting(event, c)
		size += sizeTag(fieldSpanEvents, wireLengthDelim)
		size += sizeVarint(uint64(eventSize))
		size += eventSize
	}

	if ref := c.remap(span.EnvRef); ref != 0 {
		size += sizeTag(fieldSpanEnvRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(span.VersionRef); ref != 0 {
		size += sizeTag(fieldSpanVersionRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if ref := c.remap(span.ComponentRef); ref != 0 {
		size += sizeTag(fieldSpanComponentRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if span.Kind != 0 {
		size += sizeTag(fieldSpanKind, wireVarint)
		size += sizeVarint(uint64(span.Kind))
	}

	return size
}

// sizeSpanLinkCompacting calculates the serialized size of a SpanLink with compaction
func sizeSpanLinkCompacting(link *idx.SpanLink, c *stringCompactor) int {
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
	size += sizeIdxAttributesCompacting(fieldSpanLinkAttributes, link.Attributes, c)
	if ref := c.remap(link.TracestateRef); ref != 0 {
		size += sizeTag(fieldSpanLinkTracestateRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	if link.Flags != 0 {
		size += sizeTag(fieldSpanLinkFlags, wireVarint)
		size += sizeVarint(uint64(link.Flags))
	}

	return size
}

// sizeSpanEventCompacting calculates the serialized size of a SpanEvent with compaction
func sizeSpanEventCompacting(event *idx.SpanEvent, c *stringCompactor) int {
	if event == nil {
		return 0
	}
	size := 0

	if event.Time != 0 {
		size += sizeTag(fieldSpanEventTime, wireFixed64) + 8
	}
	if ref := c.remap(event.NameRef); ref != 0 {
		size += sizeTag(fieldSpanEventNameRef, wireVarint)
		size += sizeVarint(uint64(ref))
	}
	size += sizeIdxAttributesCompacting(fieldSpanEventAttributes, event.Attributes, c)

	return size
}

// sizeIdxAttributesCompacting calculates the serialized size of an attributes map with compaction
func sizeIdxAttributesCompacting(fieldNum int, attrs map[uint32]*idx.AnyValue, c *stringCompactor) int {
	if len(attrs) == 0 {
		return 0
	}
	size := 0

	for key, value := range attrs {
		remappedKey := c.remap(key)
		// Entry size: key field + value field
		entrySize := sizeTag(1, wireVarint) + sizeVarint(uint64(remappedKey))
		valueSize := sizeIdxAnyValueCompacting(value, c)
		entrySize += sizeTag(2, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize

		// Map entry wrapper
		size += sizeTag(fieldNum, wireLengthDelim)
		size += sizeVarint(uint64(entrySize))
		size += entrySize
	}

	return size
}

// sizeIdxAnyValueCompacting calculates the serialized size of an AnyValue with compaction
func sizeIdxAnyValueCompacting(av *idx.AnyValue, c *stringCompactor) int {
	if av == nil {
		return 0
	}

	switch v := av.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return sizeTag(fieldAnyValueStringValueRef, wireVarint) + sizeVarint(uint64(c.remap(v.StringValueRef)))

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
			elemSize := sizeIdxAnyValueCompacting(elem, c)
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
				remappedKey := c.remap(kv.Key)
				kvSize := sizeTag(fieldKeyValueKey, wireVarint) + sizeVarint(uint64(remappedKey))
				valueSize := sizeIdxAnyValueCompacting(kv.Value, c)
				kvSize += sizeTag(fieldKeyValueValue, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize
				kvListSize += sizeTag(fieldKeyValueListKeyValues, wireLengthDelim) + sizeVarint(uint64(kvSize)) + kvSize
			}
		}
		return sizeTag(fieldAnyValueKeyValueList, wireLengthDelim) + sizeVarint(uint64(kvListSize)) + kvListSize
	}

	return 0
}

// =============================================================================
// Compacting append functions (for string compaction during serialization)
// =============================================================================

// appendTracerPayloadCompacting serializes a TracerPayload with string compaction.
// It serializes fields 2-11 first with remapped refs, then serializes the compacted
// strings table (field 1) last. Protobuf allows fields in any order.
func appendTracerPayloadCompacting(buf []byte, tp *idx.TracerPayload, c *stringCompactor) []byte {
	// Fields 2-11 are serialized first with remapped refs
	// Field 2: containerIDRef
	if ref := c.remap(tp.ContainerIDRef); ref != 0 {
		buf = appendTag(buf, fieldTPContainerIDRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 3: languageNameRef
	if ref := c.remap(tp.LanguageNameRef); ref != 0 {
		buf = appendTag(buf, fieldTPLanguageNameRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 4: languageVersionRef
	if ref := c.remap(tp.LanguageVersionRef); ref != 0 {
		buf = appendTag(buf, fieldTPLanguageVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 5: tracerVersionRef
	if ref := c.remap(tp.TracerVersionRef); ref != 0 {
		buf = appendTag(buf, fieldTPTracerVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 6: runtimeIDRef
	if ref := c.remap(tp.RuntimeIDRef); ref != 0 {
		buf = appendTag(buf, fieldTPRuntimeIDRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 7: envRef
	if ref := c.remap(tp.EnvRef); ref != 0 {
		buf = appendTag(buf, fieldTPEnvRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 8: hostnameRef
	if ref := c.remap(tp.HostnameRef); ref != 0 {
		buf = appendTag(buf, fieldTPHostnameRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 9: appVersionRef
	if ref := c.remap(tp.AppVersionRef); ref != 0 {
		buf = appendTag(buf, fieldTPAppVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}

	// Field 10: attributes (map<uint32, AnyValue>)
	buf = appendIdxAttributesCompacting(buf, fieldTPAttributes, tp.Attributes, c)

	// Field 11: chunks (repeated TraceChunk)
	for _, chunk := range tp.Chunks {
		buf = appendTraceChunkCompacting(buf, chunk, c)
	}

	// Field 1: strings (compacted) - serialized last
	for _, s := range c.newStrings {
		buf = appendTag(buf, fieldTPStrings, wireLengthDelim)
		buf = appendVarint(buf, uint64(len(s)))
		buf = append(buf, s...)
	}

	return buf
}

// appendTraceChunkCompacting serializes a TraceChunk with compacted refs
func appendTraceChunkCompacting(buf []byte, chunk *idx.TraceChunk, c *stringCompactor) []byte {
	if chunk == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	chunkSize := sizeTraceChunkCompacting(chunk, c)
	buf = appendTag(buf, fieldTPChunks, wireLengthDelim)
	buf = appendVarint(buf, uint64(chunkSize))

	// Write content directly to buf (no intermediate buffer)
	if chunk.Priority != 0 {
		buf = appendTag(buf, fieldTCPriority, wireVarint)
		buf = appendVarint(buf, uint64(uint32(chunk.Priority)))
	}
	if ref := c.remap(chunk.OriginRef); ref != 0 {
		buf = appendTag(buf, fieldTCOriginRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	buf = appendIdxAttributesCompacting(buf, fieldTCAttributes, chunk.Attributes, c)
	for _, span := range chunk.Spans {
		buf = appendIdxSpanCompacting(buf, span, c)
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

// appendIdxSpanCompacting serializes a Span with compacted refs
func appendIdxSpanCompacting(buf []byte, span *idx.Span, c *stringCompactor) []byte {
	if span == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	spanSize := sizeIdxSpanCompacting(span, c)
	buf = appendTag(buf, fieldTCSpans, wireLengthDelim)
	buf = appendVarint(buf, uint64(spanSize))

	// Write content directly to buf (no intermediate buffer)
	if ref := c.remap(span.ServiceRef); ref != 0 {
		buf = appendTag(buf, fieldSpanServiceRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	if ref := c.remap(span.NameRef); ref != 0 {
		buf = appendTag(buf, fieldSpanNameRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	if ref := c.remap(span.ResourceRef); ref != 0 {
		buf = appendTag(buf, fieldSpanResourceRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
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
	buf = appendIdxAttributesCompacting(buf, fieldSpanAttributes, span.Attributes, c)
	if ref := c.remap(span.TypeRef); ref != 0 {
		buf = appendTag(buf, fieldSpanTypeRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	for _, link := range span.Links {
		buf = appendSpanLinkCompacting(buf, link, c)
	}
	for _, event := range span.Events {
		buf = appendSpanEventCompacting(buf, event, c)
	}
	if ref := c.remap(span.EnvRef); ref != 0 {
		buf = appendTag(buf, fieldSpanEnvRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	if ref := c.remap(span.VersionRef); ref != 0 {
		buf = appendTag(buf, fieldSpanVersionRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	if ref := c.remap(span.ComponentRef); ref != 0 {
		buf = appendTag(buf, fieldSpanComponentRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	if span.Kind != 0 {
		buf = appendTag(buf, fieldSpanKind, wireVarint)
		buf = appendVarint(buf, uint64(span.Kind))
	}

	return buf
}

// appendSpanLinkCompacting serializes a SpanLink with compacted refs
func appendSpanLinkCompacting(buf []byte, link *idx.SpanLink, c *stringCompactor) []byte {
	if link == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	linkSize := sizeSpanLinkCompacting(link, c)
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
	buf = appendIdxAttributesCompacting(buf, fieldSpanLinkAttributes, link.Attributes, c)
	if ref := c.remap(link.TracestateRef); ref != 0 {
		buf = appendTag(buf, fieldSpanLinkTracestateRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	if link.Flags != 0 {
		buf = appendTag(buf, fieldSpanLinkFlags, wireVarint)
		buf = appendVarint(buf, uint64(link.Flags))
	}

	return buf
}

// appendSpanEventCompacting serializes a SpanEvent with compacted refs
func appendSpanEventCompacting(buf []byte, event *idx.SpanEvent, c *stringCompactor) []byte {
	if event == nil {
		return buf
	}

	// Calculate size first, write length prefix, then write content directly
	eventSize := sizeSpanEventCompacting(event, c)
	buf = appendTag(buf, fieldSpanEvents, wireLengthDelim)
	buf = appendVarint(buf, uint64(eventSize))

	// Write content directly
	if event.Time != 0 {
		buf = appendTag(buf, fieldSpanEventTime, wireFixed64)
		buf = appendFixed64(buf, event.Time)
	}
	if ref := c.remap(event.NameRef); ref != 0 {
		buf = appendTag(buf, fieldSpanEventNameRef, wireVarint)
		buf = appendVarint(buf, uint64(ref))
	}
	buf = appendIdxAttributesCompacting(buf, fieldSpanEventAttributes, event.Attributes, c)

	return buf
}

// appendIdxAttributesCompacting serializes an attributes map with compacted refs
func appendIdxAttributesCompacting(buf []byte, fieldNum int, attrs map[uint32]*idx.AnyValue, c *stringCompactor) []byte {
	if len(attrs) == 0 {
		return buf
	}

	// Sort keys for deterministic output
	keys := make([]uint32, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, key := range keys {
		value := attrs[key]
		remappedKey := c.remap(key)

		// Calculate entry size
		valueSize := sizeIdxAnyValueCompacting(value, c)
		entrySize := sizeTag(1, wireVarint) + sizeVarint(uint64(remappedKey))
		entrySize += sizeTag(2, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize

		// Write map entry directly
		buf = appendTag(buf, fieldNum, wireLengthDelim)
		buf = appendVarint(buf, uint64(entrySize))

		// Key field (remapped)
		buf = appendTag(buf, 1, wireVarint)
		buf = appendVarint(buf, uint64(remappedKey))

		// Value field
		buf = appendTag(buf, 2, wireLengthDelim)
		buf = appendVarint(buf, uint64(valueSize))
		buf = appendIdxAnyValueCompacting(buf, value, c)
	}

	return buf
}

// appendIdxAnyValueCompacting serializes an AnyValue with compacted refs
func appendIdxAnyValueCompacting(buf []byte, av *idx.AnyValue, c *stringCompactor) []byte {
	if av == nil {
		return buf
	}

	switch v := av.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		buf = appendTag(buf, fieldAnyValueStringValueRef, wireVarint)
		buf = appendVarint(buf, uint64(c.remap(v.StringValueRef)))

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
				elemSize := sizeIdxAnyValueCompacting(elem, c)
				arraySize += sizeTag(fieldArrayValueValues, wireLengthDelim) + sizeVarint(uint64(elemSize)) + elemSize
			}

			// Write directly
			buf = appendTag(buf, fieldAnyValueArrayValue, wireLengthDelim)
			buf = appendVarint(buf, uint64(arraySize))
			for _, elem := range v.ArrayValue.Values {
				elemSize := sizeIdxAnyValueCompacting(elem, c)
				buf = appendTag(buf, fieldArrayValueValues, wireLengthDelim)
				buf = appendVarint(buf, uint64(elemSize))
				buf = appendIdxAnyValueCompacting(buf, elem, c)
			}
		}

	case *idx.AnyValue_KeyValueList:
		if v.KeyValueList != nil {
			// Calculate kvList size
			kvListSize := 0
			for _, kv := range v.KeyValueList.KeyValues {
				if kv != nil {
					remappedKey := c.remap(kv.Key)
					valueSize := sizeIdxAnyValueCompacting(kv.Value, c)
					kvSize := sizeTag(fieldKeyValueKey, wireVarint) + sizeVarint(uint64(remappedKey))
					kvSize += sizeTag(fieldKeyValueValue, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize
					kvListSize += sizeTag(fieldKeyValueListKeyValues, wireLengthDelim) + sizeVarint(uint64(kvSize)) + kvSize
				}
			}

			// Write directly
			buf = appendTag(buf, fieldAnyValueKeyValueList, wireLengthDelim)
			buf = appendVarint(buf, uint64(kvListSize))
			for _, kv := range v.KeyValueList.KeyValues {
				if kv != nil {
					remappedKey := c.remap(kv.Key)
					valueSize := sizeIdxAnyValueCompacting(kv.Value, c)
					kvSize := sizeTag(fieldKeyValueKey, wireVarint) + sizeVarint(uint64(remappedKey))
					kvSize += sizeTag(fieldKeyValueValue, wireLengthDelim) + sizeVarint(uint64(valueSize)) + valueSize

					buf = appendTag(buf, fieldKeyValueListKeyValues, wireLengthDelim)
					buf = appendVarint(buf, uint64(kvSize))
					buf = appendTag(buf, fieldKeyValueKey, wireVarint)
					buf = appendVarint(buf, uint64(remappedKey))
					buf = appendTag(buf, fieldKeyValueValue, wireLengthDelim)
					buf = appendVarint(buf, uint64(valueSize))
					buf = appendIdxAnyValueCompacting(buf, kv.Value, c)
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
