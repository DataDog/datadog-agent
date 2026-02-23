// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package idx is used to unmarshal v1.0 Trace payloads
package idx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tinylib/msgp/msgp"
)

// UnmarshalSpanList unmarshals a list of InternalSpans from a byte stream, updating the strings slice with new strings
func UnmarshalSpanList(bts []byte, strings *StringTable) (spans []*InternalSpan, o []byte, err error) {
	var numSpans uint32
	numSpans, o, err = limitedReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span list header")
		return
	}
	spans = make([]*InternalSpan, numSpans)
	for i := range spans {
		spans[i] = &InternalSpan{Strings: strings}
		o, err = spans[i].UnmarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, fmt.Sprintf("Failed to read span %d", i))
			return
		}
	}
	return
}

// UnmarshalMsg unmarshals the wire representation of a Span from a byte stream, updating the strings slice with new strings
// directly into an InternalSpan. Note that the Strings field of the InternalSpan must already be initialized.
func (s *InternalSpan) UnmarshalMsg(bts []byte) (o []byte, err error) {
	if s.span == nil {
		s.span = &Span{}
	}
	var numSpanFields uint32
	numSpanFields, o, err = limitedReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span fields header")
		return
	}
	for numSpanFields > 0 {
		numSpanFields--
		var fieldNum uint32
		fieldNum, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read a span field")
			return
		}
		switch fieldNum {
		case 1:
			var service uint32
			service, o, err = UnmarshalStreamingString(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span service")
				return
			}
			s.span.ServiceRef = service
		case 2:
			var name uint32
			name, o, err = UnmarshalStreamingString(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span name")
				return
			}
			s.span.NameRef = name
		case 3:
			var resc uint32
			resc, o, err = UnmarshalStreamingString(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span resource")
				return
			}
			s.span.ResourceRef = resc
		case 4:
			var spanID uint64
			spanID, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span spanID")
				return
			}
			s.span.SpanID = spanID
		case 5:
			var parentID uint64
			parentID, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span parentID")
				return
			}
			s.span.ParentID = parentID
		case 6:
			var start uint64
			start, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span start")
				return
			}
			s.span.Start = start
		case 7:
			var duration uint64
			duration, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span duration")
				return
			}
			s.span.Duration = duration
		case 8:
			var spanError bool
			spanError, o, err = msgp.ReadBoolBytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span error")
				return
			}
			s.span.Error = spanError
		case 9:
			var kvl map[uint32]*AnyValue
			kvl, o, err = UnmarshalKeyValueMap(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span attributes")
				return
			}
			s.span.Attributes = kvl
		case 10:
			var typ uint32
			typ, o, err = UnmarshalStreamingString(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span type")
				return
			}
			s.span.TypeRef = typ
		case 11:
			var spanLinks []*SpanLink
			spanLinks, o, err = UnmarshalSpanLinks(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span links")
				return
			}
			s.span.Links = spanLinks
		case 12:
			var spanEvents []*SpanEvent
			spanEvents, o, err = UnmarshalSpanEventList(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span events")
				return
			}
			s.span.Events = spanEvents
		case 13:
			var env uint32
			env, o, err = UnmarshalStreamingString(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span env")
				return
			}
			s.span.EnvRef = env
		case 14:
			var version uint32
			version, o, err = UnmarshalStreamingString(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span version")
				return
			}
			s.span.VersionRef = version
		case 15:
			var component uint32
			component, o, err = UnmarshalStreamingString(o, s.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span component")
				return
			}
			s.span.ComponentRef = component
		case 16:
			var kind uint32
			kind, o, err = msgp.ReadUint32Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span kind")
				return
			}
			s.span.Kind = SpanKind(kind)
		default:
		}
	}
	return
}

// UnmarshalSpanEventList unmarshals a list of SpanEvents from a byte stream, updating the strings slice with new strings
func UnmarshalSpanEventList(bts []byte, strings *StringTable) (spanEvents []*SpanEvent, o []byte, err error) {
	var numSpanEvents uint32
	numSpanEvents, o, err = limitedReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span event list header")
		return
	}
	spanEvents = make([]*SpanEvent, numSpanEvents)
	for i := range spanEvents {
		spanEvents[i] = &SpanEvent{}
		o, err = spanEvents[i].UnmarshalMsg(o, strings)
		if err != nil {
			err = msgp.WrapError(err, fmt.Sprintf("Failed to read span event %d", i))
			return
		}
	}
	return
}

// UnmarshalMsg unmarshals a SpanEvent from a byte stream, updating the strings slice with new strings
func (spanEvent *SpanEvent) UnmarshalMsg(bts []byte, strings *StringTable) (o []byte, err error) {
	var numSpanEventFields uint32
	numSpanEventFields, o, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span event fields header")
		return
	}
	for numSpanEventFields > 0 {
		numSpanEventFields--
		var fieldNum uint32
		fieldNum, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read a span event field")
			return
		}
		switch fieldNum {
		case 1:
			var time uint64
			time, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span event time")
				return
			}
			spanEvent.Time = time
		case 2:
			var name uint32
			name, o, err = UnmarshalStreamingString(o, strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span event name")
				return
			}
			spanEvent.NameRef = name
		case 3:
			var kvl map[uint32]*AnyValue
			kvl, o, err = UnmarshalKeyValueMap(o, strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span event attributes")
				return
			}
			spanEvent.Attributes = kvl
		default:
		}
	}
	return
}

// UnmarshalKeyValueMap unmarshals a map of key-value pairs from the byte stream, updating the StringTable with new strings
func UnmarshalKeyValueMap(bts []byte, strings *StringTable) (kvl map[uint32]*AnyValue, o []byte, err error) {
	var numAttributes uint32
	numAttributes, o, err = limitedReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span attributes header")
		return
	}
	if numAttributes > 0 && numAttributes%3 != 0 {
		err = fmt.Errorf("Invalid number of span attributes %d - must be a multiple of 3", numAttributes)
		return
	}
	kvl = make(map[uint32]*AnyValue, numAttributes/3)
	var i uint32
	for i < numAttributes {
		var key uint32
		key, o, err = UnmarshalStreamingString(o, strings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read attribute key")
			return
		}
		var value *AnyValue
		value, o, err = UnmarshalAnyValue(o, strings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read attribute value")
			return
		}
		kvl[key] = value
		i += 3
	}
	return
}

// UnmarshalKeyValueList unmarshals a list of key-value pairs from the byte stream, updating the StringTable with new strings
func UnmarshalKeyValueList(bts []byte, strings *StringTable) (kvl []*KeyValue, o []byte, err error) {
	var numAttributes uint32
	numAttributes, o, err = limitedReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span attributes header")
		return
	}
	if numAttributes > 0 && numAttributes%3 != 0 {
		err = fmt.Errorf("Invalid number of span attributes %d - must be a multiple of 3", numAttributes)
		return
	}
	kvl = make([]*KeyValue, numAttributes/3)
	var i uint32
	for i < numAttributes {
		var key uint32
		key, o, err = UnmarshalStreamingString(o, strings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read attribute key")
			return
		}
		var value *AnyValue
		value, o, err = UnmarshalAnyValue(o, strings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read attribute value")
			return
		}
		kvl[i/3] = &KeyValue{Key: key, Value: value}
		i += 3
	}
	return
}

// UnmarshalAnyValue unmarshals an AnyValue from a byte stream, updating the strings slice with new strings
func UnmarshalAnyValue(bts []byte, strings *StringTable) (value *AnyValue, o []byte, err error) {
	value = &AnyValue{}
	var valueType uint32
	valueType, o, err = msgp.ReadUint32Bytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read attribute value type")
		return
	}
	switch valueType {
	case 1:
		var strValue uint32
		strValue, o, err = UnmarshalStreamingString(o, strings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read string attribute value")
			return
		}
		value.Value = &AnyValue_StringValueRef{StringValueRef: strValue}
	case 2: // boolValue
		var boolValue bool
		boolValue, o, err = msgp.ReadBoolBytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read bool attribute value")
			return
		}
		value.Value = &AnyValue_BoolValue{BoolValue: boolValue}
	case 3: // doubleValue
		var doubleValue float64
		doubleValue, o, err = msgp.ReadFloat64Bytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read double attribute value")
			return
		}
		value.Value = &AnyValue_DoubleValue{DoubleValue: doubleValue}
	case 4: // intValue
		var intValue int64
		intValue, o, err = msgp.ReadInt64Bytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read int attribute value")
			return
		}
		value.Value = &AnyValue_IntValue{IntValue: intValue}
	case 5: // bytesValue
		var bytesValue []byte
		bytesValue, o, err = msgp.ReadBytesBytes(o, nil)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read bytes attribute value")
			return
		}
		value.Value = &AnyValue_BytesValue{BytesValue: bytesValue}
	case 6: // arrayValue
		var numElements uint32
		numElements, o, err = limitedReadArrayHeaderBytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read array header")
			return
		}
		if numElements%2 != 0 {
			err = fmt.Errorf("Invalid number of array elements %d - should be 2 elements per AnyValue", numElements)
			return
		}
		arrayValue := make([]*AnyValue, numElements/2)
		var i uint32
		for i < numElements {
			var elemValue *AnyValue
			elemValue, o, err = UnmarshalAnyValue(o, strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read array element")
				return
			}
			arrayValue[i/2] = elemValue
			i += 2
		}
		value.Value = &AnyValue_ArrayValue{ArrayValue: &ArrayValue{Values: arrayValue}}
	case 7: // keyValueList
		var kvl []*KeyValue
		kvl, o, err = UnmarshalKeyValueList(o, strings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read keyValueList")
			return
		}
		value.Value = &AnyValue_KeyValueList{KeyValueList: &KeyValueList{KeyValues: kvl}}
	default:
		err = fmt.Errorf("Unknown anyvalue type %d", valueType)
		return
	}
	return
}

// UnmarshalStreamingString unmarshals a streaming string from a byte stream, updating the strings slice with new strings
// For streaming string details see pkg/trace/api/version.go for details
func UnmarshalStreamingString(bts []byte, strings *StringTable) (index uint32, o []byte, err error) {
	if len(bts) < 1 {
		err = errors.New("Expected streaming string but EOF")
		return
	}
	if isString(bts) {
		var s string
		s, o, err = msgp.ReadStringBytes(bts)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read streaming string as a string")
			return
		}
		index = strings.Add(s)
	} else {
		index, o, err = msgp.ReadUint32Bytes(bts)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read streaming string, failed to read uint32")
			return
		}
		if int(index) >= strings.Len() {
			err = fmt.Errorf("Streaming string referenced an unseen string index %d (string table length: %d)", index, strings.Len())
			return
		}
	}
	return
}

// Helper functions for msgp deserialization
const (
	first3        = 0xe0
	mtrue   uint8 = 0xc3
	mfixstr uint8 = 0xa0
	mstr8   uint8 = 0xd9
	mstr16  uint8 = 0xda
	mstr32  uint8 = 0xdb
)

func isString(bts []byte) bool {
	if isfixstr(bts[0]) {
		return true
	}
	switch bts[0] {
	case mstr8, mstr16, mstr32:
		return true
	default:
		return false
	}
}

func isfixstr(b byte) bool {
	return b&first3 == mfixstr
}

// UnmarshalSpanLinks unmarshals a list of SpanLinks from a byte stream, updating the strings slice with new strings
func UnmarshalSpanLinks(bts []byte, strings *StringTable) (links []*SpanLink, o []byte, err error) {
	var numLinks uint32
	numLinks, o, err = limitedReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span links header")
		return
	}
	links = make([]*SpanLink, numLinks)
	for i := range links {
		links[i] = &SpanLink{}
		o, err = links[i].UnmarshalMsg(o, strings)
		if err != nil {
			err = msgp.WrapError(err, fmt.Sprintf("Failed to read span link %d", i))
			return
		}
	}
	return
}

// UnmarshalMsg unmarshals a SpanLink from a byte stream, updating the strings slice with new strings
func (sl *SpanLink) UnmarshalMsg(bts []byte, strings *StringTable) (o []byte, err error) {
	var numFields uint32
	numFields, o, err = limitedReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span link fields header")
		return
	}
	for numFields > 0 {
		numFields--
		var fieldNum uint32
		fieldNum, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read span link field")
			return
		}
		switch fieldNum {
		case 1: // traceID
			sl.TraceID, o, err = msgp.ReadBytesBytes(o, nil)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace ID")
				return
			}
		case 2: // spanID
			sl.SpanID, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span ID")
				return
			}
		case 3: // attributes
			sl.Attributes, o, err = UnmarshalKeyValueMap(o, strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read attributes")
				return
			}
		case 4: // tracestate
			sl.TracestateRef, o, err = UnmarshalStreamingString(o, strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracestate")
				return
			}
		case 5: // flags
			sl.Flags, o, err = msgp.ReadUint32Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read flags")
				return
			}
		default:
		}
	}
	return
}

// MarshalAttributesMap marshals a map of attributes into a byte stream
func MarshalAttributesMap(bts []byte, attributes map[uint32]*AnyValue, strings *StringTable, serStrings *SerializedStrings) (o []byte, err error) {
	o = msgp.AppendArrayHeader(bts, uint32(len(attributes)*3)) // 3 entries per key value (key, type of value, value)
	for k, v := range attributes {
		o = serStrings.AppendStreamingString(strings.Get(k), k, o)
		o, err = v.MarshalMsg(o, strings, serStrings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to marshal attribute value")
			return
		}
	}
	return
}

// Msgsize returns the size of the message when serialized.
func (av *AnyValue) Msgsize() int {
	size := msgp.Uint32Size // For the type
	switch v := av.Value.(type) {
	case *AnyValue_StringValueRef:
		size += msgp.Uint32Size
	case *AnyValue_BoolValue:
		size += msgp.BoolSize
	case *AnyValue_DoubleValue:
		size += msgp.Float64Size
	case *AnyValue_IntValue:
		size += msgp.Int64Size
	case *AnyValue_BytesValue:
		size += msgp.BytesPrefixSize + len(v.BytesValue)
	case *AnyValue_ArrayValue:
		size += msgp.ArrayHeaderSize
		for _, value := range v.ArrayValue.Values {
			size += value.Msgsize()
		}
	case *AnyValue_KeyValueList:
		for _, value := range v.KeyValueList.KeyValues {
			size += msgp.ArrayHeaderSize                    // Each KV is an array of 3 elements: (key, type of value, value)
			size += msgp.Uint32Size + value.Value.Msgsize() // Key size + Value size (includes type)
		}
	}
	return size
}

// MarshalMsg marshals an AnyValue into a byte stream
func (av *AnyValue) MarshalMsg(bts []byte, strings *StringTable, serStrings *SerializedStrings) ([]byte, error) {
	var err error
	switch v := av.Value.(type) {
	case *AnyValue_StringValueRef:
		bts = msgp.AppendUint32(bts, 1) // write the type
		bts = serStrings.AppendStreamingString(strings.Get(v.StringValueRef), v.StringValueRef, bts)
	case *AnyValue_BoolValue:
		bts = msgp.AppendUint32(bts, 2) // write the type
		bts = msgp.AppendBool(bts, v.BoolValue)
	case *AnyValue_DoubleValue:
		bts = msgp.AppendUint32(bts, 3) // write the type
		bts = msgp.AppendFloat64(bts, v.DoubleValue)
	case *AnyValue_IntValue:
		bts = msgp.AppendUint32(bts, 4) // write the type
		bts = msgp.AppendInt64(bts, v.IntValue)
	case *AnyValue_BytesValue:
		bts = msgp.AppendUint32(bts, 5) // write the type
		bts = msgp.AppendBytes(bts, v.BytesValue)
	case *AnyValue_ArrayValue:
		bts = msgp.AppendUint32(bts, 6) // write the type
		bts = msgp.AppendArrayHeader(bts, uint32(len(v.ArrayValue.Values)*2))
		for _, value := range v.ArrayValue.Values {
			bts, err = value.MarshalMsg(bts, strings, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal array element")
				return bts, err
			}
		}
	case *AnyValue_KeyValueList:
		bts = msgp.AppendUint32(bts, 7)                                            // write the type
		bts = msgp.AppendArrayHeader(bts, uint32(len(v.KeyValueList.KeyValues)*3)) // 3 entries per key value (key, type of value, value)
		for _, value := range v.KeyValueList.KeyValues {
			bts, err = value.MarshalMsg(bts, strings, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal key value list element")
				return bts, err
			}
		}
	}
	return bts, nil
}

// MarshalMsg marshals a KeyValue into a byte stream
func (kv *KeyValue) MarshalMsg(bts []byte, strings *StringTable, serStrings *SerializedStrings) (o []byte, err error) {
	o = serStrings.AppendStreamingString(strings.Get(kv.Key), kv.Key, bts)
	o, err = kv.Value.MarshalMsg(o, strings, serStrings)
	if err != nil {
		err = msgp.WrapError(err, "Failed to marshal key value")
		return
	}
	return
}

// MarshalMsg marshals a SpanLink into a byte stream
func (sl *SpanLink) MarshalMsg(bts []byte, strings *StringTable, serStrings *SerializedStrings) (o []byte, err error) {
	o = msgp.AppendMapHeader(bts, 5)
	o = msgp.AppendUint32(o, 1) // traceID
	o = msgp.AppendBytes(o, sl.TraceID)
	o = msgp.AppendUint32(o, 2) // spanID
	o = msgp.AppendUint64(o, sl.SpanID)
	o = msgp.AppendUint32(o, 3) // attributes
	o, err = MarshalAttributesMap(o, sl.Attributes, strings, serStrings)
	if err != nil {
		err = msgp.WrapError(err, "Failed to marshal attributes")
		return
	}
	o = msgp.AppendUint32(o, 4) // tracestate
	o = serStrings.AppendStreamingString(strings.Get(sl.TracestateRef), sl.TracestateRef, o)
	o = msgp.AppendUint32(o, 5) // flags
	o = msgp.AppendUint32(o, sl.Flags)
	return
}

// MarshalMsg marshals a SpanEvent into a byte stream
func (spanEvent *SpanEvent) MarshalMsg(bts []byte, strings *StringTable, serStrings *SerializedStrings) (o []byte, err error) {
	o = msgp.AppendMapHeader(bts, 3)
	o = msgp.AppendUint32(o, 1) // time
	o = msgp.AppendUint64(o, spanEvent.Time)
	o = msgp.AppendUint32(o, 2) // name
	o = serStrings.AppendStreamingString(strings.Get(spanEvent.NameRef), spanEvent.NameRef, o)
	o = msgp.AppendUint32(o, 3) // attributes
	o, err = MarshalAttributesMap(o, spanEvent.Attributes, strings, serStrings)
	if err != nil {
		err = msgp.WrapError(err, "Failed to marshal attributes")
		return
	}
	return
}

// MarshalMsg marshals a Span into a byte stream
func (s *InternalSpan) MarshalMsg(bts []byte, serStrings *SerializedStrings) (o []byte, err error) {
	// Count non-default fields to determine map header size
	numFields := 0
	if s.span.ServiceRef != 0 {
		numFields++
	}
	if s.span.NameRef != 0 {
		numFields++
	}
	if s.span.ResourceRef != 0 {
		numFields++
	}
	if s.span.SpanID != 0 {
		numFields++
	}
	if s.span.ParentID != 0 {
		numFields++
	}
	if s.span.Start != 0 {
		numFields++
	}
	if s.span.Duration != 0 {
		numFields++
	}
	if s.span.Error {
		numFields++
	}
	if len(s.span.Attributes) > 0 {
		numFields++
	}
	if s.span.TypeRef != 0 {
		numFields++
	}
	if len(s.span.Links) > 0 {
		numFields++
	}
	if len(s.span.Events) > 0 {
		numFields++
	}
	if s.span.EnvRef != 0 {
		numFields++
	}
	if s.span.VersionRef != 0 {
		numFields++
	}
	if s.span.ComponentRef != 0 {
		numFields++
	}
	if s.span.Kind != 0 {
		numFields++
	}
	o = msgp.AppendMapHeader(bts, uint32(numFields))
	if s.span.ServiceRef != 0 {
		o = msgp.AppendUint32(o, 1) // service
		o = serStrings.AppendStreamingString(s.Strings.Get(s.span.ServiceRef), s.span.ServiceRef, o)
	}
	if s.span.NameRef != 0 {
		o = msgp.AppendUint32(o, 2) // name
		o = serStrings.AppendStreamingString(s.Strings.Get(s.span.NameRef), s.span.NameRef, o)
	}
	if s.span.ResourceRef != 0 {
		o = msgp.AppendUint32(o, 3) // resource
		o = serStrings.AppendStreamingString(s.Strings.Get(s.span.ResourceRef), s.span.ResourceRef, o)
	}
	if s.span.SpanID != 0 {
		o = msgp.AppendUint32(o, 4) // spanID
		o = msgp.AppendUint64(o, s.span.SpanID)
	}
	if s.span.ParentID != 0 {
		o = msgp.AppendUint32(o, 5) // parentID
		o = msgp.AppendUint64(o, s.span.ParentID)
	}
	if s.span.Start != 0 {
		o = msgp.AppendUint32(o, 6) // start
		o = msgp.AppendUint64(o, s.span.Start)
	}
	if s.span.Duration != 0 {
		o = msgp.AppendUint32(o, 7) // duration
		o = msgp.AppendUint64(o, s.span.Duration)
	}
	if s.span.Error {
		o = msgp.AppendUint32(o, 8) // error
		o = msgp.AppendBool(o, s.span.Error)
	}
	if len(s.span.Attributes) > 0 {
		o = msgp.AppendUint32(o, 9) // attributes
		o, err = MarshalAttributesMap(o, s.span.Attributes, s.Strings, serStrings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to marshal attributes")
			return
		}
	}
	if s.span.TypeRef != 0 {
		o = msgp.AppendUint32(o, 10) // type
		o = serStrings.AppendStreamingString(s.Strings.Get(s.span.TypeRef), s.span.TypeRef, o)
	}
	if len(s.span.Links) > 0 {
		o = msgp.AppendUint32(o, 11) // span links
		o = msgp.AppendArrayHeader(o, uint32(len(s.span.Links)))
		for _, link := range s.span.Links {
			o, err = link.MarshalMsg(o, s.Strings, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal span link")
				return
			}
		}
	}
	if len(s.span.Events) > 0 {
		o = msgp.AppendUint32(o, 12) // span events
		o = msgp.AppendArrayHeader(o, uint32(len(s.span.Events)))
		for _, event := range s.span.Events {
			o, err = event.MarshalMsg(o, s.Strings, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal span event")
				return
			}
		}
	}
	if s.span.EnvRef != 0 {
		o = msgp.AppendUint32(o, 13) // env
		o = serStrings.AppendStreamingString(s.Strings.Get(s.span.EnvRef), s.span.EnvRef, o)
	}
	if s.span.VersionRef != 0 {
		o = msgp.AppendUint32(o, 14) // version
		o = serStrings.AppendStreamingString(s.Strings.Get(s.span.VersionRef), s.span.VersionRef, o)
	}
	if s.span.ComponentRef != 0 {
		o = msgp.AppendUint32(o, 15) // component
		o = serStrings.AppendStreamingString(s.Strings.Get(s.span.ComponentRef), s.span.ComponentRef, o)
	}
	if s.span.Kind != 0 {
		o = msgp.AppendUint32(o, 16) // kind
		o = msgp.AppendUint32(o, uint32(s.span.Kind))
	}
	return
}

// SerializedStrings is a helper type that tracks what strings have been serialized and where
// It is only good for one serialization
type SerializedStrings struct {
	strIndexes []uint32
	curIndex   uint32
}

// NewSerializedStrings creates a new SerializedStrings object used to track what strings have been serialized
// numStrings is the number of strings that will be serialized
func NewSerializedStrings(numStrings uint32) *SerializedStrings {
	return &SerializedStrings{strIndexes: make([]uint32, numStrings), curIndex: 1} // index starts at 1 as "" is reserved at 0
}

// AppendStreamingString writes str to b if it hasn't been written before, otherwise it writes the serialization index
// strTableIndex is the location of str in the string table - this is used to track which strings have been written already
func (s *SerializedStrings) AppendStreamingString(str string, strTableIndex uint32, b []byte) []byte {
	if s.strIndexes[strTableIndex] == 0 && str != "" {
		// String is not yet serialized, serialize it
		b = msgp.AppendString(b, str)
		s.strIndexes[strTableIndex] = s.curIndex
		s.curIndex++
	} else {
		// TODO better names
		// String is already serialized, write the index
		index := s.strIndexes[strTableIndex]
		b = msgp.AppendUint32(b, index)
	}
	return b
}

// SpanConvertedFields is used to collect fields from v4 spans that have been promoted to the chunk level
// Use NewSpanConvertedFields to create a new SpanConvertedFields object with the correct initial values
type SpanConvertedFields struct {
	TraceIDLower      uint64 // the lower 64 bits of the trace ID
	TraceIDUpper      uint64 // the upper 64 bits of the trace ID
	EnvRef            uint32 // the env reference
	AppVersionRef     uint32 // the app version reference
	GitCommitShaRef   uint32 // the git commit sha reference
	SamplingMechanism uint32 // the sampling mechanism
	HostnameRef       uint32 // the hostname reference
	SamplingPriority  int32  // the sampling priority, initialized to sampler.PriorityNone (math.MinInt8)
	OriginRef         uint32 // the origin reference
	APMModeRef        uint32 // the APM mode reference
}

// NewSpanConvertedFields creates a new SpanConvertedFields object used to collect fields from v4 spans that have been promoted to the chunk level
func NewSpanConvertedFields() *SpanConvertedFields {
	return &SpanConvertedFields{
		SamplingPriority: math.MinInt8,
	}
}

// ChunkConvertedFields is used to collect fields from v4 chunks that have been promoted to the tracer payload level
type ChunkConvertedFields struct {
	EnvRef          uint32
	APMModeRef      uint32
	HostnameRef     uint32
	AppVersionRef   uint32
	GitCommitShaRef uint32
}

// ApplyPromotedFields applies chunk promoted fields to the tracer payload
func (tp *InternalTracerPayload) ApplyPromotedFields(convertedFields *ChunkConvertedFields) {
	if tp.envRef == 0 {
		tp.envRef = convertedFields.EnvRef
	}
	if tp.hostnameRef == 0 {
		tp.hostnameRef = convertedFields.HostnameRef
	}
	if tp.appVersionRef == 0 {
		tp.appVersionRef = convertedFields.AppVersionRef
	}
	if convertedFields.GitCommitShaRef != 0 {
		tp.setStringRefAttribute("_dd.git.commit.sha", convertedFields.GitCommitShaRef)
	}
	if convertedFields.APMModeRef != 0 {
		tp.setStringRefAttribute("_dd.apm_mode", convertedFields.APMModeRef)
	}
}

// UnmarshalMsgConverted unmarshals a list of list(sic) v4 spans directly into an InternalTracerPayload for efficiency
func (tp *InternalTracerPayload) UnmarshalMsgConverted(bts []byte) (o []byte, err error) {
	if tp.Attributes == nil {
		tp.Attributes = make(map[uint32]*AnyValue, 1)
	}
	if tp.Strings == nil {
		tp.Strings = NewStringTableWithCapacity(1024) // 1024 is a good default capacity
	}
	var numChunks uint32
	numChunks, o, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	if cap(tp.Chunks) >= int(numChunks) {
		tp.Chunks = tp.Chunks[:numChunks]
	} else {
		tp.Chunks = make([]*InternalTraceChunk, numChunks)
	}
	chunkConvertedFields := ChunkConvertedFields{}
	for i := range tp.Chunks {
		tp.Chunks[i] = &InternalTraceChunk{Strings: tp.Strings}
		o, err = tp.Chunks[i].UnmarshalMsgConverted(o, &chunkConvertedFields)
		if err != nil {
			err = msgp.WrapError(err, i)
			return
		}
	}
	tp.ApplyPromotedFields(&chunkConvertedFields)
	o = bts
	return
}

// ApplyPromotedFields applies span promoted fields to the chunk, copying chunk promoted fields to chunkConvertedFields
func (c *InternalTraceChunk) ApplyPromotedFields(convertedFields *SpanConvertedFields, chunkConvertedFields *ChunkConvertedFields) {
	tid := make([]byte, 16)
	binary.BigEndian.PutUint64(tid[8:], convertedFields.TraceIDLower)
	binary.BigEndian.PutUint64(tid[:8], convertedFields.TraceIDUpper)
	c.TraceID = tid
	c.samplingMechanism = convertedFields.SamplingMechanism
	c.Priority = int32(convertedFields.SamplingPriority)
	if convertedFields.OriginRef != 0 {
		c.originRef = convertedFields.OriginRef
	}
	if convertedFields.APMModeRef != 0 {
		chunkConvertedFields.APMModeRef = convertedFields.APMModeRef
	}
	if convertedFields.EnvRef != 0 {
		chunkConvertedFields.EnvRef = convertedFields.EnvRef
	}
	if convertedFields.HostnameRef != 0 {
		chunkConvertedFields.HostnameRef = convertedFields.HostnameRef
	}
	if convertedFields.AppVersionRef != 0 {
		chunkConvertedFields.AppVersionRef = convertedFields.AppVersionRef
	}
	if convertedFields.GitCommitShaRef != 0 {
		chunkConvertedFields.GitCommitShaRef = convertedFields.GitCommitShaRef
	}
}

// UnmarshalMsgConverted unmarshals a list of v4 spans directly into an InternalTraceChunk for efficiency
// The provided InternalTraceChunk must have a non-nil Strings field
func (c *InternalTraceChunk) UnmarshalMsgConverted(bts []byte, chunkConvertedFields *ChunkConvertedFields) (o []byte, err error) {
	var numSpans uint32
	numSpans, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	if cap(c.Spans) >= int(numSpans) {
		c.Spans = c.Spans[:numSpans]
	} else {
		c.Spans = make([]*InternalSpan, numSpans)
	}
	convertedFields := NewSpanConvertedFields()
	for i := range c.Spans {
		if msgp.IsNil(bts) {
			bts, err = msgp.ReadNilBytes(bts)
			if err != nil {
				return
			}
			c.Spans[i] = nil
		} else {
			c.Spans[i] = NewInternalSpan(c.Strings, &Span{})
			c.Spans[i].SetSpanKind(SpanKind_SPAN_KIND_INTERNAL) // default to internal span kind
			bts, err = c.Spans[i].UnmarshalMsgConverted(bts, convertedFields)
			if err != nil {
				err = msgp.WrapError(err, i)
				return
			}
		}
	}
	c.ApplyPromotedFields(convertedFields, chunkConvertedFields)
	o = bts
	return
}

// UnmarshalMsgConverted unmarshals a v4 span directly into an InternalSpan for efficiency
// The provided InternalSpan must have a non-nil Strings field
func (s *InternalSpan) UnmarshalMsgConverted(bts []byte, convertedFields *SpanConvertedFields) (o []byte, err error) {
	var field []byte
	_ = field
	var numFields uint32
	numFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for numFields > 0 {
		numFields--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "service":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.ServiceRef = 0
				break
			}
			s.span.ServiceRef, bts, err = parseStringBytesRef(s.Strings, bts)
			if err != nil {
				err = msgp.WrapError(err, "Service")
				return
			}
		case "name":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.NameRef = 0
				break
			}
			s.span.NameRef, bts, err = parseStringBytesRef(s.Strings, bts)
			if err != nil {
				err = msgp.WrapError(err, "Service")
				return
			}
		case "resource":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.ResourceRef = 0
				break
			}
			s.span.ResourceRef, bts, err = parseStringBytesRef(s.Strings, bts)
			if err != nil {
				err = msgp.WrapError(err, "Service")
				return
			}
		case "trace_id":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				if convertedFields.TraceIDLower != 0 {
					err = errors.New("already found lower 64 bits of trace ID but found a 0 traceID")
					break
				}
				break
			}
			convertedFields.TraceIDLower, bts, err = parseUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "TraceID")
				return
			}
		case "span_id":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.SpanID = 0
				break
			}
			s.span.SpanID, bts, err = parseUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "SpanID")
				return
			}
		case "parent_id":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.ParentID = 0
				break
			}
			s.span.ParentID, bts, err = parseUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "ParentID")
				return
			}
		case "start":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.Start = 0
				break
			}
			var spanStart int64
			spanStart, bts, err = parseInt64Bytes(bts)
			s.span.Start = uint64(spanStart)
			if err != nil {
				err = msgp.WrapError(err, "Start")
				return
			}
		case "duration":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.Duration = 0
				break
			}
			var spanDuration int64
			spanDuration, bts, err = parseInt64Bytes(bts)
			s.span.Duration = uint64(spanDuration)
			if err != nil {
				err = msgp.WrapError(err, "Duration")
				return
			}
		case "error":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.Error = false
				break
			}
			var spanError int32
			spanError, bts, err = parseInt32Bytes(bts)
			s.span.Error = spanError != 0
			if err != nil {
				err = msgp.WrapError(err, "Error")
				return
			}
		case "meta":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				break
			}
			var numMetaFields uint32
			numMetaFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
			if err != nil {
				err = msgp.WrapError(err, "Meta")
				return
			}
			if s.span.Attributes == nil && numMetaFields > 0 {
				s.span.Attributes = make(map[uint32]*AnyValue, numMetaFields)
			}
			for numMetaFields > 0 {
				var metaVal uint32
				numMetaFields--
				var metaKey uint32
				metaKey, bts, err = parseStringBytesRef(s.Strings, bts)
				if err != nil {
					err = msgp.WrapError(err, "Meta")
					return
				}
				metaVal, bts, err = parseStringBytesRef(s.Strings, bts)
				if err != nil {
					err = msgp.WrapError(err, "Meta", metaKey)
					return
				}
				s.handlePromotedMetaFields(metaKey, metaVal, convertedFields)
				s.span.Attributes[metaKey] = &AnyValue{
					Value: &AnyValue_StringValueRef{
						StringValueRef: metaVal,
					},
				}
			}
		case "metrics":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				break
			}
			var numMetricsFields uint32
			numMetricsFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
			if err != nil {
				err = msgp.WrapError(err, "Metrics")
				return
			}
			if s.span.Attributes == nil && numMetricsFields > 0 {
				s.span.Attributes = make(map[uint32]*AnyValue, numMetricsFields)
			}
			for numMetricsFields > 0 {
				var value float64
				numMetricsFields--
				var key uint32
				key, bts, err = parseStringBytesRef(s.Strings, bts)
				if err != nil {
					err = msgp.WrapError(err, "Metrics")
					return
				}
				value, bts, err = parseFloat64Bytes(bts)
				if err != nil {
					err = msgp.WrapError(err, "Metrics", key)
					return
				}
				s.handlePromotedMetricsFields(key, value, convertedFields)
				s.span.Attributes[key] = &AnyValue{
					Value: &AnyValue_DoubleValue{
						DoubleValue: value,
					},
				}
			}
		case "type":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				s.span.TypeRef = 0
				break
			}
			s.span.TypeRef, bts, err = parseStringBytesRef(s.Strings, bts)
			if err != nil {
				err = msgp.WrapError(err, "Type")
				return
			}
		case "meta_struct":
			var numMetaStructFields uint32
			numMetaStructFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
			if err != nil {
				err = msgp.WrapError(err, "MetaStruct")
				return
			}
			if s.span.Attributes == nil && numMetaStructFields > 0 {
				s.span.Attributes = make(map[uint32]*AnyValue, numMetaStructFields)
			}
			for numMetaStructFields > 0 {
				var value []byte
				numMetaStructFields--
				var key uint32
				key, bts, err = parseStringBytesRef(s.Strings, bts)
				if err != nil {
					err = msgp.WrapError(err, "MetaStruct")
					return
				}
				value, bts, err = msgp.ReadBytesBytes(bts, value)
				if err != nil {
					err = msgp.WrapError(err, "MetaStruct", key)
					return
				}
				s.span.Attributes[key] = &AnyValue{
					Value: &AnyValue_BytesValue{
						BytesValue: value,
					},
				}
			}
		case "span_links":
			var numSpanLinks uint32
			numSpanLinks, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
			if err != nil {
				err = msgp.WrapError(err, "SpanLinks")
				return
			}
			if cap(s.span.Links) >= int(numSpanLinks) {
				s.span.Links = (s.span.Links)[:numSpanLinks]
			} else {
				s.span.Links = make([]*SpanLink, numSpanLinks)
			}
			for i := range s.span.Links {
				if msgp.IsNil(bts) {
					bts, err = msgp.ReadNilBytes(bts)
					if err != nil {
						return
					}
					s.span.Links[i] = nil
				} else {
					if s.span.Links[i] == nil {
						s.span.Links[i] = new(SpanLink)
					}
					bts, err = s.span.Links[i].UnmarshalMsgConverted(s.Strings, bts)
					if err != nil {
						err = msgp.WrapError(err, "SpanLinks", i)
						return
					}
				}
			}
		case "span_events":
			var numEvents uint32
			numEvents, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
			if err != nil {
				err = msgp.WrapError(err, "SpanEvents")
				return
			}
			if cap(s.span.Events) >= int(numEvents) {
				s.span.Events = (s.span.Events)[:numEvents]
			} else {
				s.span.Events = make([]*SpanEvent, numEvents)
			}
			for i := range s.span.Events {
				if msgp.IsNil(bts) {
					bts, err = msgp.ReadNilBytes(bts)
					if err != nil {
						return
					}
					s.span.Events[i] = nil
				} else {
					if s.span.Events[i] == nil {
						s.span.Events[i] = new(SpanEvent)
					}
					bts, err = s.span.Events[i].UnmarshalMsgConverted(s.Strings, bts)
					if err != nil {
						err = msgp.WrapError(err, "SpanEvents", i)
						return
					}
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	s.SetStringAttribute("_dd.convertedv1", "v04")
	o = bts
	return
}

// UnmarshalMsgConverted unmarshals a v4 span event directly into an idx.SpanEvent for efficiency
func (spanEvent *SpanEvent) UnmarshalMsgConverted(strings *StringTable, bts []byte) (o []byte, err error) { //nolint:receiver-naming
	var field []byte
	_ = field
	var numFields uint32
	numFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for numFields > 0 {
		numFields--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "time_unix_nano":
			spanEvent.Time, bts, err = msgp.ReadUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "TimeUnixNano")
				return
			}
		case "name":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				break
			}
			spanEvent.NameRef, bts, err = parseStringBytesRef(strings, bts)
			if err != nil {
				err = msgp.WrapError(err, "Name")
				return
			}
		case "attributes":
			var numAttributes uint32
			numAttributes, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
			if err != nil {
				err = msgp.WrapError(err, "Attributes")
				return
			}
			if spanEvent.Attributes == nil {
				spanEvent.Attributes = make(map[uint32]*AnyValue, numAttributes)
			}
			for numAttributes > 0 {
				var value *AnyValue
				numAttributes--
				var keyRef uint32
				keyRef, bts, err = parseStringBytesRef(strings, bts)
				if err != nil {
					err = msgp.WrapError(err, "Attributes")
					return
				}
				if msgp.IsNil(bts) {
					bts, err = msgp.ReadNilBytes(bts)
					if err != nil {
						return
					}
					value = nil
				} else {
					value = new(AnyValue)
					bts, err = value.UnmarshalMsgConverted(strings, bts)
					if err != nil {
						err = msgp.WrapError(err, "Attributes", keyRef)
						return
					}
				}
				spanEvent.Attributes[keyRef] = value
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// UnmarshalMsgConverted unmarshals a v4 any value directly into an idx.AnyValue for efficiency
func (av *AnyValue) UnmarshalMsgConverted(strings *StringTable, bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var numFields uint32
	numFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	var valueType int32
	var strValueRef uint32
	var boolValue bool
	var intValue int64
	var doubleValue float64
	var arrayValue []*AnyValue
	for numFields > 0 {
		numFields--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "type":
			{
				valueType, bts, err = parseInt32Bytes(bts)
				if err != nil {
					err = msgp.WrapError(err, "Type")
					return
				}
			}
		case "string_value":
			strValueRef, bts, err = parseStringBytesRef(strings, bts)
			if err != nil {
				err = msgp.WrapError(err, "StringValue")
				return
			}
		case "bool_value":
			boolValue, bts, err = msgp.ReadBoolBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "BoolValue")
				return
			}
		case "int_value":
			intValue, bts, err = msgp.ReadInt64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "IntValue")
				return
			}
		case "double_value":
			doubleValue, bts, err = msgp.ReadFloat64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "DoubleValue")
				return
			}
		case "array_value":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				if err != nil {
					return
				}
				arrayValue = nil
			} else {
				var numArrayFields uint32
				numArrayFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
				if err != nil {
					err = msgp.WrapError(err, "ArrayValue")
					return
				}
				for numArrayFields > 0 {
					numArrayFields--
					field, bts, err = msgp.ReadMapKeyZC(bts)
					if err != nil {
						err = msgp.WrapError(err, "ArrayValue")
						return
					}
					switch msgp.UnsafeString(field) {
					case "values":
						var numArrayElems uint32
						numArrayElems, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
						if err != nil {
							err = msgp.WrapError(err, "ArrayValue", "Values")
							return
						}
						if cap(arrayValue) >= int(numArrayElems) {
							arrayValue = (arrayValue)[:numArrayElems]
						} else {
							arrayValue = make([]*AnyValue, numArrayElems)
						}
						for i := range arrayValue {
							if msgp.IsNil(bts) {
								bts, err = msgp.ReadNilBytes(bts)
								if err != nil {
									return
								}
								arrayValue[i] = nil
							} else {
								if arrayValue[i] == nil {
									arrayValue[i] = new(AnyValue)
								}
								bts, err = arrayValue[i].UnmarshalMsgConverted(strings, bts)
								if err != nil {
									err = msgp.WrapError(err, "ArrayValue", "Values", i)
									return
								}
							}
						}
					default:
						bts, err = msgp.Skip(bts)
						if err != nil {
							err = msgp.WrapError(err, "ArrayValue")
							return
						}
					}
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	switch valueType {
	case stringValueType:
		av.Value = &AnyValue_StringValueRef{
			StringValueRef: strValueRef,
		}
	case boolValueType:
		av.Value = &AnyValue_BoolValue{
			BoolValue: boolValue,
		}
	case intValueType:
		av.Value = &AnyValue_IntValue{
			IntValue: intValue,
		}
	case doubleValueType:
		av.Value = &AnyValue_DoubleValue{
			DoubleValue: doubleValue,
		}
	case arrayValueType:
		av.Value = &AnyValue_ArrayValue{
			ArrayValue: &ArrayValue{
				Values: arrayValue,
			},
		}
	}
	o = bts
	return
}

const (
	stringValueType int32 = 0
	boolValueType   int32 = 1
	intValueType    int32 = 2
	doubleValueType int32 = 3
	arrayValueType  int32 = 4
)

// UnmarshalMsgConverted unmarshals a v4 span link directly into an idx.SpanLink for efficiency
func (sl *SpanLink) UnmarshalMsgConverted(strings *StringTable, bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var numFields uint32
	numFields, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	var traceIDLower, traceIDUpper uint64
	for numFields > 0 {
		numFields--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "trace_id":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				break
			}
			traceIDLower, bts, err = parseUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "TraceID")
				return
			}
		case "trace_id_high":
			traceIDUpper, bts, err = msgp.ReadUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "TraceIDHigh")
				return
			}
		case "span_id":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				break
			}
			sl.SpanID, bts, err = parseUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "SpanID")
				return
			}
		case "attributes":
			var numAttributes uint32
			numAttributes, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
			if err != nil {
				err = msgp.WrapError(err, "Attributes")
				return
			}
			if sl.Attributes == nil {
				sl.Attributes = make(map[uint32]*AnyValue, numAttributes)
			}
			for numAttributes > 0 {
				var valueRef uint32
				numAttributes--
				var keyRef uint32
				keyRef, bts, err = parseStringBytesRef(strings, bts)
				if err != nil {
					err = msgp.WrapError(err, "Attributes")
					return
				}
				valueRef, bts, err = parseStringBytesRef(strings, bts)
				if err != nil {
					err = msgp.WrapError(err, "Attributes", keyRef)
					return
				}
				sl.Attributes[keyRef] = &AnyValue{
					Value: &AnyValue_StringValueRef{
						StringValueRef: valueRef,
					},
				}
			}
		case "tracestate":
			sl.TracestateRef, bts, err = parseStringBytesRef(strings, bts)
			if err != nil {
				err = msgp.WrapError(err, "Tracestate")
				return
			}
		case "flags":
			sl.Flags, bts, err = msgp.ReadUint32Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Flags")
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	tid := make([]byte, 16)
	binary.BigEndian.PutUint64(tid[8:], traceIDLower)
	binary.BigEndian.PutUint64(tid[:8], traceIDUpper)
	sl.TraceID = tid
	o = bts
	return
}

// handlePromotedMetricsFields processes promoted metrics fields that have dedicated span fields
// If we fail to parse a value we don't use the promoted value, but the original string will still be in the span attributes
func (s *InternalSpan) handlePromotedMetricsFields(key uint32, val float64, convertedFields *SpanConvertedFields) {
	if s.Strings.Get(key) == "_sampling_priority_v1" {
		convertedFields.SamplingPriority = int32(val)
	}
}

// handlePromotedMetaFields processes promoted meta fields that have dedicated span fields
// If we fail to parse a value we don't use the promoted value, but the original string will still be in the span attributes
func (s *InternalSpan) handlePromotedMetaFields(metaKey, metaVal uint32, convertedFields *SpanConvertedFields) {
	switch s.Strings.Get(metaKey) {
	case "_dd.p.tid":
		tidUpper, err := strconv.ParseUint(s.Strings.Get(metaVal), 16, 64)
		if err != nil {
			return
		}
		convertedFields.TraceIDUpper = tidUpper
	case "env":
		s.span.EnvRef = metaVal
		convertedFields.EnvRef = metaVal
	case "version":
		s.span.VersionRef = metaVal
		convertedFields.AppVersionRef = metaVal
	case "component":
		s.span.ComponentRef = metaVal
	case "span.kind":
		kindStr := s.Strings.Get(metaVal)
		switch kindStr {
		case "server":
			s.span.Kind = SpanKind_SPAN_KIND_SERVER
		case "client":
			s.span.Kind = SpanKind_SPAN_KIND_CLIENT
		case "producer":
			s.span.Kind = SpanKind_SPAN_KIND_PRODUCER
		case "consumer":
			s.span.Kind = SpanKind_SPAN_KIND_CONSUMER
		case "internal":
			s.span.Kind = SpanKind_SPAN_KIND_INTERNAL
		default:
			s.span.Kind = SpanKind_SPAN_KIND_INTERNAL
		}
	case "_dd.git.commit.sha":
		convertedFields.GitCommitShaRef = metaVal
	case "_dd.p.dm":
		payloadDecisionMaker := s.Strings.Get(metaVal)
		payloadDecisionMaker, _ = strings.CutPrefix(payloadDecisionMaker, "-")
		var err error
		var samplingMechanism uint64
		samplingMechanism, err = strconv.ParseUint(payloadDecisionMaker, 10, 32)
		if err != nil {
			return
		}
		convertedFields.SamplingMechanism = uint32(samplingMechanism)
	case "_dd.hostname":
		convertedFields.HostnameRef = metaVal
	case "_dd.origin":
		convertedFields.OriginRef = metaVal
	case "_dd.apm_mode":
		convertedFields.APMModeRef = metaVal
	}
}

// parseStringBytes reads the next type in the msgpack payload and
// converts the BinType or the StrType in a valid string returning the index of the string in the string table
func parseStringBytesRef(stringTable *StringTable, bts []byte) (uint32, []byte, error) {
	ref, bts, err := parseStringBytes(bts)
	if err != nil {
		return 0, bts, err
	}
	return stringTable.Add(ref), bts, nil
}

// parseStringBytesRef reads the next type in the msgpack payload and
// converts the BinType or the StrType in a valid string returning the string itself
func parseStringBytes(bts []byte) (string, []byte, error) {
	if msgp.IsNil(bts) {
		bts, err := msgp.ReadNilBytes(bts)
		return "", bts, err
	}
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var (
		err error
		i   []byte
	)
	switch t {
	case msgp.BinType:
		i, bts, err = msgp.ReadBytesZC(bts)
	case msgp.StrType:
		i, bts, err = msgp.ReadStringZC(bts)
	default:
		return "", bts, msgp.TypeError{Encoded: t, Method: msgp.StrType}
	}
	if err != nil {
		return "", bts, err
	}
	if utf8.Valid(i) {
		return string(i), bts, nil
	}
	return repairUTF8(msgp.UnsafeString(i)), bts, nil
}

// repairUTF8 ensures all characters in s are UTF-8 by replacing non-UTF-8 characters
// with the replacement char �
func repairUTF8(s string) string {
	in := strings.NewReader(s)
	var out bytes.Buffer
	out.Grow(len(s))

	for {
		r, _, err := in.ReadRune()
		if err != nil {
			// note: by contract, if `in` contains non-valid utf-8, no error is returned. Rather the utf-8 replacement
			// character is returned. Therefore, the only error should usually be io.EOF indicating end of string.
			// If any other error is returned by chance, we quit as well, outputting whatever part of the string we
			// had already constructed.
			return out.String()
		}
		out.WriteRune(r)
	}
}

// parseUint64Bytes parses an uint64 even if the sent value is an int64;
// this is required because the language used for the encoding library
// may not have unsigned types. An example is early version of Java
// (and so JRuby interpreter) that encodes uint64 as int64:
// http://docs.oracle.com/javase/tutorial/java/nutsandbolts/datatypes.html
func parseUint64Bytes(bts []byte) (uint64, []byte, error) {
	if msgp.IsNil(bts) {
		bts, err := msgp.ReadNilBytes(bts)
		return 0, bts, err
	}
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var (
		i   int64
		u   uint64
		err error
	)
	switch t {
	case msgp.UintType:
		u, bts, err = msgp.ReadUint64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return u, bts, err
	case msgp.IntType:
		i, bts, err = msgp.ReadInt64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return uint64(i), bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// parseInt64Bytes parses an int64 even if the sent value is an uint64;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseInt64Bytes(bts []byte) (int64, []byte, error) {
	if msgp.IsNil(bts) {
		bts, err := msgp.ReadNilBytes(bts)
		return 0, bts, err
	}
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var (
		i   int64
		u   uint64
		err error
	)
	switch t {
	case msgp.IntType:
		i, bts, err = msgp.ReadInt64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return i, bts, nil
	case msgp.UintType:
		u, bts, err = msgp.ReadUint64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		// force-cast
		i, ok := castInt64(u)
		if !ok {
			return 0, bts, errors.New("found uint64, overflows int64")
		}
		return i, bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// cast to int64 values that are int64 but that are sent in uint64
// over the wire. Set to 0 if they overflow the MaxInt64 size. This
// cast should be used ONLY while decoding int64 values that are
// sent as uint64 to reduce the payload size, otherwise the approach
// is not correct in the general sense.
func castInt64(v uint64) (int64, bool) {
	if v > math.MaxInt64 {
		return 0, false
	}
	return int64(v), true
}

// parseInt32Bytes parses an int32 even if the sent value is an uint32;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseInt32Bytes(bts []byte) (int32, []byte, error) {
	if msgp.IsNil(bts) {
		bts, err := msgp.ReadNilBytes(bts)
		return 0, bts, err
	}
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var (
		i   int32
		u   uint32
		err error
	)
	switch t {
	case msgp.IntType:
		i, bts, err = msgp.ReadInt32Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return i, bts, nil
	case msgp.UintType:
		u, bts, err = msgp.ReadUint32Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		// force-cast
		i, ok := castInt32(u)
		if !ok {
			return 0, bts, errors.New("found uint32, overflows int32")
		}
		return i, bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// cast to int32 values that are int32 but that are sent in uint32
// over the wire. Set to 0 if they overflow the MaxInt32 size. This
// cast should be used ONLY while decoding int32 values that are
// sent as uint32 to reduce the payload size, otherwise the approach
// is not correct in the general sense.
func castInt32(v uint32) (int32, bool) {
	if v > math.MaxInt32 {
		return 0, false
	}
	return int32(v), true
}

// parseFloat64Bytes parses a float64 even if the sent value is an int64 or an uint64;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseFloat64Bytes(bts []byte) (float64, []byte, error) {
	if msgp.IsNil(bts) {
		bts, err := msgp.ReadNilBytes(bts)
		return 0, bts, err
	}
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var err error
	switch t {
	case msgp.IntType:
		var i int64
		i, bts, err = msgp.ReadInt64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		return float64(i), bts, nil
	case msgp.UintType:
		var i uint64
		i, bts, err = msgp.ReadUint64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		return float64(i), bts, nil
	case msgp.Float64Type:
		var f float64
		f, bts, err = msgp.ReadFloat64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		return f, bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.Float64Type}
	}
}
