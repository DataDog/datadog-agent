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

func TestMarshalSpan(t *testing.T) {
	t.Run("valid span", func(t *testing.T) {
		// From RFC example
		strings := NewStringTable()

		span := &InternalSpan{
			Strings: strings,
			span: &Span{
				ServiceRef:  strings.Add("my-service"),
				NameRef:     strings.Add("span-name"),
				ResourceRef: strings.Add("GET /res"),
				SpanID:      12345678,
				Attributes: map[uint32]*AnyValue{
					strings.Add("foo"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("bar")}},
				},
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

func TestMarshalUnmarshalSpan(t *testing.T) {
	t.Run("valid span", func(t *testing.T) {
		strings := NewStringTable()
		span := &InternalSpan{
			Strings: strings,
			span: &Span{
				ServiceRef:   strings.Add("my-service"),
				NameRef:      strings.Add("span-name"),
				ResourceRef:  strings.Add("GET /res"),
				TypeRef:      strings.Add("someType"),
				EnvRef:       strings.Add("someEnv"),
				VersionRef:   strings.Add("someVersion"),
				ComponentRef: strings.Add("someComponent"),
				Kind:         SpanKind_SPAN_KIND_SERVER,
				ParentID:     444,
				Start:        123,
				Duration:     432,
				Error:        true,
				SpanID:       12345678,
				Attributes: map[uint32]*AnyValue{
					strings.Add("foo"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("bar")}},
				},
			},
		}
		bts, err := span.MarshalMsg(nil, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)
		resultSpan := &InternalSpan{Strings: NewStringTable()}
		_, err = resultSpan.UnmarshalMsg(bts)
		assert.NoError(t, err)
		resultSpan.assertEqual(t, span)
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
			event: &SpanEvent{
				Time:    7,
				NameRef: fooIdx,
				Attributes: map[uint32]*AnyValue{
					fooIdx: {Value: &AnyValue_StringValueRef{StringValueRef: barIdx}},
				},
			},
		}
		bts, err := event.event.MarshalMsg(nil, strings, serStrings)
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

func (av *AnyValue) assertEqual(t *testing.T, expected *AnyValue, actualStrings *StringTable, expectedStrings *StringTable) {
	switch v := av.Value.(type) {
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
			expectedKeyStr := expectedStrings.Get(expectedList.KeyValues[i].Key)
			actualKeyStr := actualStrings.Get(actualSubValue.Key)
			assert.Equal(t, expectedKeyStr, actualKeyStr)
			actualSubValue.Value.assertEqual(t, expectedList.KeyValues[i].Value, actualStrings, expectedStrings)
		}
	default:
		assert.Fail(t, "Unknown AnyValue type")
	}
}

func FuzzSpanLinkMarshalUnmarshal(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		strings := NewStringTable()
		link := &SpanLink{}
		_, err := link.UnmarshalMsg(data, strings)
		if err != nil {
			t.Skip()
		}
		// We got a valid AnyValue, let's marshal it and check if we can unmarshal it back to the same structure
		bts, err := link.MarshalMsg(nil, strings, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)

		strings2 := NewStringTable()
		link2 := &SpanLink{}
		_, err = link2.UnmarshalMsg(bts, strings2)
		assert.NoError(t, err)
		il := &InternalSpanLink{Strings: strings, link: link}
		il2 := &InternalSpanLink{Strings: strings2, link: link2}
		il.assertEqual(t, il2)
	})
}

func (sl *InternalSpanLink) assertEqual(t *testing.T, expected *InternalSpanLink) {
	assert.Equal(t, expected.link.TraceID, sl.link.TraceID)
	assert.Equal(t, expected.link.SpanID, sl.link.SpanID)
	assert.Len(t, sl.link.Attributes, len(expected.link.Attributes))
	for k, v := range expected.link.Attributes {
		// If a key is overwritten in unmarshalling it can result in a different index
		// so we need to lookup the key from the strings table
		expectedKey := expected.Strings.Get(k)
		actualKeyIndex := sl.Strings.lookup[expectedKey]
		sl.link.Attributes[actualKeyIndex].assertEqual(t, v, sl.Strings, expected.Strings)
	}
	assert.Equal(t, expected.Strings.Get(expected.link.TracestateRef), sl.Strings.Get(sl.link.TracestateRef))
	assert.Equal(t, expected.link.Flags, sl.link.Flags)
}

func FuzzSpanEventMarshalUnmarshal(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		strings := NewStringTable()
		event := &SpanEvent{}
		_, err := event.UnmarshalMsg(data, strings)
		if err != nil {
			t.Skip()
		}
		bts, err := event.MarshalMsg(nil, strings, NewSerializedStrings(uint32(strings.Len())))
		assert.NoError(t, err)

		strings2 := NewStringTable()
		event2 := &SpanEvent{}
		_, err = event2.UnmarshalMsg(bts, strings2)
		assert.NoError(t, err)
		ie := &InternalSpanEvent{Strings: strings, event: event}
		ie2 := &InternalSpanEvent{Strings: strings2, event: event2}
		ie.assertEqual(t, ie2)
	})
}

func (evt *InternalSpanEvent) assertEqual(t *testing.T, expected *InternalSpanEvent) {
	assert.Equal(t, expected.event.Time, evt.event.Time)
	assert.Equal(t, expected.Strings.Get(expected.event.NameRef), evt.Strings.Get(evt.event.NameRef))
	assert.Len(t, evt.event.Attributes, len(expected.event.Attributes))
	for k, v := range expected.event.Attributes {
		// If a key is overwritten in unmarshalling it can result in a different index
		// so we need to lookup the key from the strings table
		expectedKey := expected.Strings.Get(k)
		actualKeyIndex := evt.Strings.lookup[expectedKey]
		evt.event.Attributes[actualKeyIndex].assertEqual(t, v, evt.Strings, expected.Strings)
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

func TestSpanMarshalUnmarshal_KeyValueListKeyIndexMismatch(t *testing.T) {
	// Reproduce fuzz failure where KeyValueList keys are compared by index across different string tables
	strings := NewStringTable()
	// Seed string table to create differing indices after re-unmarshal
	svc := strings.Add("svc")
	name := strings.Add("nm")
	res := strings.Add("res")

	// Add KV keys in a specific order to influence original indices
	kvB := strings.Add("kv-b")
	kvA := strings.Add("kv-a")

	// Attribute key
	attrKey := strings.Add("attr")

	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			ServiceRef:  svc,
			NameRef:     name,
			ResourceRef: res,
			SpanID:      1,
			Attributes: map[uint32]*AnyValue{
				attrKey: {Value: &AnyValue_KeyValueList{KeyValueList: &KeyValueList{KeyValues: []*KeyValue{
					{Key: kvA, Value: &AnyValue{Value: &AnyValue_IntValue{IntValue: 1}}},
					{Key: kvB, Value: &AnyValue{Value: &AnyValue_IntValue{IntValue: 2}}},
				}}}},
			},
		},
	}

	bts, err := span.MarshalMsg(nil, NewSerializedStrings(uint32(strings.Len())))
	require.NoError(t, err)

	strings2 := NewStringTable()
	span2 := &InternalSpan{Strings: strings2}
	_, err = span2.UnmarshalMsg(bts)
	require.NoError(t, err)

	// Prior to fix, this assertion fails with a key index mismatch inside KeyValueList
	span.assertEqual(t, span2)
}

func (s *InternalSpan) assertEqual(t *testing.T, expected *InternalSpan) {
	assert.Equal(t, expected.Service(), s.Service())
	assert.Equal(t, expected.Name(), s.Name())
	assert.Equal(t, expected.Resource(), s.Resource())
	assert.Equal(t, expected.span.SpanID, s.span.SpanID)
	assert.Equal(t, expected.span.ParentID, s.span.ParentID)
	assert.Equal(t, expected.span.Start, s.span.Start)
	assert.Equal(t, expected.span.Duration, s.span.Duration)
	assert.Equal(t, expected.span.Error, s.span.Error)
	assert.Len(t, s.span.Attributes, len(expected.span.Attributes))
	for k, v := range expected.span.Attributes {
		// If a key is overwritten in unmarshalling it can result in a different index
		// so we need to lookup the key from the strings table
		expectedKey := expected.Strings.Get(k)
		actualKeyIndex := s.Strings.lookup[expectedKey]
		s.span.Attributes[actualKeyIndex].assertEqual(t, v, s.Strings, expected.Strings)
	}
	for i, link := range expected.Links() {
		s.Links()[i].assertEqual(t, link)
	}
	for i, event := range expected.Events() {
		s.Events()[i].assertEqual(t, event)
	}
	assert.Equal(t, expected.Env(), s.Env())
	assert.Equal(t, expected.Version(), s.Version())
	assert.Equal(t, expected.Component(), s.Component())
	assert.Equal(t, expected.Type(), s.Type())
	assert.Equal(t, expected.span.Kind, s.span.Kind)
}
