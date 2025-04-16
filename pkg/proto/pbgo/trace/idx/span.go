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
	var numSpanFields uint32
	numSpanFields, bts, err = limitedReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span fields header")
		return
	}
	for numSpanFields > 0 {
		numSpanFields--
		var fieldNum uint32
		fieldNum, bts, err = msgp.ReadUint32Bytes(bts)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read a span field")
			return
		}
		switch fieldNum {
		case 1:
			var service uint32
			service, bts, err = UnmarshalStreamingString(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span service")
				return
			}
			span.ServiceRef = service
		case 2:
			var name uint32
			name, bts, err = UnmarshalStreamingString(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span name")
				return
			}
			span.NameRef = name

		case 3:
			var resc uint32
			resc, bts, err = UnmarshalStreamingString(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span resource")
				return
			}
			span.ResourceRef = resc
		case 4:
			var spanID uint64
			spanID, bts, err = msgp.ReadUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span spanID")
				return
			}
			span.SpanID = spanID
		case 5:
			var parentID uint64
			parentID, bts, err = msgp.ReadUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span parentID")
				return
			}
			span.ParentID = parentID
		case 6:
			var start uint64
			start, bts, err = msgp.ReadUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span start")
				return
			}
			span.Start = start
		case 7:
			var duration uint64
			duration, bts, err = msgp.ReadUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span duration")
				return
			}
			span.Duration = duration
		case 8:
			var spanError bool
			spanError, bts, err = msgp.ReadBoolBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span error")
				return
			}
			span.Error = spanError
		case 9:
			var kvl map[uint32]*AnyValue
			kvl, bts, err = UnmarshalKeyValueMap(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span attributes")
				return
			}
			span.Attributes = kvl
		case 10:
			var typ uint32
			typ, bts, err = msgp.ReadUint32Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span type")
				return
			}
			span.TypeRef = typ
		case 11:
			var spanLinks []*InternalSpanLink
			spanLinks, bts, err = UnmarshalSpanLinks(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span links")
				return
			}
			span.SpanLinks = spanLinks
		case 12:
			var spanEvents []*InternalSpanEvent
			spanEvents, bts, err = UnmarshalSpanEventList(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span events")
				return
			}
			span.SpanEvents = spanEvents
		case 13:
			var env uint32
			env, bts, err = UnmarshalStreamingString(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span env")
				return
			}
			span.EnvRef = env
		case 14:
			var version uint32
			version, bts, err = UnmarshalStreamingString(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span version")
				return
			}
			span.VersionRef = version
		case 15:
			var component uint32
			component, bts, err = UnmarshalStreamingString(bts, span.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span component")
				return
			}
			span.ComponentRef = component
		case 16:
			var kind uint32
			kind, bts, err = msgp.ReadUint32Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span kind")
				return
			}
			span.Kind = SpanKind(kind)
		default:
		}
	}
	return
}

// UnmarshalSpanEventList unmarshals a list of SpanEvents from a byte stream, updating the strings slice with new strings
func UnmarshalSpanEventList(bts []byte, strings *StringTable) (spanEvents []*InternalSpanEvent, o []byte, err error) {
	var numSpanEvents uint32
	numSpanEvents, o, err = limitedReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span event list header")
		return
	}
	spanEvents = make([]*InternalSpanEvent, numSpanEvents)
	for i := range spanEvents {
		spanEvents[i] = &InternalSpanEvent{Strings: strings}
		o, err = spanEvents[i].UnmarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, fmt.Sprintf("Failed to read span event %d", i))
			return
		}
	}
	return
}

// UnmarshalMsg unmarshals a SpanEvent from a byte stream, updating the strings slice with new strings
func (spanEvent *InternalSpanEvent) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var numSpanEventFields uint32
	numSpanEventFields, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span event fields header")
		return
	}
	for numSpanEventFields > 0 {
		numSpanEventFields--
		var fieldNum uint32
		fieldNum, bts, err = msgp.ReadUint32Bytes(bts)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read a span event field")
			return
		}
		switch fieldNum {
		case 1:
			var time uint64
			time, bts, err = msgp.ReadUint64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span event time")
				return
			}
			spanEvent.Time = time
		case 2:
			var name uint32
			name, bts, err = UnmarshalStreamingString(bts, spanEvent.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span event name")
				return
			}
			spanEvent.NameRef = name
		case 3:
			var kvl map[uint32]*AnyValue
			kvl, bts, err = UnmarshalKeyValueMap(bts, spanEvent.Strings)
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
	mfalse  uint8 = 0xc2
	mtrue   uint8 = 0xc3
	mfixstr uint8 = 0xa0
	mstr8   uint8 = 0xd9
	mstr16  uint8 = 0xda
	mstr32  uint8 = 0xdb
	muint8  uint8 = 0xcc
	muint16 uint8 = 0xcd
	muint32 uint8 = 0xce
	muint64 uint8 = 0xcf
	mint8   uint8 = 0xd0
	mint16  uint8 = 0xd1
	mint32  uint8 = 0xd2
	mint64  uint8 = 0xd3
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
func UnmarshalSpanLinks(bts []byte, strings *StringTable) (links []*InternalSpanLink, o []byte, err error) {
	var numLinks uint32
	numLinks, o, err = limitedReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read span links header")
		return
	}
	links = make([]*InternalSpanLink, numLinks)
	for i := range links {
		links[i] = &InternalSpanLink{Strings: strings}
		o, err = links[i].UnmarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, fmt.Sprintf("Failed to read span link %d", i))
			return
		}
	}
	return
}

// UnmarshalMsg unmarshals a SpanLink from a byte stream, updating the strings slice with new strings
func (link *InternalSpanLink) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
			link.TraceID, o, err = msgp.ReadBytesBytes(o, nil)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace ID")
				return
			}
		case 2: // spanID
			link.SpanID, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read span ID")
				return
			}
		case 3: // attributes
			link.Attributes, o, err = UnmarshalKeyValueMap(o, link.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read attributes")
				return
			}
		case 4: // tracestate
			link.TracestateRef, o, err = UnmarshalStreamingString(o, link.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracestate")
				return
			}
		case 5: // flags
			link.FlagsRef, o, err = msgp.ReadUint32Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read flags")
				return
			}
		default:
		}
	}
	return
}
