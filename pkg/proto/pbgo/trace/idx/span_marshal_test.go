// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
)

// The functions and tests below are provided to enable fuzz testing of the span Unmarshal functions
// The trace agent does not have a use for Marshaling to messagepack so this code is kept separately to avoid bloating the main codebase

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
func (link *InternalSpanLink) MarshalMsg(bts []byte, serStrings *SerializedStrings) (o []byte, err error) {
	o = msgp.AppendMapHeader(bts, 5)
	o = msgp.AppendUint32(o, 1) // traceID
	o = msgp.AppendBytes(o, link.TraceID)
	o = msgp.AppendUint32(o, 2) // spanID
	o = msgp.AppendUint64(o, link.SpanID)
	o = msgp.AppendUint32(o, 3) // attributes
	o, err = MarshalAttributesMap(o, link.Attributes, link.Strings, serStrings)
	if err != nil {
		err = msgp.WrapError(err, "Failed to marshal attributes")
		return
	}
	o = msgp.AppendUint32(o, 4) // tracestate
	o = serStrings.AppendStreamingString(link.Strings.Get(link.TracestateRef), link.TracestateRef, o)
	o = msgp.AppendUint32(o, 5) // flags
	o = msgp.AppendUint32(o, link.FlagsRef)
	return
}

// MarshalMsg marshals a SpanEvent into a byte stream
func (evt *InternalSpanEvent) MarshalMsg(bts []byte, serStrings *SerializedStrings) (o []byte, err error) {
	o = msgp.AppendMapHeader(bts, 3)
	o = msgp.AppendUint32(o, 1) // time
	o = msgp.AppendUint64(o, evt.Time)
	o = msgp.AppendUint32(o, 2) // name
	o = serStrings.AppendStreamingString(evt.Strings.Get(evt.NameRef), evt.NameRef, o)
	o = msgp.AppendUint32(o, 3) // attributes
	o, err = MarshalAttributesMap(o, evt.Attributes, evt.Strings, serStrings)
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
	if span.ServiceRef != 0 {
		numFields++
	}
	if span.NameRef != 0 {
		numFields++
	}
	if span.ResourceRef != 0 {
		numFields++
	}
	if span.SpanID != 0 {
		numFields++
	}
	if span.ParentID != 0 {
		numFields++
	}
	if span.Start != 0 {
		numFields++
	}
	if span.Duration != 0 {
		numFields++
	}
	if span.Error {
		numFields++
	}
	if len(span.Attributes) > 0 {
		numFields++
	}
	if span.TypeRef != 0 {
		numFields++
	}
	if len(span.SpanLinks) > 0 {
		numFields++
	}
	if len(span.SpanEvents) > 0 {
		numFields++
	}
	if span.EnvRef != 0 {
		numFields++
	}
	if span.VersionRef != 0 {
		numFields++
	}
	if span.ComponentRef != 0 {
		numFields++
	}
	if span.Kind != 0 {
		numFields++
	}
	o = msgp.AppendMapHeader(bts, uint32(numFields))
	if span.ServiceRef != 0 {
		o = msgp.AppendUint32(o, 1) // service
		o = serStrings.AppendStreamingString(span.Strings.Get(span.ServiceRef), span.ServiceRef, o)
	}
	if span.NameRef != 0 {
		o = msgp.AppendUint32(o, 2) // name
		o = serStrings.AppendStreamingString(span.Strings.Get(span.NameRef), span.NameRef, o)
	}
	if span.ResourceRef != 0 {
		o = msgp.AppendUint32(o, 3) // resource
		o = serStrings.AppendStreamingString(span.Strings.Get(span.ResourceRef), span.ResourceRef, o)
	}
	if span.SpanID != 0 {
		o = msgp.AppendUint32(o, 4) // spanID
		o = msgp.AppendUint64(o, span.SpanID)
	}
	if span.ParentID != 0 {
		o = msgp.AppendUint32(o, 5) // parentID
		o = msgp.AppendUint64(o, span.ParentID)
	}
	if span.Start != 0 {
		o = msgp.AppendUint32(o, 6) // start
		o = msgp.AppendUint64(o, span.Start)
	}
	if span.Duration != 0 {
		o = msgp.AppendUint32(o, 7) // duration
		o = msgp.AppendUint64(o, span.Duration)
	}
	if span.Error {
		o = msgp.AppendUint32(o, 8) // error
		o = msgp.AppendBool(o, span.Error)
	}
	if len(span.Attributes) > 0 {
		o = msgp.AppendUint32(o, 9) // attributes
		o, err = MarshalAttributesMap(o, span.Attributes, span.Strings, serStrings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to marshal attributes")
			return
		}
	}
	if span.TypeRef != 0 {
		o = msgp.AppendUint32(o, 10) // type
		o = msgp.AppendUint32(o, span.TypeRef)
	}
	if len(span.SpanLinks) > 0 {
		o = msgp.AppendUint32(o, 11) // span links
		o = msgp.AppendArrayHeader(o, uint32(len(span.SpanLinks)))
		for _, link := range span.SpanLinks {
			o, err = link.MarshalMsg(o, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal span link")
				return
			}
		}
	}
	if len(span.SpanEvents) > 0 {
		o = msgp.AppendUint32(o, 12) // span events
		o = msgp.AppendArrayHeader(o, uint32(len(span.SpanEvents)))
		for _, event := range span.SpanEvents {
			o, err = event.MarshalMsg(o, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal span event")
				return
			}
		}
	}
	if span.EnvRef != 0 {
		o = msgp.AppendUint32(o, 13) // env
		o = serStrings.AppendStreamingString(span.Strings.Get(span.EnvRef), span.EnvRef, o)
	}
	if span.VersionRef != 0 {
		o = msgp.AppendUint32(o, 14) // version
		o = serStrings.AppendStreamingString(span.Strings.Get(span.VersionRef), span.VersionRef, o)
	}
	if span.ComponentRef != 0 {
		o = msgp.AppendUint32(o, 15) // component
		o = serStrings.AppendStreamingString(span.Strings.Get(span.ComponentRef), span.ComponentRef, o)
	}
	if span.Kind != 0 {
		o = msgp.AppendUint32(o, 16) // kind
		o = msgp.AppendUint32(o, uint32(span.Kind))
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
		// String is already serialized, write the index
		index := s.strIndexes[strTableIndex]
		b = msgp.AppendUint32(b, index)
	}
	return b
}

func TestMarshalSpan(t *testing.T) {
	t.Run("valid span", func(t *testing.T) {
		// From RFC example
		strings := NewStringTable()

		span := &InternalSpan{
			Strings:     strings,
			ServiceRef:  strings.Add("my-service"),
			NameRef:     strings.Add("span-name"),
			ResourceRef: strings.Add("GET /res"),
			SpanID:      12345678,
			Attributes: map[uint32]*AnyValue{
				strings.Add("foo"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("bar")}},
			},
		}
		bts, err := span.MarshalMsg(nil, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)
		expectedBts := []byte{0x85, 0x01, 0xAA} //fixmap 5 elements, 1st key (service), fixstr length 10
		expectedBts = append(expectedBts, []byte("my-service")...)
		expectedBts = append(expectedBts, []byte{0x02, 0xA9}...) //2nd key (name), fixstr length 9
		expectedBts = append(expectedBts, []byte("span-name")...)
		expectedBts = append(expectedBts, []byte{0x03, 0xA8}...) //3rd key (resource), fixstr length 8
		expectedBts = append(expectedBts, []byte("GET /res")...)
		expectedBts = append(expectedBts, []byte{0x04, 0xCE, 0x00, 0xBC, 0x61, 0x4E}...) //4th key (spanID), uint32 header, 12345678
		expectedBts = append(expectedBts, []byte{0x09, 0x93}...)                         //9th key (Attributes), array header 3 elements
		expectedBts = append(expectedBts, []byte{0xA3}...)                               //fixstr length 3
		expectedBts = append(expectedBts, []byte("foo")...)
		expectedBts = append(expectedBts, []byte{0x01, 0xA3}...) //1 string value type, fixstr length 3
		expectedBts = append(expectedBts, []byte("bar")...)
		assert.Equal(t, expectedBts, bts)
	})
}

// FuzzUnmarshalTracerPayload is a fuzz test for UnmarshalTracerPayload
// It generates arbitrary inputs to ensure we do not panic on decoding
func FuzzUnmarshalTracerPayload(f *testing.F) {
	bts := []byte{0x89, 0x02} // map header 9 elements, 2 key (container ID)
	bts = msgp.AppendString(bts, "cidcid")
	bts = append(bts, []byte{0x03}...) // 3 key (Language Name), string of length 2
	bts = msgp.AppendString(bts, "go")
	bts = append(bts, []byte{0x04}...) // 4 key (Language Version), string of length 4
	bts = msgp.AppendString(bts, "1.24")
	bts = append(bts, []byte{0x05}...) // 5 key (Tracer Version), string of length 6
	bts = msgp.AppendString(bts, "v11.24")
	bts = append(bts, []byte{0x06}...) // 6 key (Runtime ID), string of length 10
	bts = msgp.AppendString(bts, "runtime-id")
	bts = append(bts, []byte{0x07}...) // 7 key (Env), string of length 3
	bts = msgp.AppendString(bts, "env")
	bts = append(bts, []byte{0x08}...) // 8 key (Hostname), string of length 6
	bts = msgp.AppendString(bts, "hostname")
	bts = append(bts, []byte{0x09}...) // 9 key (App Version), string of length 6
	bts = msgp.AppendString(bts, "appver")
	bts = append(bts, []byte{0x0A, 0x93, 0x01, 0x04, 0x02}...) // 10 key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2
	f.Add(bts)
	f.Fuzz(func(t *testing.T, data []byte) {
		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		assert.NotPanics(t, func() { tp.UnmarshalMsg(data) })
	})
}

func TestMarshalAnyValue(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		strings := NewStringTable()
		strings.Add("bar")
		serStrings := NewSerializedStrings(2)
		anyValue := &AnyValue{Value: &AnyValue_StringValueRef{StringValueRef: 1}}
		bts, err := anyValue.MarshalMsg(nil, strings, serStrings)
		assert.NoError(t, err)

		expectedBts := []byte{0x1, 0xA3, 0x62, 0x61, 0x72}
		assert.Equal(t, expectedBts, bts)
	})

	t.Run("bool", func(t *testing.T) {
		anyValue := &AnyValue{Value: &AnyValue_BoolValue{BoolValue: true}}
		bts, err := anyValue.MarshalMsg(nil, nil, nil)
		assert.NoError(t, err)

		expectedBts := []byte{0x2, mtrue}
		assert.Equal(t, expectedBts, bts)
	})

	t.Run("double", func(t *testing.T) {
		anyValue := &AnyValue{Value: &AnyValue_DoubleValue{DoubleValue: 3.14}}
		bts, err := anyValue.MarshalMsg(nil, nil, nil)
		assert.NoError(t, err)

		expectedBts := []byte{0x3, 0xcb, 0x40, 0x09, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f}
		assert.Equal(t, expectedBts, bts)
	})

	t.Run("int", func(t *testing.T) {
		anyValue := &AnyValue{Value: &AnyValue_IntValue{IntValue: 7}}
		bts, err := anyValue.MarshalMsg(nil, nil, nil)
		assert.NoError(t, err)

		expectedBts := []byte{0x4, 0x07}
		assert.Equal(t, expectedBts, bts)
	})

	t.Run("bytes", func(t *testing.T) {
		anyValue := &AnyValue{Value: &AnyValue_BytesValue{BytesValue: []byte("foo")}}
		bts, err := anyValue.MarshalMsg(nil, nil, nil)
		assert.NoError(t, err)

		expectedBts := []byte{0x5, 0xc4, 0x3, 0x66, 0x6f, 0x6f}
		assert.Equal(t, expectedBts, bts)
	})

	t.Run("array", func(t *testing.T) {
		anyValue := &AnyValue{Value: &AnyValue_ArrayValue{ArrayValue: &ArrayValue{Values: []*AnyValue{
			{Value: &AnyValue_IntValue{IntValue: 1}},
			{Value: &AnyValue_IntValue{IntValue: 2}},
			{Value: &AnyValue_IntValue{IntValue: 3}},
		}}}}
		bts, err := anyValue.MarshalMsg(nil, nil, nil)
		assert.NoError(t, err)

		expectedBts := []byte{0x6, 0x96, 0x4, 0x1, 0x4, 0x2, 0x4, 0x3}
		assert.Equal(t, expectedBts, bts)
	})

	t.Run("key value list", func(t *testing.T) {
		strings := NewStringTable()
		strings.Add("foo")
		strings.Add("bar")
		serStrings := NewSerializedStrings(uint32(strings.Len()))
		anyValue := &AnyValue{Value: &AnyValue_KeyValueList{KeyValueList: &KeyValueList{KeyValues: []*KeyValue{
			{Key: 1, Value: &AnyValue{Value: &AnyValue_StringValueRef{StringValueRef: 2}}},
		}}}}
		bts, err := anyValue.MarshalMsg(nil, strings, serStrings)
		assert.NoError(t, err)

		expectedBts := []byte{0x7, 0x93, 0xa3, 0x66, 0x6f, 0x6f, 0x1, 0xa3, 0x62, 0x61, 0x72}
		assert.Equal(t, expectedBts, bts)
	})
}

func TestMarshalSpanEvent(t *testing.T) {
	t.Run("valid-event", func(t *testing.T) {
		strings := NewStringTable()
		fooIdx := strings.Add("foo")
		barIdx := strings.Add("bar")
		serStrings := NewSerializedStrings(uint32(strings.Len()))
		event := &InternalSpanEvent{
			Strings: strings,
			Time:    7,
			NameRef: fooIdx,
			Attributes: map[uint32]*AnyValue{
				fooIdx: {Value: &AnyValue_StringValueRef{StringValueRef: barIdx}},
			},
		}
		bts, err := event.MarshalMsg(nil, serStrings)
		assert.NoError(t, err)

		expectedBts := []byte{0x83, 0x01, 0x07, 0x02, 0xA3} // map header 3 elements, 1 key (time), 7 (int64), 2 key (name), string of length 3
		expectedBts = append(expectedBts, []byte("foo")...)
		expectedBts = append(expectedBts, []byte{0x03, 0x93, 0x01, 0x01, 0xA3}...) // 3rd key (attributes), array header 3 elements, fixint 1 (string index), string of length 3
		expectedBts = append(expectedBts, []byte("bar")...)
		assert.Equal(t, expectedBts, bts)
	})
}

func FuzzAnyValueMarshalUnmarshal(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		strings := NewStringTable()
		value, _, err := UnmarshalAnyValue(data, strings)
		if err != nil {
			t.Skip()
		}
		// We got a valid AnyValue, let's marshal it and check if we can unmarshal it back to the same structure
		bts, err := value.MarshalMsg(nil, strings, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)

		strings2 := NewStringTable()
		value2, _, err := UnmarshalAnyValue(bts, strings2)
		assert.NoError(t, err)
		value.assertEqual(t, value2, strings, strings2)
	})
}

func (val *AnyValue) assertEqual(t *testing.T, expected *AnyValue, actualStrings *StringTable, expectedStrings *StringTable) {
	switch v := val.Value.(type) {
	case *AnyValue_StringValueRef:
		actualString := actualStrings.Get(v.StringValueRef)
		expectedString := expectedStrings.Get(expected.Value.(*AnyValue_StringValueRef).StringValueRef)
		assert.Equal(t, expectedString, actualString)
	case *AnyValue_BoolValue:
		assert.Equal(t, expected.Value.(*AnyValue_BoolValue).BoolValue, v.BoolValue)
	case *AnyValue_DoubleValue:
		if math.IsNaN(expected.Value.(*AnyValue_DoubleValue).DoubleValue) && math.IsNaN(v.DoubleValue) {
			return
		}
		assert.InDelta(t, expected.Value.(*AnyValue_DoubleValue).DoubleValue, v.DoubleValue, 0.0001) // Allow for some floating point precision differences
	case *AnyValue_IntValue:
		assert.Equal(t, expected.Value.(*AnyValue_IntValue).IntValue, v.IntValue)
	case *AnyValue_BytesValue:
		assert.Equal(t, expected.Value.(*AnyValue_BytesValue).BytesValue, v.BytesValue)
	case *AnyValue_ArrayValue:
		var expectedArray = expected.Value.(*AnyValue_ArrayValue).ArrayValue
		require.Len(t, v.ArrayValue.Values, len(expectedArray.Values))
		for i, actualSubValue := range v.ArrayValue.Values {
			actualSubValue.assertEqual(t, expectedArray.Values[i], actualStrings, expectedStrings)
		}
	case *AnyValue_KeyValueList:
		var expectedList = expected.Value.(*AnyValue_KeyValueList).KeyValueList
		require.Len(t, v.KeyValueList.KeyValues, len(expectedList.KeyValues))
		for i, actualSubValue := range v.KeyValueList.KeyValues {
			assert.Equal(t, expectedList.KeyValues[i].Key, actualSubValue.Key)
			actualSubValue.Value.assertEqual(t, expectedList.KeyValues[i].Value, actualStrings, expectedStrings)
		}
	default:
		assert.Fail(t, "Unknown AnyValue type")
	}
}

func FuzzSpanLinkMarshalUnmarshal(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		strings := NewStringTable()
		link := &InternalSpanLink{Strings: strings}
		_, err := link.UnmarshalMsg(data)
		if err != nil {
			t.Skip()
		}
		// We got a valid AnyValue, let's marshal it and check if we can unmarshal it back to the same structure
		bts, err := link.MarshalMsg(nil, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)

		strings2 := NewStringTable()
		link2 := &InternalSpanLink{Strings: strings2}
		_, err = link2.UnmarshalMsg(bts)
		assert.NoError(t, err)
		link.assertEqual(t, link2)
	})
}

func (link *InternalSpanLink) assertEqual(t *testing.T, expected *InternalSpanLink) {
	assert.Equal(t, expected.TraceID, link.TraceID)
	assert.Equal(t, expected.SpanID, link.SpanID)
	assert.Len(t, link.Attributes, len(expected.Attributes))
	for k, v := range expected.Attributes {
		// If a key is overwritten in unmarshalling it can result in a different index
		// so we need to lookup the key from the strings table
		expectedKey := expected.Strings.Get(k)
		actualKeyIndex := link.Strings.lookup[expectedKey]
		link.Attributes[actualKeyIndex].assertEqual(t, v, link.Strings, expected.Strings)
	}
	assert.Equal(t, expected.Strings.Get(expected.TracestateRef), link.Strings.Get(link.TracestateRef))
	assert.Equal(t, expected.FlagsRef, link.FlagsRef)
}

func FuzzSpanEventMarshalUnmarshal(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		strings := NewStringTable()
		event := &InternalSpanEvent{Strings: strings}
		_, err := event.UnmarshalMsg(data)
		if err != nil {
			t.Skip()
		}
		bts, err := event.MarshalMsg(nil, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)

		strings2 := NewStringTable()
		event2 := &InternalSpanEvent{Strings: strings2}
		_, err = event2.UnmarshalMsg(bts)
		assert.NoError(t, err)
		event.assertEqual(t, event2)
	})
}

func (evt *InternalSpanEvent) assertEqual(t *testing.T, expected *InternalSpanEvent) {
	assert.Equal(t, expected.Time, evt.Time)
	assert.Equal(t, expected.Strings.Get(expected.NameRef), evt.Strings.Get(evt.NameRef))
	assert.Len(t, evt.Attributes, len(expected.Attributes))
	for k, v := range expected.Attributes {
		// If a key is overwritten in unmarshalling it can result in a different index
		// so we need to lookup the key from the strings table
		expectedKey := expected.Strings.Get(k)
		actualKeyIndex := evt.Strings.lookup[expectedKey]
		evt.Attributes[actualKeyIndex].assertEqual(t, v, evt.Strings, expected.Strings)
	}
}

func FuzzSpanMarshalUnmarshal(f *testing.F) {
	validSpanBts := []byte{0x85, 0x01, 0xAA} //fixmap 5 elements, 1st key (service), fixstr length 10
	validSpanBts = append(validSpanBts, []byte("my-service")...)
	validSpanBts = append(validSpanBts, []byte{0x02, 0xA9}...) //2nd key (name), fixstr length 9
	validSpanBts = append(validSpanBts, []byte("span-name")...)
	validSpanBts = append(validSpanBts, []byte{0x03, 0xA8}...) //3rd key (resource), fixstr length 8
	validSpanBts = append(validSpanBts, []byte("GET /res")...)
	validSpanBts = append(validSpanBts, []byte{0x04, 0xCE, 0x00, 0xBC, 0x61, 0x4E}...) //4th key (spanID), uint32 header, 12345678
	validSpanBts = append(validSpanBts, []byte{0x09, 0x93}...)                         //9th key (Attributes), array header 3 elements
	validSpanBts = append(validSpanBts, []byte{0xA3}...)                               //fixstr length 3
	validSpanBts = append(validSpanBts, []byte("foo")...)
	validSpanBts = append(validSpanBts, []byte{0x01, 0xA3}...) //1 string value type, fixstr length 3
	validSpanBts = append(validSpanBts, []byte("bar")...)
	f.Add(validSpanBts)
	f.Fuzz(func(t *testing.T, data []byte) {
		strings := NewStringTable()
		span := &InternalSpan{Strings: strings}
		_, err := span.UnmarshalMsg(data)
		if err != nil {
			t.Skip()
		}
		bts, err := span.MarshalMsg(nil, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)

		strings2 := NewStringTable()
		span2 := &InternalSpan{Strings: strings2}
		_, err = span2.UnmarshalMsg(bts)
		assert.NoError(t, err)
		span.assertEqual(t, span2)
	})
}

func (span *InternalSpan) assertEqual(t *testing.T, expected *InternalSpan) {
	assert.Equal(t, expected.Strings.Get(expected.ServiceRef), span.Strings.Get(span.ServiceRef))
	assert.Equal(t, expected.Strings.Get(expected.NameRef), span.Strings.Get(span.NameRef))
	assert.Equal(t, expected.Strings.Get(expected.ResourceRef), span.Strings.Get(span.ResourceRef))
	assert.Equal(t, expected.SpanID, span.SpanID)
	assert.Equal(t, expected.ParentID, span.ParentID)
	assert.Equal(t, expected.Start, span.Start)
	assert.Equal(t, expected.Duration, span.Duration)
	assert.Equal(t, expected.Error, span.Error)
	for k, v := range expected.Attributes {
		// If a key is overwritten in unmarshalling it can result in a different index
		// so we need to lookup the key from the strings table
		expectedKey := expected.Strings.Get(k)
		actualKeyIndex := span.Strings.lookup[expectedKey]
		span.Attributes[actualKeyIndex].assertEqual(t, v, span.Strings, expected.Strings)
	}
	for i, link := range expected.SpanLinks {
		span.SpanLinks[i].assertEqual(t, link)
	}
	for i, event := range expected.SpanEvents {
		span.SpanEvents[i].assertEqual(t, event)
	}
	assert.Equal(t, expected.Strings.Get(expected.EnvRef), span.Strings.Get(span.EnvRef))
	assert.Equal(t, expected.Strings.Get(expected.VersionRef), span.Strings.Get(span.VersionRef))
	assert.Equal(t, expected.Strings.Get(expected.ComponentRef), span.Strings.Get(span.ComponentRef))
	assert.Equal(t, expected.Kind, span.Kind)
}
