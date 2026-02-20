// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalStreamingString(t *testing.T) {
	t.Run("new string", func(t *testing.T) {
		strings := NewStringTable()
		bts := []byte{0xAA} // FIXSTR of 10 bytes
		bts = append(bts, []byte("my-service")...)
		index, o, err := UnmarshalStreamingString(bts, strings)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), index)
		assert.Empty(t, o)
		assert.Equal(t, "my-service", strings.Get(1))
	})
	t.Run("existing string", func(t *testing.T) {
		strings := NewStringTable()
		strings.addUnchecked("existing-string")
		bts := []byte{0x01} // fixint pointed to index 1
		index, o, err := UnmarshalStreamingString(bts, strings)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), index)
		assert.Empty(t, o)
		assert.Equal(t, "existing-string", strings.Get(1))
	})
}

func TestUnmarshalAnyValue(t *testing.T) {
	t.Run("string value", func(t *testing.T) {
		strings := NewStringTable()
		bts := []byte{0x01, 0xA4} // fixint of 1 (String anytype), fixstr of 4 bytes
		bts = append(bts, []byte("test")...)
		value, o, err := UnmarshalAnyValue(bts, strings)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), value.Value.(*AnyValue_StringValueRef).StringValueRef)
		assert.Empty(t, o)
		assert.Equal(t, "test", strings.Get(1))
	})
	t.Run("bool value", func(t *testing.T) {
		bts := []byte{0x02, 0xc3} // fixint of 2 (Bool anytype), bool true
		value, o, err := UnmarshalAnyValue(bts, nil)
		assert.NoError(t, err)
		assert.Equal(t, true, value.Value.(*AnyValue_BoolValue).BoolValue)
		assert.Empty(t, o)
	})
	t.Run("double value", func(t *testing.T) {
		bts := []byte{0x03, 0xcb, 0x40, 0x09, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f} // fixint of 3 (Double anytype), double 3.14
		value, o, err := UnmarshalAnyValue(bts, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3.14, value.Value.(*AnyValue_DoubleValue).DoubleValue)
		assert.Empty(t, o)
	})
	t.Run("int value", func(t *testing.T) {
		bts := []byte{0x04, 0x02} // fixint of 4 (Int anytype), int 2
		value, o, err := UnmarshalAnyValue(bts, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), value.Value.(*AnyValue_IntValue).IntValue)
		assert.Empty(t, o)
	})
	t.Run("bytes value", func(t *testing.T) {
		bts := []byte{0x05, 0xc4, 0x01, 0xAF} // fixint of 5 (Bytes anytype), bin header, 1 byte in length, 0xAF
		value, o, err := UnmarshalAnyValue(bts, nil)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0xAF}, value.Value.(*AnyValue_BytesValue).BytesValue)
		assert.Empty(t, o)
	})
	t.Run("array value", func(t *testing.T) {
		bts := []byte{0x06, 0x94, 0x04, 0x02, 0x04, 0x03} // fixint of 6 (Array anytype), array header 4 elements, ints 2 and 3
		value, o, err := UnmarshalAnyValue(bts, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), value.Value.(*AnyValue_ArrayValue).ArrayValue.Values[0].Value.(*AnyValue_IntValue).IntValue)
		assert.Equal(t, int64(3), value.Value.(*AnyValue_ArrayValue).ArrayValue.Values[1].Value.(*AnyValue_IntValue).IntValue)
		assert.Empty(t, o)
	})
	t.Run("keyvalue list", func(t *testing.T) {
		strings := NewStringTable()
		bts := []byte{0x07, 0x96, 0xA4}                // fixint of 7 (KeyValueList anytype), array header 6 elements, fixint of 1 (String anytype), fixstr of 4 bytes
		bts = append(bts, []byte("test")...)           // Append "test" bytes
		bts = append(bts, []byte{0x04, 0x08, 0xA5}...) // Append value, fixint of 4 (int anytype), posint 8, String len 5
		bts = append(bts, []byte("test2")...)          // Append 2nd Key "test2" bytes
		bts = append(bts, []byte{0x04, 0x09}...)       // Append value, fixint of 4 (int anytype), posint 9
		value, o, err := UnmarshalAnyValue(bts, strings)
		assert.NoError(t, err)
		assert.Equal(t, 3, strings.Len())
		assert.Len(t, value.Value.(*AnyValue_KeyValueList).KeyValueList.KeyValues, 2)
		assert.Equal(t, uint32(1), value.Value.(*AnyValue_KeyValueList).KeyValueList.KeyValues[0].Key)
		assert.Equal(t, uint32(2), value.Value.(*AnyValue_KeyValueList).KeyValueList.KeyValues[1].Key)
		assert.Equal(t, int64(8), value.Value.(*AnyValue_KeyValueList).KeyValueList.KeyValues[0].Value.Value.(*AnyValue_IntValue).IntValue)
		assert.Equal(t, int64(9), value.Value.(*AnyValue_KeyValueList).KeyValueList.KeyValues[1].Value.Value.(*AnyValue_IntValue).IntValue)
		assert.Empty(t, o)
	})
}

func TestUnmarshalSpanLinks(t *testing.T) {
	t.Run("valid span links", func(t *testing.T) {
		strings := NewStringTable()
		strings.addUnchecked("potato")
		bts := []byte{0x91, 0x85, 0x01, 0xc4, 0x01, 0xAF}          // array header 1 element, FixMap of 5 elements, 1 key (traceID), bytes header, 1 byte in length, 0xAF (TraceID)
		bts = append(bts, []byte{0x02, 0x12}...)                   // 2nd key (spanID), fixint 12
		bts = append(bts, []byte{0x03, 0x93, 0x01, 0x04, 0x02}...) // 3rd key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2
		bts = append(bts, []byte{0x04, 0xAA}...)                   // 4th key (tracestate), string of length 10
		bts = append(bts, []byte("test-state")...)                 // test-state bytes
		bts = append(bts, []byte{0x05, 0x03}...)                   // 5th key (flags), fixint 3
		links, o, err := UnmarshalSpanLinks(bts, strings)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0xAF}, links[0].TraceID)
		assert.Equal(t, uint64(0x12), links[0].SpanID)
		assert.Len(t, links[0].Attributes, 1)
		assert.Equal(t, int64(2), links[0].Attributes[1].Value.(*AnyValue_IntValue).IntValue)
		assert.Equal(t, "test-state", strings.Get(2))
		assert.Equal(t, uint32(2), links[0].TracestateRef)
		assert.Equal(t, uint32(3), links[0].Flags)
		assert.Empty(t, o)
	})
}

func rawSpan() []byte {
	// From RFC example
	bts := []byte{0x85}
	bts = append(bts, []byte{0x1, 0xAA}...) // 1st key (service), fixstr length 10
	bts = append(bts, []byte("my-service")...)
	bts = append(bts, []byte{0x02, 0xA9}...) // 2nd key (name), fixstr length 9
	bts = append(bts, []byte("span-name")...)
	bts = append(bts, []byte{0x03, 0xA8}...) // 3rd key (resource), fixstr length 8
	bts = append(bts, []byte("GET /res")...)
	bts = append(bts, []byte{0x04, 0xCE, 0x00, 0xBC, 0x61, 0x4E}...) // 4th key (spanID), uint32 header, 12345678

	bts = append(bts, []byte{0x09, 0x99}...) // 9th key (Attributes), array header 9 elements
	bts = append(bts, []byte{0xA3}...)       // 1st key (key), fixstr length 3
	bts = append(bts, []byte("foo")...)
	bts = append(bts, []byte{0x1, 0xA3}...) // 1 string value, fixstr length 3
	bts = append(bts, []byte("bar")...)
	bts = append(bts, []byte{0xA4}...) // fixstr length 2
	bts = append(bts, []byte("foo2")...)
	bts = append(bts, []byte{0x01, 0x05}...) // 01 string value, fixint index to 5
	bts = append(bts, []byte{0xA8}...)       // fixstr length 8
	bts = append(bts, []byte("some-num")...)
	bts = append(bts, []byte{0x04, 0x2A}...) // 4 int value, posint 42
	return bts
}

func TestUnmarshalSpan(t *testing.T) {
	t.Run("valid span", func(t *testing.T) {
		strings := NewStringTable()
		bts := rawSpan()
		var span = &InternalSpan{Strings: strings}
		o, err := span.UnmarshalMsg(bts)
		assert.NoError(t, err)
		assert.Empty(t, o)
		assert.Equal(t, "my-service", strings.Get(1))
		assert.Equal(t, "span-name", strings.Get(2))
		assert.Equal(t, "GET /res", strings.Get(3))
		assert.Equal(t, "foo", strings.Get(4))
		assert.Equal(t, "bar", strings.Get(5))
		assert.Equal(t, "foo2", strings.Get(6))
		assert.Equal(t, "some-num", strings.Get(7))
		assert.Equal(t, uint32(1), span.span.ServiceRef)
		assert.Equal(t, uint32(2), span.span.NameRef)
		assert.Equal(t, uint32(3), span.span.ResourceRef)
		assert.Equal(t, uint64(0xBC_61_4E), span.span.SpanID)
		assert.Equal(t, uint32(5), span.span.Attributes[4].Value.(*AnyValue_StringValueRef).StringValueRef)
		assert.Equal(t, uint32(5), span.span.Attributes[6].Value.(*AnyValue_StringValueRef).StringValueRef)
		assert.Equal(t, int64(42), span.span.Attributes[7].Value.(*AnyValue_IntValue).IntValue)
	})
}

func TestUnmarshalSpanEventList(t *testing.T) {
	t.Run("valid span event", func(t *testing.T) {
		strings := NewStringTable()
		bts := []byte{0x91, 0x83, 0x01, 0x02, 0x02, 0xA6} // array header 1 element, map header 3 elements, 1 key (time), 2 (int64), 2 key (name), string of length 6
		bts = append(bts, []byte("name12")...)
		bts = append(bts, []byte{0x03, 0x93, 0x01, 0x04, 0x07}...) // 3rd key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 7
		spanEvents, o, err := UnmarshalSpanEventList(bts, strings)
		assert.NoError(t, err)
		assert.Empty(t, o)
		assert.Len(t, spanEvents, 1)
		assert.Equal(t, uint64(2), spanEvents[0].Time)
		assert.Equal(t, uint32(1), spanEvents[0].NameRef)
		assert.Len(t, spanEvents[0].Attributes, 1)
		assert.Equal(t, int64(7), spanEvents[0].Attributes[1].Value.(*AnyValue_IntValue).IntValue)
	})
}

func TestSafeReadHeaderBytesLimitsSize(t *testing.T) {
	// Test that safeReadHeaderBytes properly rejects payloads claiming to be too large.
	// The limit is 25MB (25*1e6 elements).
	// 0xdd is the msgpack array32 header, followed by 4 bytes for the size.
	// 0xdf is the msgpack map32 header, followed by 4 bytes for the size.

	t.Run("rejects oversized array header in InternalTracerPayload.UnmarshalMsgConverted", func(t *testing.T) {
		// Array header claiming 0xFFFFFFFF elements (over 4 billion)
		oversizedPayload := []byte{0xdd, 0xff, 0xff, 0xff, 0xff}
		tp := &InternalTracerPayload{}
		_, err := tp.UnmarshalMsgConverted(oversizedPayload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too long payload")
	})

	t.Run("rejects oversized map header in InternalSpan.UnmarshalMsgConverted", func(t *testing.T) {
		// Map header claiming 0xFFFFFFFF elements (over 4 billion)
		oversizedPayload := []byte{0xdf, 0xff, 0xff, 0xff, 0xff}
		strings := NewStringTable()
		span := NewInternalSpan(strings, &Span{})
		convertedFields := NewSpanConvertedFields()
		_, err := span.UnmarshalMsgConverted(oversizedPayload, convertedFields)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too long payload")
	})

	t.Run("rejects oversized array header in InternalTraceChunk.UnmarshalMsgConverted", func(t *testing.T) {
		// Array header claiming 0xFFFFFFFF elements (over 4 billion)
		oversizedPayload := []byte{0xdd, 0xff, 0xff, 0xff, 0xff}
		strings := NewStringTable()
		chunk := &InternalTraceChunk{Strings: strings}
		chunkConvertedFields := &ChunkConvertedFields{}
		_, err := chunk.UnmarshalMsgConverted(oversizedPayload, chunkConvertedFields)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too long payload")
	})
}
