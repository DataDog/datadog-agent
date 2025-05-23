// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

func TestDecode(t *testing.T) {
	irData := ir.Program{
		ID: 1,
		Types: []ir.Type{
			&ir.PointerType{
				TypeCommon: ir.TypeCommon{
					ID:   0,
					Name: "*main.Param",
					Size: 8,
				},
				GoTypeAttributes: ir.GoTypeAttributes{
					HasGoKind: true,
					GoKind:    reflect.Pointer,
				},
				Pointee: &ir.StructureType{
					TypeCommon: ir.TypeCommon{
						ID: 1,
					},
				},
			},
			&ir.StructureType{
				TypeCommon: ir.TypeCommon{
					ID:   1,
					Name: "main.Param",
					Size: 8,
				},
				GoTypeAttributes: ir.GoTypeAttributes{
					HasGoKind: true,
					GoKind:    reflect.Struct,
				},
				Fields: []ir.Field{
					{
						Name:   "idx",
						Offset: 0,
						Type:   2,
					},
					{
						Name:   "random",
						Offset: 4,
						Type:   2,
					},
				},
			},
			&ir.BaseType{
				TypeCommon: ir.TypeCommon{
					ID:   2,
					Name: "uint32",
					Size: 4,
				},
				GoTypeAttributes: ir.GoTypeAttributes{
					HasGoKind: true,
					GoKind:    reflect.Uint32,
				},
			},
			&ir.EventRootType{
				TypeCommon: ir.TypeCommon{
					ID:   3,
					Name: "param",
					Size: 8,
				},
				Expressions: []ir.RootExpression{
					{
						Offset: 0,
						Expression: ir.Expression{
							Type: &ir.PointerType{
								TypeCommon: ir.TypeCommon{
									Name: "*main.Param",
									ID:   0,
									Size: 8,
								},
							},
						},
					},
				},
			},
		},
	}

	fullRingbufferToDecode := []byte{}

	eventHeader := &output.EventHeader{
		Data_byte_len:  0x58,
		Prog_id:        0x1,
		Event_id:       0x2,
		Stack_byte_len: 0x3,
		X__padding:     [2]int8{0, 0},
		Stack_hash:     0x1,
		Ktime_ns:       0x2,
	}
	eventHeaderBytes := unsafe.Slice((*byte)(unsafe.Pointer(eventHeader)), unsafe.Sizeof(*eventHeader))
	fullRingbufferToDecode = append(fullRingbufferToDecode, eventHeaderBytes...)

	dataItem1Header := &output.DataItemHeader{Type: 0x3, Length: 0x8, Address: 0x0}
	dataItem1Bytes := []byte{0x1, 0x28, 0x3f, 0x5, 0x0, 0x40, 0x0, 0x0}
	dataItem1HeaderBytes := unsafe.Slice((*byte)(unsafe.Pointer(dataItem1Header)), unsafe.Sizeof(*dataItem1Header))
	fullRingbufferToDecode = append(fullRingbufferToDecode, dataItem1HeaderBytes...)
	fullRingbufferToDecode = append(fullRingbufferToDecode, dataItem1Bytes...)

	dataItem2Header := &output.DataItemHeader{Type: 0x1, Length: 0x8, Address: 0x4000053f2801}
	dataItem2Bytes := []byte{0x1, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0}
	dataItem2HeaderBytes := unsafe.Slice((*byte)(unsafe.Pointer(dataItem2Header)), unsafe.Sizeof(*dataItem2Header))
	fullRingbufferToDecode = append(fullRingbufferToDecode, dataItem2HeaderBytes...)
	fullRingbufferToDecode = append(fullRingbufferToDecode, dataItem2Bytes...)

	// Now fullRingbufferToDecode contains the complete binary representation

	decoder := NewDecoder(&irData)
	err := decoder.Decode(fullRingbufferToDecode)
	assert.NoError(t, err)
}
