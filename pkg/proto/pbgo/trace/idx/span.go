// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package idx is used to unmarshal v1.0 Trace payloads
package idx

import (
	"fmt"

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
func (span *InternalSpan) UnmarshalMsg(bts []byte) (o []byte, err error) {
	if span.span == nil {
		span.span = &Span{}
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
			service, o, err = UnmarshalStreamingString(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span service")
				return
			}
			span.span.ServiceRef = service
		case 2:
			var name uint32
			name, o, err = UnmarshalStreamingString(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span name")
				return
			}
			span.span.NameRef = name
		case 3:
			var resc uint32
			resc, o, err = UnmarshalStreamingString(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span resource")
				return
			}
			span.span.ResourceRef = resc
		case 4:
			var spanID uint64
			spanID, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span spanID")
				return
			}
			span.span.SpanID = spanID
		case 5:
			var parentID uint64
			parentID, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span parentID")
				return
			}
			span.span.ParentID = parentID
		case 6:
			var start uint64
			start, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span start")
				return
			}
			span.span.Start = start
		case 7:
			var duration uint64
			duration, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span duration")
				return
			}
			span.span.Duration = duration
		case 8:
			var spanError bool
			spanError, o, err = msgp.ReadBoolBytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span error")
				return
			}
			span.span.Error = spanError
		case 9:
			var kvl map[uint32]*AnyValue
			kvl, o, err = UnmarshalKeyValueMap(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span attributes")
				return
			}
			span.span.Attributes = kvl
		case 10:
			var typ uint32
			typ, o, err = UnmarshalStreamingString(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span type")
				return
			}
			span.span.TypeRef = typ
		case 11:
			var spanLinks []*SpanLink
			spanLinks, o, err = UnmarshalSpanLinks(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span links")
				return
			}
			span.span.Links = spanLinks
		case 12:
			var spanEvents []*SpanEvent
			spanEvents, o, err = UnmarshalSpanEventList(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span events")
				return
			}
			span.span.Events = spanEvents
		case 13:
			var env uint32
			env, o, err = UnmarshalStreamingString(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span env")
				return
			}
			span.span.EnvRef = env
		case 14:
			var version uint32
			version, o, err = UnmarshalStreamingString(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span version")
				return
			}
			span.span.VersionRef = version
		case 15:
			var component uint32
			component, o, err = UnmarshalStreamingString(o, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span component")
				return
			}
			span.span.ComponentRef = component
		case 16:
			var kind uint32
			kind, o, err = msgp.ReadUint32Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span kind")
				return
			}
			span.span.Kind = SpanKind(kind)
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
	numSpanEventFields, o, err = msgp.ReadMapHeaderBytes(bts)
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
		err = msgp.WrapError(err, fmt.Sprintf("Invalid number of span attributes %d - must be a multiple of 3", numAttributes))
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
		err = msgp.WrapError(err, fmt.Sprintf("Invalid number of span attributes %d - must be a multiple of 3", numAttributes))
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
			err = msgp.WrapError(err, "Invalid number of array elements, should be 2 elements per AnyValue")
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
		err = msgp.WrapError(err, fmt.Sprintf("Unknown anyvalue type %d", valueType))
		return
	}
	return
}

// UnmarshalStreamingString unmarshals a streaming string from a byte stream, updating the strings slice with new strings
// For streaming string details see pkg/trace/api/version.go for details
func UnmarshalStreamingString(bts []byte, strings *StringTable) (index uint32, o []byte, err error) {
	if len(bts) < 1 {
		err = msgp.WrapError(err, "Expected streaming string but EOF")
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
			err = msgp.WrapError(err, "Streaming string referenced an unseen string index")
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
func (val *AnyValue) Msgsize() int {
	size := msgp.Uint32Size // For the type
	switch v := val.Value.(type) {
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
func (val *AnyValue) MarshalMsg(bts []byte, strings *StringTable, serStrings *SerializedStrings) ([]byte, error) {
	var err error
	switch v := val.Value.(type) {
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
func (span *InternalSpan) MarshalMsg(bts []byte, serStrings *SerializedStrings) (o []byte, err error) {
	// Count non-default fields to determine map header size
	numFields := 0
	if span.span.ServiceRef != 0 {
		numFields++
	}
	if span.span.NameRef != 0 {
		numFields++
	}
	if span.span.ResourceRef != 0 {
		numFields++
	}
	if span.span.SpanID != 0 {
		numFields++
	}
	if span.span.ParentID != 0 {
		numFields++
	}
	if span.span.Start != 0 {
		numFields++
	}
	if span.span.Duration != 0 {
		numFields++
	}
	if span.span.Error {
		numFields++
	}
	if len(span.span.Attributes) > 0 {
		numFields++
	}
	if span.span.TypeRef != 0 {
		numFields++
	}
	if len(span.span.Links) > 0 {
		numFields++
	}
	if len(span.span.Events) > 0 {
		numFields++
	}
	if span.span.EnvRef != 0 {
		numFields++
	}
	if span.span.VersionRef != 0 {
		numFields++
	}
	if span.span.ComponentRef != 0 {
		numFields++
	}
	if span.span.Kind != 0 {
		numFields++
	}
	o = msgp.AppendMapHeader(bts, uint32(numFields))
	if span.span.ServiceRef != 0 {
		o = msgp.AppendUint32(o, 1) // service
		o = serStrings.AppendStreamingString(span.Strings.Get(span.span.ServiceRef), span.span.ServiceRef, o)
	}
	if span.span.NameRef != 0 {
		o = msgp.AppendUint32(o, 2) // name
		o = serStrings.AppendStreamingString(span.Strings.Get(span.span.NameRef), span.span.NameRef, o)
	}
	if span.span.ResourceRef != 0 {
		o = msgp.AppendUint32(o, 3) // resource
		o = serStrings.AppendStreamingString(span.Strings.Get(span.span.ResourceRef), span.span.ResourceRef, o)
	}
	if span.span.SpanID != 0 {
		o = msgp.AppendUint32(o, 4) // spanID
		o = msgp.AppendUint64(o, span.span.SpanID)
	}
	if span.span.ParentID != 0 {
		o = msgp.AppendUint32(o, 5) // parentID
		o = msgp.AppendUint64(o, span.span.ParentID)
	}
	if span.span.Start != 0 {
		o = msgp.AppendUint32(o, 6) // start
		o = msgp.AppendUint64(o, span.span.Start)
	}
	if span.span.Duration != 0 {
		o = msgp.AppendUint32(o, 7) // duration
		o = msgp.AppendUint64(o, span.span.Duration)
	}
	if span.span.Error {
		o = msgp.AppendUint32(o, 8) // error
		o = msgp.AppendBool(o, span.span.Error)
	}
	if len(span.span.Attributes) > 0 {
		o = msgp.AppendUint32(o, 9) // attributes
		o, err = MarshalAttributesMap(o, span.span.Attributes, span.Strings, serStrings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to marshal attributes")
			return
		}
	}
	if span.span.TypeRef != 0 {
		o = msgp.AppendUint32(o, 10) // type
		o = serStrings.AppendStreamingString(span.Strings.Get(span.span.TypeRef), span.span.TypeRef, o)
	}
	if len(span.span.Links) > 0 {
		o = msgp.AppendUint32(o, 11) // span links
		o = msgp.AppendArrayHeader(o, uint32(len(span.span.Links)))
		for _, link := range span.span.Links {
			o, err = link.MarshalMsg(o, span.Strings, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal span link")
				return
			}
		}
	}
	if len(span.span.Events) > 0 {
		o = msgp.AppendUint32(o, 12) // span events
		o = msgp.AppendArrayHeader(o, uint32(len(span.span.Events)))
		for _, event := range span.span.Events {
			o, err = event.MarshalMsg(o, span.Strings, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal span event")
				return
			}
		}
	}
	if span.span.EnvRef != 0 {
		o = msgp.AppendUint32(o, 13) // env
		o = serStrings.AppendStreamingString(span.Strings.Get(span.span.EnvRef), span.span.EnvRef, o)
	}
	if span.span.VersionRef != 0 {
		o = msgp.AppendUint32(o, 14) // version
		o = serStrings.AppendStreamingString(span.Strings.Get(span.span.VersionRef), span.span.VersionRef, o)
	}
	if span.span.ComponentRef != 0 {
		o = msgp.AppendUint32(o, 15) // component
		o = serStrings.AppendStreamingString(span.Strings.Get(span.span.ComponentRef), span.span.ComponentRef, o)
	}
	if span.span.Kind != 0 {
		o = msgp.AppendUint32(o, 16) // kind
		o = msgp.AppendUint32(o, uint32(span.span.Kind))
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
