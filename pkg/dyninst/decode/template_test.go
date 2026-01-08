// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

func TestFormatBaseType(t *testing.T) {
	tests := []struct {
		name     string
		typ      *ir.BaseType
		data     []byte
		expected string
	}{
		{
			name: "int64",
			typ: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{Name: "int64", ByteSize: 8},
				GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Int64},
			},
			data:     int64ToBytes(42),
			expected: "42",
		},
		{
			name: "uint32",
			typ: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{Name: "uint32", ByteSize: 4},
				GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Uint32},
			},
			data:     uint32ToBytes(100),
			expected: "100",
		},
		{
			name: "bool true",
			typ: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{Name: "bool", ByteSize: 1},
				GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Bool},
			},
			data:     []byte{1},
			expected: "true",
		},
		{
			name: "bool false",
			typ: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{Name: "bool", ByteSize: 1},
				GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Bool},
			},
			data:     []byte{0},
			expected: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal encodingContext for testing
			ctx := &encodingContext{
				typesByID: map[ir.TypeID]decoderType{
					tt.typ.GetID(): (*baseType)(tt.typ),
				},
				typesByGoRuntimeType: make(map[uint32]ir.TypeID),
				currentlyEncoding:    make(map[typeAndAddr]struct{}),
				dataItems:            make(map[typeAndAddr]output.DataItem),
			}

			var buf bytes.Buffer
			limits := &formatLimits{
				maxBytes:           maxLogLineBytes,
				maxCollectionItems: maxLogCollectionItems,
				maxFields:          maxLogFieldCount,
			}
			err := formatType(ctx, &buf, tt.typ, tt.data, limits)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestFormatStruct(t *testing.T) {
	// Create a simple struct type: User { ID: int64, Age: int32 }
	userType := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "User",
			ByteSize: 12,
		},
		RawFields: []ir.Field{
			{
				Name:   "ID",
				Offset: 0,
				Type: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{Name: "int64", ByteSize: 8},
					GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Int64},
				},
			},
			{
				Name:   "Age",
				Offset: 8,
				Type: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{Name: "int32", ByteSize: 4},
					GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Int32},
				},
			},
		},
	}

	// Create data: ID=42, Age=30
	data := make([]byte, 12)
	binary.NativeEndian.PutUint64(data[0:8], 42)
	binary.NativeEndian.PutUint32(data[8:12], 30)

	// Create minimal encodingContext for testing
	ctx := &encodingContext{
		typesByID: map[ir.TypeID]decoderType{
			userType.GetID():                   (*structureType)(userType),
			userType.RawFields[0].Type.GetID(): (*baseType)(userType.RawFields[0].Type.(*ir.BaseType)),
			userType.RawFields[1].Type.GetID(): (*baseType)(userType.RawFields[1].Type.(*ir.BaseType)),
		},
		typesByGoRuntimeType: make(map[uint32]ir.TypeID),
		currentlyEncoding:    make(map[typeAndAddr]struct{}),
		dataItems:            make(map[typeAndAddr]output.DataItem),
	}

	var buf bytes.Buffer
	limits := &formatLimits{
		maxBytes:           maxLogLineBytes,
		maxCollectionItems: maxLogCollectionItems,
		maxFields:          maxLogFieldCount,
	}
	err := formatType(ctx, &buf, userType, data, limits)
	assert.NoError(t, err)
	assert.Equal(t, "{ID: 42, Age: 30}", buf.String())
}

func TestReadInt(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		size     uint32
		expected int64
	}{
		{
			name:     "int8",
			data:     []byte{0xFF}, // -1
			size:     1,
			expected: -1,
		},
		{
			name:     "int16",
			data:     int16ToBytes(-1000),
			size:     2,
			expected: -1000,
		},
		{
			name:     "int32",
			data:     int32ToBytes(-100000),
			size:     4,
			expected: -100000,
		},
		{
			name:     "int64",
			data:     int64ToBytes(-9876543210),
			size:     8,
			expected: -9876543210,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := readInt(tt.data, tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReadUint(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		size     uint32
		expected uint64
	}{
		{
			name:     "uint8",
			data:     []byte{255},
			size:     1,
			expected: 255,
		},
		{
			name:     "uint16",
			data:     uint16ToBytes(65000),
			size:     2,
			expected: 65000,
		},
		{
			name:     "uint32",
			data:     uint32ToBytes(4000000000),
			size:     4,
			expected: 4000000000,
		},
		{
			name:     "uint64",
			data:     uint64ToBytes(18446744073709551615),
			size:     8,
			expected: 18446744073709551615,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := readUint(tt.data, tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions - use NativeEndian for consistency with code

func int64ToBytes(v int64) []byte {
	b := make([]byte, 8)
	binary.NativeEndian.PutUint64(b, uint64(v))
	return b
}

func int32ToBytes(v int32) []byte {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, uint32(v))
	return b
}

func int16ToBytes(v int16) []byte {
	b := make([]byte, 2)
	binary.NativeEndian.PutUint16(b, uint16(v))
	return b
}

func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.NativeEndian.PutUint64(b, v)
	return b
}

func uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, v)
	return b
}

func uint16ToBytes(v uint16) []byte {
	b := make([]byte, 2)
	binary.NativeEndian.PutUint16(b, v)
	return b
}
