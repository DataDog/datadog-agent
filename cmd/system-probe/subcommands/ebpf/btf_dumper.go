// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"

	"github.com/cilium/ebpf/btf"
)

// BTFDumper converts raw eBPF map data into human-readable JSON format
// using BTF (BPF Type Format) type information. It handles all standard
// BTF types including integers, pointers, structs, arrays, unions, and enums.
//
// Example:
//
//	spec, keyType, valueType, _ := getBTFInfoForMap(m)
//	dumper := NewBTFDumper(spec)
//	formatted, _ := dumper.DumpValue(rawBytes, keyType)
type BTFDumper struct {
	spec      *btf.Spec
	typeCache map[btf.Type]btf.Type
}

// NewBTFDumper creates a new BTF dumper with the given BTF specification.
func NewBTFDumper(spec *btf.Spec) *BTFDumper {
	return &BTFDumper{
		spec:      spec,
		typeCache: make(map[btf.Type]btf.Type),
	}
}

// DumpValue converts raw bytes to a formatted value according to the
// provided BTF type. The returned interface{} can be:
//   - uint64, int64 for integers
//   - string for pointers (hex formatted) and char arrays
//   - map[string]interface{} for structs and unions
//   - []interface{} for arrays
//
// Returns an error if the data cannot be interpreted according to the type,
// for example if the data buffer is too small for the type.
func (d *BTFDumper) DumpValue(data []byte, typ btf.Type) (interface{}, error) {
	// Resolve typedefs, const, volatile, restrict
	typ = d.resolveType(typ)

	// Dispatch to type-specific handlers
	switch t := typ.(type) {
	case *btf.Int:
		return d.dumpInt(data, t)
	case *btf.Pointer:
		return d.dumpPointer(data, t)
	case *btf.Enum:
		return d.dumpEnum(data, t)
	case *btf.Struct:
		return d.dumpStruct(data, t)
	case *btf.Array:
		return d.dumpArray(data, t)
	case *btf.Union:
		return d.dumpUnion(data, t)
	default:
		return fmt.Sprintf("<unsupported type: %T>", typ), nil
	}
}

// resolveType follows type chains (typedefs, const, volatile, restrict) to get the underlying type.
func (d *BTFDumper) resolveType(typ btf.Type) btf.Type {
	// Check cache
	if cached, ok := d.typeCache[typ]; ok {
		return cached
	}

	original := typ

	// Follow type chains
	for {
		switch t := typ.(type) {
		case *btf.Typedef:
			typ = t.Type
		case *btf.Const:
			typ = t.Type
		case *btf.Volatile:
			typ = t.Type
		case *btf.Restrict:
			typ = t.Type
		default:
			d.typeCache[original] = typ
			return typ
		}
	}
}

// dumpInt handles integer types (signed, unsigned, bool, char).
func (d *BTFDumper) dumpInt(data []byte, t *btf.Int) (interface{}, error) {
	switch t.Encoding {
	case btf.Signed:
		return d.readSignedInt(data, t.Size)
	case btf.Unsigned, btf.Char:
		return d.readUnsignedInt(data, t.Size)
	case btf.Bool:
		return data[0] != 0, nil
	default:
		return d.readUnsignedInt(data, t.Size)
	}
}

// readSignedInt reads a signed integer from data with the given size.
func (d *BTFDumper) readSignedInt(data []byte, size uint32) (int64, error) {
	if len(data) < int(size) {
		return 0, fmt.Errorf("buffer too small: need %d bytes, got %d", size, len(data))
	}

	switch size {
	case 1:
		return int64(int8(data[0])), nil
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(data))), nil
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(data))), nil
	case 8:
		return int64(binary.LittleEndian.Uint64(data)), nil
	default:
		return 0, fmt.Errorf("unsupported int size: %d", size)
	}
}

// readUnsignedInt reads an unsigned integer from data with the given size.
func (d *BTFDumper) readUnsignedInt(data []byte, size uint32) (uint64, error) {
	if len(data) < int(size) {
		return 0, fmt.Errorf("buffer too small: need %d bytes, got %d", size, len(data))
	}

	switch size {
	case 1:
		return uint64(data[0]), nil
	case 2:
		return uint64(binary.LittleEndian.Uint16(data)), nil
	case 4:
		return uint64(binary.LittleEndian.Uint32(data)), nil
	case 8:
		return binary.LittleEndian.Uint64(data), nil
	default:
		return 0, fmt.Errorf("unsupported int size: %d", size)
	}
}

// dumpPointer formats a pointer as a hex string.
func (d *BTFDumper) dumpPointer(data []byte, t *btf.Pointer) (interface{}, error) {
	// Read pointer as uintptr (depends on architecture)
	var addr uint64
	if len(data) == 8 {
		addr = binary.LittleEndian.Uint64(data)
	} else if len(data) == 4 {
		addr = uint64(binary.LittleEndian.Uint32(data))
	} else {
		return nil, fmt.Errorf("unsupported pointer size: %d", len(data))
	}

	// Format as hex string
	if len(data) == 8 {
		return fmt.Sprintf("0x%016x", addr), nil
	}
	return fmt.Sprintf("0x%08x", addr), nil
}

// dumpEnum formats an enum value, showing both symbolic name and numeric value.
func (d *BTFDumper) dumpEnum(data []byte, t *btf.Enum) (interface{}, error) {
	// Read numeric value
	var value int64
	if t.Signed {
		val, err := d.readSignedInt(data, t.Size)
		if err != nil {
			return nil, err
		}
		value = val
	} else {
		uval, err := d.readUnsignedInt(data, t.Size)
		if err != nil {
			return nil, err
		}
		value = int64(uval)
	}

	// Look up symbolic name
	for _, ev := range t.Values {
		if int64(ev.Value) == value {
			return map[string]interface{}{
				"name":  ev.Name,
				"value": value,
			}, nil
		}
	}

	// No match found, return numeric value only
	return map[string]interface{}{
		"value": value,
	}, nil
}

// dumpStruct formats a struct by dumping each member.
func (d *BTFDumper) dumpStruct(data []byte, t *btf.Struct) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(t.Members))

	for _, member := range t.Members {
		// Skip anonymous padding members
		if member.Name == "" {
			continue
		}

		// Handle bitfields specially
		if member.BitfieldSize > 0 {
			value, err := d.dumpBitfield(data, member)
			if err != nil {
				return nil, fmt.Errorf("bitfield %s: %w", member.Name, err)
			}
			result[member.Name] = value
			continue
		}

		// Regular member
		offset := member.Offset.Bytes()
		memberSize, err := btf.Sizeof(member.Type)
		if err != nil {
			return nil, fmt.Errorf("sizeof member %s: %w", member.Name, err)
		}

		// Check bounds
		if int(offset)+memberSize > len(data) {
			return nil, fmt.Errorf("member %s out of bounds (offset %d + size %d > data len %d)",
				member.Name, offset, memberSize, len(data))
		}

		memberData := data[offset : int(offset)+memberSize]
		value, err := d.DumpValue(memberData, member.Type)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", member.Name, err)
		}

		result[member.Name] = value
	}

	return result, nil
}

// dumpBitfield extracts and formats a bitfield value.
func (d *BTFDumper) dumpBitfield(data []byte, member btf.Member) (interface{}, error) {
	// member.Offset is in bits. Calculate byte and bit offsets.
	byteOffset := member.Offset.Bytes()
	bitOffset := uint64(member.Offset) % 8

	intType, ok := member.Type.(*btf.Int)
	if !ok {
		return nil, fmt.Errorf("bitfield type is not int")
	}

	// Check bounds
	if int(byteOffset)+int(intType.Size) > len(data) {
		return nil, fmt.Errorf("bitfield out of bounds")
	}

	// Read full integer
	intData := data[byteOffset : int(byteOffset)+int(intType.Size)]
	rawValue, err := d.readUnsignedInt(intData, intType.Size)
	if err != nil {
		return nil, err
	}

	// Extract bitfield
	mask := uint64((1 << member.BitfieldSize) - 1)
	value := (rawValue >> bitOffset) & mask

	// Apply signedness if needed
	if intType.Encoding == btf.Signed {
		// Sign extend if high bit is set
		if value&(1<<(member.BitfieldSize-1)) != 0 {
			value |= ^uint64(0) << member.BitfieldSize
			return int64(value), nil
		}
	}

	return value, nil
}

// dumpArray formats an array, with special handling for char arrays (strings).
func (d *BTFDumper) dumpArray(data []byte, t *btf.Array) (interface{}, error) {
	// Special case: char arrays (potential strings)
	if d.isCharArray(t) {
		return d.dumpCharArray(data, t.Nelems), nil
	}

	// Regular arrays
	elemSize, err := btf.Sizeof(t.Type)
	if err != nil {
		return nil, fmt.Errorf("sizeof array element: %w", err)
	}

	result := make([]interface{}, t.Nelems)

	for i := uint32(0); i < t.Nelems; i++ {
		offset := int(i) * int(elemSize)
		if offset+int(elemSize) > len(data) {
			return nil, fmt.Errorf("array element %d out of bounds", i)
		}

		elemData := data[offset : offset+int(elemSize)]
		value, err := d.DumpValue(elemData, t.Type)
		if err != nil {
			return nil, fmt.Errorf("array element %d: %w", i, err)
		}

		result[i] = value
	}

	return result, nil
}

// isCharArray checks if an array type is a char array (byte array).
func (d *BTFDumper) isCharArray(t *btf.Array) bool {
	typ := d.resolveType(t.Type)
	intType, ok := typ.(*btf.Int)
	if !ok {
		return false
	}
	return intType.Size == 1 && intType.Encoding == btf.Char
}

// dumpCharArray formats a char array as a null-terminated string.
func (d *BTFDumper) dumpCharArray(data []byte, length uint32) interface{} {
	// Find null terminator
	end := 0
	maxLen := int(length)
	if maxLen > len(data) {
		maxLen = len(data)
	}

	for end < maxLen && data[end] != 0 {
		end++
	}

	// Return as string if valid UTF-8
	str := string(data[:end])
	if utf8.ValidString(str) {
		return str
	}

	// Fall back to byte array for invalid UTF-8
	return bytesToHexArray(data[:maxLen])
}

// dumpUnion formats a union by showing all possible interpretations.
func (d *BTFDumper) dumpUnion(data []byte, t *btf.Union) (map[string]interface{}, error) {
	// Show all possible interpretations
	result := make(map[string]interface{}, len(t.Members))

	for _, member := range t.Members {
		// Skip anonymous padding members
		if member.Name == "" {
			continue
		}

		memberSize, err := btf.Sizeof(member.Type)
		if err != nil {
			continue
		}

		if int(memberSize) > len(data) {
			continue
		}

		memberData := data[:memberSize]
		value, err := d.DumpValue(memberData, member.Type)
		if err == nil {
			result[member.Name] = value
		}
	}

	return map[string]interface{}{"union": result}, nil
}

// bytesToHexArray converts a byte slice to an array of hex strings (fallback format).
func bytesToHexArray(data []byte) []string {
	result := make([]string, len(data))
	for i, b := range data {
		result[i] = fmt.Sprintf("0x%02x", b)
	}
	return result
}
