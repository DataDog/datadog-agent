// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	fmt "fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

// These tests check the custom modifications made on top of the msgpack
// deserializer generated by tinylib/msgp.

func decodeBytes(bts []byte) (*Span, error) {
	var s Span
	_, err := s.UnmarshalMsg(bts)
	if err != nil {
		fmt.Printf("cause: %v\n", msgp.Cause(err))
	}
	return &s, err
}

func newEmptyMessage() []byte {
	b := []byte{}
	b = msgp.AppendMapHeader(b, 1)
	return b
}

func testStringField(t *testing.T, name string, get func(span *Span) string) {
	t.Run("Nil", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, "", get(s))
	})

	t.Run("BinType", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendBytes(b, []byte("test_string"))
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, "test_string", get(s))
	})

	t.Run("StrType", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendString(b, "test_string")
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, "test_string", get(s))
	})

	t.Run("BinType_InvalidUTF8", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendBytes(b, []byte("op\x99\xbf"))
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, "op��", get(s))
	})

	t.Run("StrType_InvalidUTF8", func(t *testing.T) {
		bts := newEmptyMessage()
		bts = msgp.AppendString(bts, name)
		bts = msgp.AppendString(bts, "op\x99\xbf")
		s, err := decodeBytes(bts)
		assert.Nil(t, err)
		assert.Equal(t, "op��", get(s))
	})
}

func testUint64Field(t *testing.T, name string, get func(span *Span) uint64) {
	t.Run("Nil", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, uint64(0), get(s))
	})

	t.Run("Unsigned", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendUint64(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, uint64(42), get(s))
	})

	t.Run("Signed", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendInt64(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, uint64(42), get(s))
	})
}

func testInt64Field(t *testing.T, name string, get func(span *Span) int64) {
	t.Run("Nil", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, int64(0), get(s))
	})

	t.Run("Unsigned", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendUint64(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, int64(42), get(s))
	})

	t.Run("Signed", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendInt64(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, int64(42), get(s))
	})
}

func testInt32Field(t *testing.T, name string, get func(span *Span) int32) {
	t.Run("Nil", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, int32(0), get(s))
	})

	t.Run("Unsigned", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendUint32(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, int32(42), get(s))
	})

	t.Run("Signed", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, name)
		b = msgp.AppendInt32(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, int32(42), get(s))
	})
}

func TestMetaMapDeserialization(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Nil(t, s.Meta)
	})

	t.Run("Empty", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 0)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Nil(t, s.Meta)
	})

	t.Run("StrType", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key")
		b = msgp.AppendString(b, "value")
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"key": "value"}, s.Meta)
	})

	t.Run("BinType", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendBytes(b, []byte("key"))
		b = msgp.AppendBytes(b, []byte("value"))
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"key": "value"}, s.Meta)
	})

	t.Run("StrType_InvalidUTF8", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key")
		// b = msgp.AppendString(b, "op\x99\xbf")
		b = msgp.AppendString(b, "op��")
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"key": "op��"}, s.Meta)
	})

	t.Run("BinType_InvalidUTF8", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendBytes(b, []byte("key"))
		b = msgp.AppendBytes(b, []byte("op\x99\xbf"))
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"key": "op��"}, s.Meta)
	})

	t.Run("Nil_key", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendNil(b)
		b = msgp.AppendString(b, "val")
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"": "val"}, s.Meta)
	})

	t.Run("Nil_val", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key")
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"key": ""}, s.Meta)
	})
}

func TestMetricsMapDeserialization(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Nil(t, s.Metrics)
	})

	t.Run("Empty", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 0)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Nil(t, s.Metrics)
	})

	t.Run("StrType_Float64", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key")
		b = msgp.AppendFloat64(b, 42.42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"key": 42.42}, s.Metrics)
	})

	t.Run("BinType_Float64", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendBytes(b, []byte("key"))
		b = msgp.AppendFloat64(b, 42.42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"key": 42.42}, s.Metrics)
	})

	t.Run("StrType_InvalidUTF8_Float64", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key\x99\xbf")
		b = msgp.AppendFloat64(b, 42.42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"key��": 42.42}, s.Metrics)
	})

	t.Run("BinType_InvalidUTF8_Float64", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendBytes(b, []byte("key\x99\xbf"))
		b = msgp.AppendFloat64(b, 42.42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"key��": 42.42}, s.Metrics)
	})

	t.Run("StrType_UInt64", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key")
		b = msgp.AppendUint64(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"key": 42}, s.Metrics)
	})

	t.Run("StrType_Int64", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key")
		b = msgp.AppendInt64(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"key": 42}, s.Metrics)
	})

	t.Run("Nil_key", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendNil(b)
		b = msgp.AppendInt64(b, 42)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"": 42}, s.Metrics)
	})

	t.Run("Nil_val", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "metrics")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "key")
		b = msgp.AppendNil(b)
		s, err := decodeBytes(b)
		assert.Nil(t, err)
		assert.Equal(t, map[string]float64{"key": 0}, s.Metrics)
	})
}

func TestDeserialization(t *testing.T) {
	t.Run("service", func(t *testing.T) {
		testStringField(t, "service", func(span *Span) string {
			return span.Service
		})
	})

	t.Run("name", func(t *testing.T) {
		testStringField(t, "name", func(span *Span) string {
			return span.Name
		})
	})

	t.Run("resoure", func(t *testing.T) {
		testStringField(t, "resource", func(span *Span) string {
			return span.Resource
		})
	})

	t.Run("type", func(t *testing.T) {
		testStringField(t, "type", func(span *Span) string {
			return span.Type
		})
	})

	t.Run("trace_id", func(t *testing.T) {
		testUint64Field(t, "trace_id", func(span *Span) uint64 {
			return span.TraceID
		})
	})

	t.Run("span_id", func(t *testing.T) {
		testUint64Field(t, "span_id", func(span *Span) uint64 {
			return span.SpanID
		})
	})

	t.Run("start", func(t *testing.T) {
		testInt64Field(t, "start", func(span *Span) int64 {
			return span.Start
		})
	})

	t.Run("duration", func(t *testing.T) {
		testInt64Field(t, "duration", func(span *Span) int64 {
			return span.Duration
		})
	})

	t.Run("error", func(t *testing.T) {
		testInt32Field(t, "error", func(span *Span) int32 {
			return span.Error
		})
	})
}
