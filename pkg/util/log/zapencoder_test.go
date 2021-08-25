// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestClone(t *testing.T) {
	enc := &encoder{}
	enc.OpenNamespace("ns1")

	enc2 := enc.Clone()
	enc2.OpenNamespace("ns2")

	require.Equal(t, "ns1/", enc.prefix)
	require.Equal(t, "ns1/ns2/", enc2.prefix)

	enc.AddString("key", "value")
	enc2.AddString("key2", "value2")
	assert.Equal(t, []interface{}{"ns1/key", "value"}, enc.ctx)
	assert.Equal(t, []interface{}{"ns1/ns2/key2", "value2"}, enc2.ctx)

}

func TestArrayEncoder(t *testing.T) {
	// Modified from https://github.com/uber-go/zap/blob/v1.19.0/zapcore/memory_encoder_test.go#L227
	tests := []struct {
		desc     string
		encode   func(zapcore.ArrayEncoder)
		expected interface{}
	}{
		{"AppendBool", func(e zapcore.ArrayEncoder) { e.AppendBool(true) }, true},
		{"AppendByteString", func(e zapcore.ArrayEncoder) { e.AppendByteString([]byte("foo")) }, "foo"},
		{"AppendComplex128", func(e zapcore.ArrayEncoder) { e.AppendComplex128(1 + 2i) }, 1 + 2i},
		{"AppendComplex64", func(e zapcore.ArrayEncoder) { e.AppendComplex64(1 + 2i) }, complex64(1 + 2i)},
		{"AppendDuration", func(e zapcore.ArrayEncoder) { e.AppendDuration(time.Second) }, time.Second},
		{"AppendFloat64", func(e zapcore.ArrayEncoder) { e.AppendFloat64(3.14) }, 3.14},
		{"AppendFloat32", func(e zapcore.ArrayEncoder) { e.AppendFloat32(3.14) }, float32(3.14)},
		{"AppendInt", func(e zapcore.ArrayEncoder) { e.AppendInt(42) }, 42},
		{"AppendInt64", func(e zapcore.ArrayEncoder) { e.AppendInt64(42) }, int64(42)},
		{"AppendInt32", func(e zapcore.ArrayEncoder) { e.AppendInt32(42) }, int32(42)},
		{"AppendInt16", func(e zapcore.ArrayEncoder) { e.AppendInt16(42) }, int16(42)},
		{"AppendInt8", func(e zapcore.ArrayEncoder) { e.AppendInt8(42) }, int8(42)},
		{"AppendString", func(e zapcore.ArrayEncoder) { e.AppendString("foo") }, "foo"},
		{"AppendTime", func(e zapcore.ArrayEncoder) { e.AppendTime(time.Unix(0, 100)) }, time.Unix(0, 100)},
		{"AppendUint", func(e zapcore.ArrayEncoder) { e.AppendUint(42) }, uint(42)},
		{"AppendUint64", func(e zapcore.ArrayEncoder) { e.AppendUint64(42) }, uint64(42)},
		{"AppendUint32", func(e zapcore.ArrayEncoder) { e.AppendUint32(42) }, uint32(42)},
		{"AppendUint16", func(e zapcore.ArrayEncoder) { e.AppendUint16(42) }, uint16(42)},
		{"AppendUint8", func(e zapcore.ArrayEncoder) { e.AppendUint8(42) }, uint8(42)},
		{"AppendUintptr", func(e zapcore.ArrayEncoder) { e.AppendUintptr(42) }, uintptr(42)},
		{"AppendReflected", func(e zapcore.ArrayEncoder) { e.AppendReflected(map[string]int{"foo": 5}) }, map[string]int{"foo": 5}},
		{
			desc: "AppendArray (arrays of arrays)",
			encode: func(e zapcore.ArrayEncoder) {
				e.AppendArray(zapcore.ArrayMarshalerFunc(func(inner zapcore.ArrayEncoder) error {
					inner.AppendBool(true)
					inner.AppendBool(false)
					return nil
				}))
			},
			expected: []interface{}{true, false},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.desc, func(t *testing.T) {
			enc := &sliceArrayEncoder{}
			testInstance.encode(enc)
			testInstance.encode(enc)
			assert.Equal(t, []interface{}{testInstance.expected, testInstance.expected}, enc.elems, "Unexpected output.")
		})
	}
}

func TestObjectEncoder(t *testing.T) {
	// Adapted from https://github.com/uber-go/zap/blob/v1.19.0/zapcore/memory_encoder_test.go#L31
	tests := []struct {
		desc     string
		encode   func(zapcore.ObjectEncoder)
		expected interface{}
	}{
		{
			desc: "AddArray",
			encode: func(e zapcore.ObjectEncoder) {
				assert.NoError(t, e.AddArray("k", zapcore.ArrayMarshalerFunc(func(arr zapcore.ArrayEncoder) error {
					arr.AppendBool(true)
					arr.AppendBool(false)
					arr.AppendBool(true)
					return nil
				})), "Expected AddArray to succeed.")
			},
			expected: []interface{}{"k", []interface{}{true, false, true}},
		},
		{"AddBinary", func(e zapcore.ObjectEncoder) { e.AddBinary("k", []byte("foo")) }, []interface{}{"k", []byte("foo")}},
		{"AddByteString", func(e zapcore.ObjectEncoder) { e.AddByteString("k", []byte("foo")) }, []interface{}{"k", "foo"}},
		{"AddBool", func(e zapcore.ObjectEncoder) { e.AddBool("k", true) }, []interface{}{"k", true}},
		{"AddComplex128", func(e zapcore.ObjectEncoder) { e.AddComplex128("k", 1+2i) }, []interface{}{"k", 1 + 2i}},
		{"AddComplex64", func(e zapcore.ObjectEncoder) { e.AddComplex64("k", 1+2i) }, []interface{}{"k", complex64(1 + 2i)}},
		{"AddDuration", func(e zapcore.ObjectEncoder) { e.AddDuration("k", time.Millisecond) }, []interface{}{"k", time.Millisecond}},
		{"AddFloat64", func(e zapcore.ObjectEncoder) { e.AddFloat64("k", 3.14) }, []interface{}{"k", 3.14}},
		{"AddFloat32", func(e zapcore.ObjectEncoder) { e.AddFloat32("k", 3.14) }, []interface{}{"k", float32(3.14)}},
		{"AddInt", func(e zapcore.ObjectEncoder) { e.AddInt("k", 42) }, []interface{}{"k", 42}},
		{"AddInt64", func(e zapcore.ObjectEncoder) { e.AddInt64("k", 42) }, []interface{}{"k", int64(42)}},
		{"AddInt32", func(e zapcore.ObjectEncoder) { e.AddInt32("k", 42) }, []interface{}{"k", int32(42)}},
		{"AddInt16", func(e zapcore.ObjectEncoder) { e.AddInt16("k", 42) }, []interface{}{"k", int16(42)}},
		{"AddInt8", func(e zapcore.ObjectEncoder) { e.AddInt8("k", 42) }, []interface{}{"k", int8(42)}},
		{"AddString", func(e zapcore.ObjectEncoder) { e.AddString("k", "v") }, []interface{}{"k", "v"}},
		{"AddTime", func(e zapcore.ObjectEncoder) { e.AddTime("k", time.Unix(0, 100)) }, []interface{}{"k", time.Unix(0, 100)}},
		{"AddUint", func(e zapcore.ObjectEncoder) { e.AddUint("k", 42) }, []interface{}{"k", uint(42)}},
		{"AddUint64", func(e zapcore.ObjectEncoder) { e.AddUint64("k", 42) }, []interface{}{"k", uint64(42)}},
		{"AddUint32", func(e zapcore.ObjectEncoder) { e.AddUint32("k", 42) }, []interface{}{"k", uint32(42)}},
		{"AddUint16", func(e zapcore.ObjectEncoder) { e.AddUint16("k", 42) }, []interface{}{"k", uint16(42)}},
		{"AddUint8", func(e zapcore.ObjectEncoder) { e.AddUint8("k", 42) }, []interface{}{"k", uint8(42)}},
		{"AddUintptr", func(e zapcore.ObjectEncoder) { e.AddUintptr("k", 42) }, []interface{}{"k", uintptr(42)}},
		{
			desc: "AddReflected",
			encode: func(e zapcore.ObjectEncoder) {
				assert.NoError(t, e.AddReflected("k", map[string]interface{}{"foo": 5}), "Expected AddReflected to succeed.")
			},
			expected: []interface{}{"k", map[string]interface{}{"foo": 5}},
		},
		{
			desc: "OpenNamespace",
			encode: func(e zapcore.ObjectEncoder) {
				e.OpenNamespace("k")
				e.AddInt("foo", 1)
				e.OpenNamespace("middle")
				e.AddInt("foo", 2)
				e.OpenNamespace("inner")
				e.AddInt("foo", 3)
			},
			expected: []interface{}{"k/foo", 1, "k/middle/foo", 2, "k/middle/inner/foo", 3},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.desc, func(t *testing.T) {
			enc := &encoder{}
			testInstance.encode(enc)
			assert.Equal(t, testInstance.expected, enc.ctx, "Unexpected encoder output.")
		})
	}

}
