// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

const maxDepth = 10 // Prevent infinite recursion when formatting nested types

// formatType recursively formats a value based on its type.
func formatType(
	t ir.Type,
	data []byte,
	dataItems map[typeAndAddr]output.DataItem,
	depth int,
) string {
	if depth > maxDepth {
		return "<max depth>"
	}

	switch t := t.(type) {
	case *ir.BaseType:
		return formatBaseType(t, data)

	case *ir.StructureType:
		return formatStruct(t, data, dataItems, depth)

	case *ir.GoStringHeaderType:
		return formatString(t, data, dataItems)

	case *ir.PointerType:
		return formatPointer(t, data, dataItems, depth)

	case *ir.GoSliceHeaderType:
		return formatSlice(t, data, dataItems, depth)

	default:
		// Fallback for unsupported types
		return fmt.Sprintf("<%s>", t.GetName())
	}
}

func formatBaseType(t *ir.BaseType, data []byte) string {
	goKind, ok := t.GetGoKind()
	if !ok {
		return fmt.Sprintf("<%s>", t.Name)
	}

	switch goKind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", readInt(data, t.ByteSize))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", readUint(data, t.ByteSize))
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", readFloat(data, t.ByteSize))
	case reflect.Bool:
		if len(data) > 0 && data[0] != 0 {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("<%s>", t.Name)
	}
}

func formatStruct(
	t *ir.StructureType,
	data []byte,
	dataItems map[typeAndAddr]output.DataItem,
	depth int,
) string {
	var result strings.Builder
	result.WriteString(t.Name)
	result.WriteByte('{')

	first := true
	for field := range t.Fields() {
		if !first {
			result.WriteString(", ")
		}
		first = false

		result.WriteString(field.Name)
		result.WriteString(": ")

		fieldEnd := field.Offset + field.Type.GetByteSize()
		if fieldEnd > uint32(len(data)) {
			result.WriteString("<truncated>")
			continue
		}

		fieldData := data[field.Offset:fieldEnd]
		result.WriteString(formatType(field.Type, fieldData, dataItems, depth+1))
	}

	result.WriteByte('}')
	return result.String()
}

func formatString(
	t *ir.GoStringHeaderType,
	data []byte,
	dataItems map[typeAndAddr]output.DataItem,
) string {
	if len(data) < 16 {
		return "<string header truncated>"
	}

	// Read string pointer and length from header
	ptr := binary.LittleEndian.Uint64(data[0:8])
	length := binary.LittleEndian.Uint64(data[8:16])

	// Look up string data in dataItems
	key := typeAndAddr{
		irType: uint32(t.Data.ID),
		addr:   ptr,
	}

	item, ok := dataItems[key]
	if !ok {
		return "<string not captured>"
	}

	strData, ok := item.Data()
	if !ok {
		return "<string read failed>"
	}

	if uint64(len(strData)) < length {
		length = uint64(len(strData))
	}

	return fmt.Sprintf("%q", string(strData[:length]))
}

func formatPointer(
	t *ir.PointerType,
	data []byte,
	dataItems map[typeAndAddr]output.DataItem,
	depth int,
) string {
	if len(data) < 8 {
		return "<pointer truncated>"
	}

	ptrVal := binary.LittleEndian.Uint64(data)
	if ptrVal == 0 {
		return "nil"
	}

	// Look up pointed-to data
	key := typeAndAddr{
		irType: uint32(t.Pointee.GetID()),
		addr:   ptrVal,
	}

	item, ok := dataItems[key]
	if !ok {
		return fmt.Sprintf("*%s@0x%x", t.Pointee.GetName(), ptrVal)
	}

	pointeeData, ok := item.Data()
	if !ok {
		return fmt.Sprintf("*%s@0x%x<read failed>", t.Pointee.GetName(), ptrVal)
	}

	// Dereference and format
	formatted := formatType(t.Pointee, pointeeData, dataItems, depth+1)
	return formatted
}

func formatSlice(
	t *ir.GoSliceHeaderType,
	data []byte,
	dataItems map[typeAndAddr]output.DataItem,
	depth int,
) string {
	if len(data) < 24 {
		return "<slice header truncated>"
	}

	// Read slice header: ptr, len, cap
	ptr := binary.LittleEndian.Uint64(data[0:8])
	length := binary.LittleEndian.Uint64(data[8:16])

	if length == 0 {
		return "[]"
	}

	// Limit display length
	displayLen := length
	if displayLen > 10 {
		displayLen = 10
	}

	// Look up slice data
	key := typeAndAddr{
		irType: uint32(t.Data.GetID()),
		addr:   ptr,
	}

	item, ok := dataItems[key]
	if !ok {
		return fmt.Sprintf("[%d elements]", length)
	}

	sliceData, ok := item.Data()
	if !ok {
		return "[<read failed>]"
	}

	var result strings.Builder
	result.WriteByte('[')

	elemSize := t.Data.Element.GetByteSize()
	for i := uint64(0); i < displayLen; i++ {
		if i > 0 {
			result.WriteString(", ")
		}

		elemStart := i * uint64(elemSize)
		elemEnd := elemStart + uint64(elemSize)
		if elemEnd > uint64(len(sliceData)) {
			result.WriteString("...")
			break
		}

		elemData := sliceData[elemStart:elemEnd]
		result.WriteString(formatType(t.Data.Element, elemData, dataItems, depth+1))
	}

	if length > displayLen {
		result.WriteString(", ...")
	}

	result.WriteByte(']')
	return result.String()
}

// Helper functions for reading numeric types

func readInt(data []byte, size uint32) int64 {
	if uint32(len(data)) < size {
		return 0
	}
	switch size {
	case 1:
		return int64(int8(data[0]))
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(data)))
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(data)))
	case 8:
		return int64(binary.LittleEndian.Uint64(data))
	default:
		return 0
	}
}

func readUint(data []byte, size uint32) uint64 {
	if uint32(len(data)) < size {
		return 0
	}
	switch size {
	case 1:
		return uint64(data[0])
	case 2:
		return uint64(binary.LittleEndian.Uint16(data))
	case 4:
		return uint64(binary.LittleEndian.Uint32(data))
	case 8:
		return binary.LittleEndian.Uint64(data)
	default:
		return 0
	}
}

func readFloat(data []byte, size uint32) float64 {
	if uint32(len(data)) < size {
		return 0
	}
	switch size {
	case 4:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(data)))
	case 8:
		return math.Float64frombits(binary.LittleEndian.Uint64(data))
	default:
		return 0
	}
}
