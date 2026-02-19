// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package idx is used to unmarshal v1.0 Trace payloads
package idx

import (
	"errors"

	"github.com/tinylib/msgp/msgp"
)

// buildStringTable builds a string table from a list of unique v05 strings
// However, unlike the v05 array, the string table expects that the 0 index is always the empty string
// To get around this we swap whatever string is at index 0 with the location of the empty string (if present), or append it to the end
// We then return the string table and the new location for string ref `0` (0 if unchanged)
func buildStringTable(v05Strings []string) (*StringTable, uint32) {
	if len(v05Strings) == 0 {
		return NewStringTable(), 0
	}
	if v05Strings[0] == "" {
		// The empty string is already at index 0, so we can just use the string table as is
		return StringTableFromArray(v05Strings), 0
	}
	emptyStringIndex := uint32(0)
	for i, str := range v05Strings {
		if str == "" {
			emptyStringIndex = uint32(i)
			break
		}
	}
	newZeroRef := emptyStringIndex
	if emptyStringIndex != 0 {
		v05Strings[0], v05Strings[emptyStringIndex] = v05Strings[emptyStringIndex], v05Strings[0]
	} else {
		// The empty string is not present, so we append the 0th string to the end and set the first index to the empty string
		v05Strings = append(v05Strings, v05Strings[0])
		v05Strings[0] = ""
		newZeroRef = uint32(len(v05Strings) - 1)
	}
	return StringTableFromArray(v05Strings), newZeroRef
}

// UnmarshalMsgDictionary decodes a InternalTracerPayload using the specification from the v0.5 endpoint.
// For details, see the documentation for endpoint v0.5 in pkg/trace/api/version.go
func (tp *InternalTracerPayload) UnmarshalMsgDictionary(bts []byte) error {
	var err error
	if _, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes); err != nil {
		return err
	}
	// read dictionary
	var sz uint32
	if sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes); err != nil {
		return err
	}
	dict := make([]string, sz)
	for i := range dict {
		var str string
		str, bts, err = parseStringBytes(bts)
		if err != nil {
			return err
		}
		dict[i] = str
	}
	stringTable, newZeroRef := buildStringTable(dict)
	tp.Strings = stringTable
	// read num chunks
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
	if err != nil {
		return err
	}
	if cap(tp.Chunks) >= int(sz) {
		tp.Chunks = tp.Chunks[:sz]
	} else {
		tp.Chunks = make([]*InternalTraceChunk, sz)
	}
	chunkConvertedFields := ChunkConvertedFields{}
	for i := range tp.Chunks {
		sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
		if err != nil {
			return err
		}
		if tp.Chunks[i] == nil {
			tp.Chunks[i] = &InternalTraceChunk{Strings: stringTable}
		}
		if cap(tp.Chunks[i].Spans) >= int(sz) {
			tp.Chunks[i].Spans = tp.Chunks[i].Spans[:sz]
		} else {
			tp.Chunks[i].Spans = make([]*InternalSpan, sz)
		}
		convertedFields := NewSpanConvertedFields()
		for j := range tp.Chunks[i].Spans {
			if tp.Chunks[i].Spans[j] == nil {
				tp.Chunks[i].Spans[j] = NewInternalSpan(stringTable, &Span{})
			}
			if bts, err = tp.Chunks[i].Spans[j].UnmarshalMsgDictionaryConverted(bts, convertedFields, newZeroRef); err != nil {
				return err
			}
		}
		tp.Chunks[i].ApplyPromotedFields(convertedFields, &chunkConvertedFields)
	}
	tp.ApplyPromotedFields(&chunkConvertedFields)
	return nil
}

// spanPropertyCount specifies the number of top-level properties that a span
// has.
const spanPropertyCount = 12

func readV05StringRef(newZeroRef uint32, bts []byte) (uint32, []byte, error) {
	var parsedRef uint32
	var err error
	parsedRef, bts, err = msgp.ReadUint32Bytes(bts)
	if err != nil {
		return 0, bts, err
	}
	if parsedRef == 0 && newZeroRef != 0 {
		// This string was moved from index 0 to index newZeroRef, so we return the new index
		return newZeroRef, bts, nil
	}
	return parsedRef, bts, nil
}

// UnmarshalMsgDictionaryConverted decodes a v05 span directly into an InternalSpan, for details, see the documentation for endpoint v0.5
// in pkg/trace/api/version.go
// The provided InternalSpan must have a pre-populated Strings field
// newZeroRef is the new location for string ref `0` (0 if unchanged) see buildStringTable for more details
func (s *InternalSpan) UnmarshalMsgDictionaryConverted(bts []byte, convertedFields *SpanConvertedFields, newZeroRef uint32) ([]byte, error) {
	var (
		sz  uint32
		err error
	)
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
	if err != nil {
		return bts, err
	}
	if sz != spanPropertyCount {
		return bts, errors.New("encoded span needs exactly 12 elements in array")
	}
	// Service (0)
	s.span.ServiceRef, bts, err = readV05StringRef(newZeroRef, bts)
	if err != nil {
		return bts, err
	}
	// Name (1)
	s.span.NameRef, bts, err = readV05StringRef(newZeroRef, bts)
	if err != nil {
		return bts, err
	}
	// Resource (2)
	s.span.ResourceRef, bts, err = readV05StringRef(newZeroRef, bts)
	if err != nil {
		return bts, err
	}
	// TraceID (3)
	convertedFields.TraceIDLower, bts, err = parseUint64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// SpanID (4)
	s.span.SpanID, bts, err = parseUint64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// ParentID (5)
	s.span.ParentID, bts, err = parseUint64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// Start (6)
	var spanStart int64
	spanStart, bts, err = parseInt64Bytes(bts)
	s.span.Start = uint64(spanStart)
	if err != nil {
		return bts, err
	}
	// Duration (7)
	var spanDuration int64
	spanDuration, bts, err = parseInt64Bytes(bts)
	s.span.Duration = uint64(spanDuration)
	if err != nil {
		return bts, err
	}
	// Error (8)
	var spanError int32
	spanError, bts, err = parseInt32Bytes(bts)
	s.span.Error = spanError != 0
	if err != nil {
		return bts, err
	}
	// Meta (9)
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		return bts, err
	}
	if s.span.Attributes == nil && sz > 0 {
		s.span.Attributes = make(map[uint32]*AnyValue, sz)
	}
	for sz > 0 {
		sz--
		var key, val uint32
		key, bts, err = readV05StringRef(newZeroRef, bts)
		if err != nil {
			return bts, err
		}
		val, bts, err = readV05StringRef(newZeroRef, bts)
		if err != nil {
			return bts, err
		}
		s.handlePromotedMetaFields(key, val, convertedFields)
		s.span.Attributes[key] = &AnyValue{
			Value: &AnyValue_StringValueRef{
				StringValueRef: val,
			},
		}
	}
	// Metrics (10)
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		return bts, err
	}
	if s.span.Attributes == nil && sz > 0 {
		s.span.Attributes = make(map[uint32]*AnyValue, sz)
	}
	for sz > 0 {
		sz--
		var (
			key uint32
			val float64
		)
		key, bts, err = readV05StringRef(newZeroRef, bts)
		if err != nil {
			return bts, err
		}
		val, bts, err = parseFloat64Bytes(bts)
		if err != nil {
			return bts, err
		}
		s.handlePromotedMetricsFields(key, val, convertedFields)
		s.span.Attributes[key] = &AnyValue{
			Value: &AnyValue_DoubleValue{
				DoubleValue: val,
			},
		}
	}
	// Type (11)
	s.span.TypeRef, bts, err = readV05StringRef(newZeroRef, bts)
	if err != nil {
		return bts, err
	}
	s.SetStringAttribute("_dd.convertedv1", "v05")
	return bts, nil
}

// safeReadHeaderBytes wraps msgp header readers (typically ReadArrayHeaderBytes and ReadMapHeaderBytes).
// It enforces the dictionary max size of 25MB and protects the caller from making unbounded allocations through `make(any, sz)`.
func safeReadHeaderBytes(b []byte, read func([]byte) (uint32, []byte, error)) (uint32, []byte, error) {
	sz, bts, err := read(b)
	if err != nil {
		return 0, nil, err
	}
	if sz > 25*1e6 {
		// Dictionary can't be larger than 25 MB
		return 0, nil, errors.New("too long payload")
	}
	return sz, bts, err
}
