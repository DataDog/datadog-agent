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

func (s *StringTable) assertEqual(t *testing.T, expected []string) {
	assert.Equal(t, expected, s.strings)
	assert.Len(t, s.lookup, len(expected))
	for i, str := range expected {
		assert.Equal(t, uint32(i), s.lookup[str])
	}
}

func TestUnmarshalTracerPayload(t *testing.T) {
	t.Run("tracer payload no chunks", func(t *testing.T) {
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

		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		o, err := tp.UnmarshalMsg(bts)
		assert.NoError(t, err)
		assert.Len(t, o, 0)
		expectedStrings := []string{"", "cidcid", "go", "1.24", "v11.24", "runtime-id", "env", "hostname", "appver"}

		expectedTP := &InternalTracerPayload{
			Strings:         tp.Strings, // We will assert on this separately for improved readability here
			ContainerID:     1,
			LanguageName:    2,
			LanguageVersion: 3,
			TracerVersion:   4,
			RuntimeID:       5,
			Env:             6,
			Hostname:        7,
			AppVersion:      8,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
		}
		tp.Strings.assertEqual(t, expectedStrings)
		assert.Equal(t, expectedTP, tp)
	})

	t.Run("strings up front", func(t *testing.T) {
		strings := []string{"", "cidcid", "go", "1.24", "v11.24", "runtime-id", "env", "hostname", "appver"}
		bts := []byte{0x8A, 0x01, 0x99} // map header 9 elements, 1 key (strings), array header 9 elements
		for _, v := range strings {
			bts = msgp.AppendString(bts, v)
		}
		bts = append(bts, []byte{0x02, 0x01}...)                   // 2 key (container ID), string index 1
		bts = append(bts, []byte{0x03, 0x02}...)                   // 3 key (Language Name), string index 2
		bts = append(bts, []byte{0x04, 0x03}...)                   // 4 key (Language Version), string index 3
		bts = append(bts, []byte{0x05, 0x04}...)                   // 5 key (Tracer Version), string index 4
		bts = append(bts, []byte{0x06, 0x05}...)                   // 6 key (Runtime ID), string index 5
		bts = append(bts, []byte{0x07, 0x06}...)                   // 7 key (Env), string index 6
		bts = append(bts, []byte{0x08, 0x07}...)                   // 8 key (Hostname), string index 7
		bts = append(bts, []byte{0x09, 0x08}...)                   // 9 key (App Version), string index 8
		bts = append(bts, []byte{0x0A, 0x93, 0x01, 0x04, 0x02}...) // 10 key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2

		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		o, err := tp.UnmarshalMsg(bts)
		assert.NoError(t, err)
		assert.Len(t, o, 0)

		expectedTP := &InternalTracerPayload{
			Strings:         tp.Strings, // We will assert on this separately for improved readability here
			ContainerID:     1,
			LanguageName:    2,
			LanguageVersion: 3,
			TracerVersion:   4,
			RuntimeID:       5,
			Env:             6,
			Hostname:        7,
			AppVersion:      8,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
		}
		tp.Strings.assertEqual(t, strings)
		assert.Equal(t, expectedTP, tp)
	})
}

func TestUnmarshalTraceChunk(t *testing.T) {
	t.Run("trace chunk no spans", func(t *testing.T) {
		strings := NewStringTable()
		bts := []byte{0x91, 0x86, 0x01, 0x02, 0x02, 0xA6}          // array header 1 element, map header 2 elements, 1 key (priority), 2 (int32), 2 key (origin), string of length 6
		bts = append(bts, []byte("lambda")...)                     // lambda bytes
		bts = append(bts, []byte{0x03, 0x93, 0x01, 0x04, 0x02}...) // 3rd key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2
		bts = append(bts, []byte{0x05, mtrue}...)                  // 5th key (droppedTrace), bool true
		bts = append(bts, []byte{0x06, 0xc4, 0x01, 0xAF}...)       // 6th key (TraceID), bin header, 1 byte in length, 0xAF
		bts = append(bts, []byte{0x07, 0xA2}...)                   // 7th key (decisionMaker), string of length 2
		bts = append(bts, []byte("-9")...)

		chunks, o, err := UnmarshalTraceChunkList(bts, strings)
		assert.NoError(t, err)
		assert.Len(t, chunks, 1)
		assert.Len(t, o, 0)

		expectedChunk := &InternalTraceChunk{
			Strings:  strings, // We will assert on this separately for improved readability here
			Priority: 2,
			Origin:   1,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
			DroppedTrace:  true,
			TraceID:       []byte{0xAF},
			DecisionMaker: 2,
		}
		assert.Equal(t, expectedChunk, chunks[0])
		strings.assertEqual(t, []string{"", "lambda", "-9"})
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
		anyValue := &AnyValue{Value: &AnyValue_StringValue{StringValue: 1}}
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
			{Key: 1, Value: &AnyValue{Value: &AnyValue_StringValue{StringValue: 2}}},
		}}}}
		bts, err := anyValue.MarshalMsg(nil, strings, serStrings)
		assert.NoError(t, err)

		expectedBts := []byte{0x7, 0x93, 0xa3, 0x66, 0x6f, 0x6f, 0x1, 0xa3, 0x62, 0x61, 0x72}
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
		//assert.Equal(t, data, bts)

		strings2 := NewStringTable()
		value2, _, err := UnmarshalAnyValue(bts, strings2)
		assert.NoError(t, err)
		value.assertEqual(t, value2, strings, strings2)
	})
}

func (actual *AnyValue) assertEqual(t *testing.T, expected *AnyValue, actualStrings *StringTable, expectedStrings *StringTable) {
	switch v := actual.Value.(type) {
	case *AnyValue_StringValue:
		actualString := actualStrings.Get(v.StringValue)
		expectedString := expectedStrings.Get(expected.Value.(*AnyValue_StringValue).StringValue)
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
		//assert.Equal(t, data, bts)

		strings2 := NewStringTable()
		link2 := &InternalSpanLink{Strings: strings2}
		_, err = link2.UnmarshalMsg(bts)
		assert.NoError(t, err)
		link.assertEqual(t, link2)
	})
}

func (actual *InternalSpanLink) assertEqual(t *testing.T, expected *InternalSpanLink) {
	assert.Equal(t, expected.TraceID, actual.TraceID)
	assert.Equal(t, expected.SpanID, actual.SpanID)
	assert.Len(t, actual.Attributes, len(expected.Attributes))
	for k, v := range expected.Attributes {
		// If a key is overwritten in unmarshalling it can result in a different index
		// so we need to lookup the key from the strings table
		expectedKey := expected.Strings.Get(k)
		actualKeyIndex := actual.Strings.lookup[expectedKey]
		actual.Attributes[actualKeyIndex].assertEqual(t, v, actual.Strings, expected.Strings)
	}
	assert.Equal(t, expected.Strings.Get(expected.Tracestate), actual.Strings.Get(actual.Tracestate))
	assert.Equal(t, expected.Flags, actual.Flags)
}
