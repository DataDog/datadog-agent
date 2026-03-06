// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"unsafe"

	"github.com/dustin/go-humanize"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// formatLimits tracks formatting limits for log output.
type formatLimits struct {
	maxBytes           int
	maxCollectionItems int
	maxFields          int
}

const (
	maxLogLineBytes       = 8192
	maxLogCollectionItems = 3
	maxLogFieldCount      = 5
	unlimitedItems        = -1 // Sentinel value for no limit
)

// Formatting constants for consistent output.
const (
	formatUnavailable     = "{unavailable}"
	formatNil             = "nil"
	formatCycle           = "{cycle}"
	formatTruncated       = "{truncated}"
	formatEllipsis        = "..."
	formatEllipsisComma   = ", ..."
	formatEllipsisCommaRB = ", ...}"
	formatCommaSpace      = ", "
	formatColonSpace      = ": "
	formatEmptyMap        = "map[]"
	formatEmptySlice      = "[]"
	formatEmptyElement    = "{}"
)

// canWrite checks if we can write the specified number of bytes.
func (fl *formatLimits) canWrite(bytes int) bool {
	return fl.maxBytes >= bytes
}

// consume marks bytes as consumed.
func (fl *formatLimits) consume(bytes int) {
	if bytes < 0 {
		return
	}
	fl.maxBytes -= bytes
	if fl.maxBytes < 0 {
		fl.maxBytes = 0
	}
}

// writeBoundedString writes a string to the buffer if there's enough space.
// Returns true if the string was written, false otherwise.
func writeBoundedString(
	buf *bytes.Buffer, limits *formatLimits, s string,
) bool {
	if !limits.canWrite(len(s)) {
		return false
	}
	buf.WriteString(s)
	limits.consume(len(s))
	return true
}

// writeBoundedError writes an error message wrapped in braces, truncating the
// inner message if needed to preserve the braces.
func writeBoundedError(
	buf *bytes.Buffer, limits *formatLimits, prefix, msg string,
) bool {
	var errorMsg string
	if prefix == "" {
		// Format: "{message}"
		if !limits.canWrite(2) {
			return false
		}
		available := limits.maxBytes - 2
		if len(msg) > available {
			msg = msg[:available]
		}
		errorMsg = "{" + msg + "}"
	} else {
		// Format: "{prefix: message}"
		prefixLen := len(prefix) + 4 // "{prefix: }"
		if !limits.canWrite(prefixLen) {
			return false
		}
		available := limits.maxBytes - prefixLen
		if len(msg) > available {
			msg = msg[:available]
		}
		errorMsg = "{" + prefix + ": " + msg + "}"
	}
	buf.WriteString(errorMsg)
	limits.consume(len(errorMsg))
	return true
}

// writeBoundedFallback writes an error message when a type cannot be
// formatted. The message describes the specific failure mode.
func writeBoundedFallback(
	buf *bytes.Buffer, limits *formatLimits, msg string,
) bool {
	return writeBoundedError(buf, limits, "", msg)
}

// decoderType is a decoder-specific representation of an ir.Type. It is used
// so that specific types can implement their own encoding methods. We can
// track these types in the decoder as a way of caching type-specific
// information such as map key and value types.
type decoderType interface {
	irType() ir.Type
	encodeValueFields(
		c *encodingContext,
		enc *jsontext.Encoder,
		data []byte,
	) error
	formatValueFields(
		c *encodingContext,
		buf *bytes.Buffer,
		data []byte,
		limits *formatLimits,
	) error
}

type encodingContext struct {
	typesByID            map[ir.TypeID]decoderType
	typesByGoRuntimeType map[uint32]ir.TypeID
	currentlyEncoding    map[typeAndAddr]struct{}
	dataItems            map[typeAndAddr]output.DataItem
	typeResolver         TypeNameResolver
	missingTypeCollector MissingTypeCollector
}

// ResolveTypeName implements encodingContext.
func (e *encodingContext) ResolveTypeName(typeID gotype.TypeID) (string, error) {
	return e.typeResolver.ResolveTypeName(typeID)
}

// getPtr implements encodingContext.
func (e *encodingContext) getPtr(addr uint64, typeID ir.TypeID) (output.DataItem, bool) {
	di, ok := e.dataItems[typeAndAddr{addr: addr, irType: uint32(typeID)}]
	return di, ok
}

// getType implements encodingContext.
func (e *encodingContext) getType(typeID ir.TypeID) (decoderType, bool) {
	t, ok := e.typesByID[typeID]
	return t, ok
}

// getTypeIDByGoRuntimeType implements encodingContext.
func (e *encodingContext) getTypeIDByGoRuntimeType(runtimeType uint32) (ir.TypeID, bool) {
	typeID, ok := e.typesByGoRuntimeType[runtimeType]
	return typeID, ok
}

// recordPointer implements encodingContext.
func (e *encodingContext) recordPointer(addr uint64, typeID ir.TypeID) (release func(), ok bool) {
	key := typeAndAddr{addr: addr, irType: uint32(typeID)}
	_, ok = e.currentlyEncoding[typeAndAddr{addr: addr, irType: uint32(typeID)}]
	if ok {
		return nil, false
	}
	e.currentlyEncoding[typeAndAddr{addr: addr, irType: uint32(typeID)}] = struct{}{}
	return func() {
		delete(e.currentlyEncoding, key)
	}, true
}

// Type equivalent definitions
type baseType ir.BaseType
type pointerType ir.PointerType
type structureType ir.StructureType
type arrayType ir.ArrayType
type voidPointerType ir.VoidPointerType
type goSliceHeaderType ir.GoSliceHeaderType
type goSliceDataType ir.GoSliceDataType
type goStringHeaderType struct {
	*ir.GoStringHeaderType
	strFieldOffset uint32
	strFieldSize   uint32
	lenFieldOffset uint32
	lenFieldSize   uint32
}
type goStringDataType ir.GoStringDataType
type goMapType ir.GoMapType
type goHMapHeaderType struct {
	*ir.GoHMapHeaderType

	// Offsets and types for data in the header.
	countOffset      uint32
	bucketsTypeID    ir.TypeID
	bucketsOffset    uint32
	oldBucketsOffset uint32

	// Bucket type information.
	bucketTypeID   ir.TypeID
	bucketByteSize uint32
	tophashOfset   uint32
	keysOffset     uint32
	valuesOffset   uint32
	overflowOffset uint32

	// Key and value type information.
	keyTypeID     ir.TypeID
	keyTypeName   string
	keyTypeSize   uint32
	valueTypeID   ir.TypeID
	valueTypeSize uint32
	valueTypeName string
}

type goHMapBucketType ir.GoHMapBucketType
type goSwissMapHeaderType struct {
	*ir.GoSwissMapHeaderType
	// User-defined key and value type information
	keyTypeID           ir.TypeID
	keyTypeName         string
	keyTypeSize         uint32
	valueTypeID         ir.TypeID
	valueTypeName       string
	valueTypeSize       uint32
	keyFieldOffset      uint32
	valueFieldOffset    uint32
	slotsArrayEntrySize uint32

	// Internal Go swiss map representation fields
	dirPtrOffset     uint32
	dirPtrSize       uint32
	dirLenOffset     uint32
	dirLenSize       uint32
	usedOffset       uint32
	usedSize         uint32
	ctrlOffset       uint32
	ctrlSize         uint32
	slotsOffset      uint32
	slotsSize        uint32
	groupFieldOffset uint32
	groupFieldSize   uint32
	dataFieldOffset  uint32
	dataFieldSize    uint32
	tableTypeID      ir.TypeID
	groupTypeID      ir.TypeID
	groupSliceTypeID ir.TypeID
	elementTypeSize  uint32
}
type goSwissMapGroupsType ir.GoSwissMapGroupsType
type goChannelType ir.GoChannelType
type goEmptyInterfaceType ir.GoEmptyInterfaceType
type goInterfaceType ir.GoInterfaceType
type goSubroutineType ir.GoSubroutineType
type eventRootType ir.EventRootType
type unresolvedPointeeType ir.UnresolvedPointeeType

// Compile-time type assertions
var (
	_ decoderType = (*baseType)(nil)
	_ decoderType = (*pointerType)(nil)
	_ decoderType = (*structureType)(nil)
	_ decoderType = (*arrayType)(nil)
	_ decoderType = (*voidPointerType)(nil)
	_ decoderType = (*goSliceHeaderType)(nil)
	_ decoderType = (*goSliceDataType)(nil)
	_ decoderType = (*goStringHeaderType)(nil)
	_ decoderType = (*goStringDataType)(nil)
	_ decoderType = (*goMapType)(nil)
	_ decoderType = (*goHMapHeaderType)(nil)
	_ decoderType = (*goHMapBucketType)(nil)
	_ decoderType = (*goSwissMapGroupsType)(nil)
	_ decoderType = (*goChannelType)(nil)
	_ decoderType = (*goEmptyInterfaceType)(nil)
	_ decoderType = (*goInterfaceType)(nil)
	_ decoderType = (*goSubroutineType)(nil)
	_ decoderType = (*eventRootType)(nil)
	_ decoderType = (*unresolvedPointeeType)(nil)
)

func newDecoderType(
	irType ir.Type,
	types map[ir.TypeID]ir.Type,
) (decoderType, error) {
	switch s := irType.(type) {
	case *ir.GoSwissMapHeaderType:
		dirPtrField, err := getFieldByName(s.RawFields, "dirPtr")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		dirPtrOffset := dirPtrField.Offset
		dirPtrSize := dirPtrField.Type.GetByteSize()
		dirLenField, err := getFieldByName(s.RawFields, "dirLen")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		dirLenOffset := dirLenField.Offset
		dirLenSize := dirLenField.Type.GetByteSize()

		usedField, err := getFieldByName(s.RawFields, "used")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		usedOffset := usedField.Offset
		usedSize := usedField.Type.GetByteSize()

		slotsField, err := getFieldByName(s.GroupType.RawFields, "slots")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		slotsFieldType, ok := types[slotsField.Type.GetID()]
		if !ok {
			return nil, fmt.Errorf("type map slot field not found in types: %s", s.GroupType.Name)
		}
		ctrlField, err := getFieldByName(s.GroupType.RawFields, "ctrl")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		ctrlOffset := ctrlField.Offset
		ctrlSize := ctrlField.Type.GetByteSize()
		entryArray, ok := slotsFieldType.(*ir.ArrayType)
		if !ok {
			return nil, fmt.Errorf("type map slot field is not an array type: %s", slotsFieldType.GetName())
		}
		noalgstructType, ok := entryArray.Element.(*ir.StructureType)
		if !ok {
			return nil, fmt.Errorf("type map entry array element is not a structure type: %s", entryArray.Element.GetName())
		}
		keyField, err := getFieldByName(noalgstructType.RawFields, "key")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		if keyField == nil {
			return nil, fmt.Errorf("type map entry array element has no key field: %s", entryArray.Element.GetName())
		}
		elem, err := getFieldByName(noalgstructType.RawFields, "elem")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		tablePtrType := s.TablePtrSliceType.Element.(*ir.PointerType)
		tableType := tablePtrType.Pointee.(*ir.StructureType)
		groupField, err := getFieldByName(tableType.RawFields, "groups")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		groupType, ok := groupField.Type.(*ir.GoSwissMapGroupsType)
		if !ok {
			return nil, fmt.Errorf("group field type is not a swiss map groups type: %s", groupField.Type.GetName())
		}
		groupFieldOffset := groupField.Offset
		groupFieldSize := groupField.Type.GetByteSize()
		dataField, err := getFieldByName(groupType.RawFields, "data")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		dataFieldOffset := dataField.Offset
		dataFieldSize := dataField.Type.GetByteSize()

		keyFieldOffset := keyField.Offset
		valueFieldOffset := elem.Offset

		return &goSwissMapHeaderType{
			GoSwissMapHeaderType: s,
			// Fields related to user defined key and value types
			keyTypeID:           keyField.Type.GetID(),
			valueTypeID:         elem.Type.GetID(),
			keyTypeSize:         keyField.Type.GetByteSize(),
			valueTypeSize:       elem.Type.GetByteSize(),
			keyTypeName:         keyField.Type.GetName(),
			valueTypeName:       elem.Type.GetName(),
			slotsArrayEntrySize: noalgstructType.GetByteSize(),
			keyFieldOffset:      keyFieldOffset,
			valueFieldOffset:    valueFieldOffset,

			// Fields in go swiss map internal representation
			// See https://github.com/golang/go/blob/cd3655a8/src/internal/runtime/maps/map.go#L195
			dirLenOffset:     dirLenOffset,
			dirLenSize:       dirLenSize,
			dirPtrOffset:     dirPtrOffset,
			dirPtrSize:       dirPtrSize,
			ctrlOffset:       ctrlOffset,
			ctrlSize:         ctrlSize,
			slotsOffset:      slotsField.Offset,
			slotsSize:        slotsFieldType.GetByteSize(),
			groupFieldOffset: groupFieldOffset,
			groupFieldSize:   groupFieldSize,
			dataFieldOffset:  dataFieldOffset,
			dataFieldSize:    dataFieldSize,
			tableTypeID:      tablePtrType.Pointee.GetID(),
			groupTypeID:      s.GroupType.GetID(),
			groupSliceTypeID: groupType.GroupSliceType.GetID(),
			elementTypeSize:  uint32(groupType.GroupSliceType.Element.GetByteSize()),
			usedOffset:       usedOffset,
			usedSize:         usedSize,
		}, nil
	case *ir.BaseType:
		return (*baseType)(s), nil
	case *ir.StructureType:
		return (*structureType)(s), nil
	case *ir.ArrayType:
		return (*arrayType)(s), nil
	case *ir.GoSliceHeaderType:
		return (*goSliceHeaderType)(s), nil
	case *ir.VoidPointerType:
		return (*voidPointerType)(s), nil
	case *ir.PointerType:
		return (*pointerType)(s), nil
	case *ir.GoSliceDataType:
		return (*goSliceDataType)(s), nil
	case *ir.GoStringHeaderType:
		strField, err := getFieldByName(s.RawFields, "str")
		if err != nil {
			return nil, fmt.Errorf("malformed string header type: %w", err)
		}
		lenField, err := getFieldByName(s.RawFields, "len")
		if err != nil {
			return nil, fmt.Errorf("malformed string header type: %w", err)
		}
		return &goStringHeaderType{
			GoStringHeaderType: s,
			strFieldOffset:     strField.Offset,
			strFieldSize:       strField.Type.GetByteSize(),
			lenFieldOffset:     lenField.Offset,
			lenFieldSize:       lenField.Type.GetByteSize(),
		}, nil
	case *ir.GoStringDataType:
		return (*goStringDataType)(s), nil
	case *ir.GoMapType:
		return (*goMapType)(s), nil
	case *ir.GoHMapHeaderType:
		countField, err := getFieldByName(s.RawFields, "count")
		if err != nil {
			return nil, fmt.Errorf("malformed hmap header type: %w", err)
		}
		bucketsField, err := getFieldByName(s.RawFields, "buckets")
		if err != nil {
			return nil, fmt.Errorf("malformed hmap header type: %w", err)
		}
		oldBucketsField, err := getFieldByName(s.RawFields, "oldbuckets")
		if err != nil {
			return nil, fmt.Errorf("malformed hmap header type: %w", err)
		}
		topHashField, err := getFieldByName(s.BucketType.RawFields, "tophash")
		if err != nil {
			return nil, fmt.Errorf("malformed hmap header type: %w", err)
		}
		keysField, err := getFieldByName(s.BucketType.RawFields, "keys")
		if err != nil {
			return nil, fmt.Errorf("malformed hmap header type: %w", err)
		}
		valuesField, err := getFieldByName(s.BucketType.RawFields, "values")
		if err != nil {
			return nil, fmt.Errorf("malformed hmap header type: %w", err)
		}
		overflowField, err := getFieldByName(s.BucketType.RawFields, "overflow")
		if err != nil {
			return nil, fmt.Errorf("malformed hmap header type: %w", err)
		}
		return &goHMapHeaderType{
			GoHMapHeaderType: s,
			countOffset:      countField.Offset,
			bucketsTypeID:    s.BucketsType.GetID(),
			bucketsOffset:    bucketsField.Offset,
			oldBucketsOffset: oldBucketsField.Offset,
			bucketTypeID:     s.BucketType.GetID(),
			bucketByteSize:   s.BucketType.GetByteSize(),
			tophashOfset:     topHashField.Offset,
			keysOffset:       keysField.Offset,
			valuesOffset:     valuesField.Offset,
			overflowOffset:   overflowField.Offset,
			keyTypeID:        s.BucketType.KeyType.GetID(),
			keyTypeSize:      s.BucketType.KeyType.GetByteSize(),
			keyTypeName:      s.BucketType.KeyType.GetName(),
			valueTypeID:      s.BucketType.ValueType.GetID(),
			valueTypeSize:    s.BucketType.ValueType.GetByteSize(),
			valueTypeName:    s.BucketType.ValueType.GetName(),
		}, nil

	case *ir.GoHMapBucketType:
		return (*goHMapBucketType)(s), nil
	case *ir.GoSwissMapGroupsType:
		return (*goSwissMapGroupsType)(s), nil
	case *ir.GoChannelType:
		return (*goChannelType)(s), nil
	case *ir.GoEmptyInterfaceType:
		return (*goEmptyInterfaceType)(s), nil
	case *ir.GoInterfaceType:
		return (*goInterfaceType)(s), nil
	case *ir.GoSubroutineType:
		return (*goSubroutineType)(s), nil
	case *ir.EventRootType:
		return (*eventRootType)(s), nil
	case *ir.UnresolvedPointeeType:
		return (*unresolvedPointeeType)(s), nil
	default:
		return nil, fmt.Errorf("unknown type %s (%T)", irType.GetName(), irType)
	}
}

func (b *baseType) irType() ir.Type { return (*ir.BaseType)(b) }
func (b *baseType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	if err := writeTokens(enc,
		jsontext.String("value"),
	); err != nil {
		return err
	}
	kind, ok := b.GetGoKind()
	if !ok {
		return fmt.Errorf("no go kind for type %s (ID: %d)", b.GetName(), b.GetID())
	}
	switch kind {
	case reflect.Bool:
		if len(data) < 1 {
			return errors.New("passed data not long enough for bool")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatBool(data[0] == 1)))
	case reflect.Int:
		if len(data) < 8 {
			return errors.New("passed data not long enough for int")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint64(data)), 10)))
	case reflect.Int8:
		if len(data) < 1 {
			return errors.New("passed data not long enough for int8")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatInt(int64(int8(data[0])), 10)))
	case reflect.Int16:
		if len(data) < 2 {
			return errors.New("passed data not long enough for int16")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatInt(int64(int16(binary.NativeEndian.Uint16(data))), 10)))
	case reflect.Int32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for int32")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatInt(int64(int32(binary.NativeEndian.Uint32(data))), 10)))
	case reflect.Int64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for int64")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint64(data)), 10)))
	case reflect.Uint:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatUint(binary.NativeEndian.Uint64(data), 10)))
	case reflect.Uint8:
		if len(data) != 1 {
			return errors.New("passed data not long enough for uint8")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatUint(uint64(data[0]), 10)))
	case reflect.Uint16:
		if len(data) != 2 {
			return errors.New("passed data not long enough for uint16")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatUint(uint64(binary.NativeEndian.Uint16(data)), 10)))
	case reflect.Uint32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for uint32")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatUint(uint64(binary.NativeEndian.Uint32(data)), 10)))
	case reflect.Uint64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint64")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatUint(binary.NativeEndian.Uint64(data), 10)))
	case reflect.Uintptr:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uintptr")
		}
		return writeTokens(enc, jsontext.String("0x"+strconv.FormatUint(binary.NativeEndian.Uint64(data), 16)))
	case reflect.Float32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for float32")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatFloat(float64(math.Float32frombits(binary.NativeEndian.Uint32(data))), 'f', -1, 64)))
	case reflect.Float64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for float64")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatFloat(math.Float64frombits(binary.NativeEndian.Uint64(data)), 'f', -1, 64)))
	case reflect.Complex64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for complex64")
		}
		realBits := math.Float32frombits(binary.NativeEndian.Uint32(data[0:4]))
		imagBits := math.Float32frombits(binary.NativeEndian.Uint32(data[4:8]))
		return writeTokens(enc, jsontext.String(strconv.FormatComplex(complex(float64(realBits), float64(imagBits)), 'f', -1, 64)))
	case reflect.Complex128:
		if len(data) != 16 {
			return errors.New("passed data not long enough for complex128")
		}
		realBits := math.Float64frombits(binary.NativeEndian.Uint64(data[0:8]))
		imagBits := math.Float64frombits(binary.NativeEndian.Uint64(data[8:16]))
		return writeTokens(enc, jsontext.String(strconv.FormatComplex(complex(realBits, imagBits), 'f', -1, 64)))
	default:
		return fmt.Errorf("%s is not a base type", kind)
	}
}

func (b *baseType) formatValueFields(
	_ *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	kind, ok := b.GetGoKind()
	if !ok {
		if !writeBoundedFallback(
			buf, limits, "unknown kind for type "+b.GetName(),
		) {
			return nil
		}
		return nil
	}
	var output string
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val := readInt(data, b.ByteSize)
		output = strconv.FormatInt(val, 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val := readUint(data, b.ByteSize)
		output = strconv.FormatUint(val, 10)
	case reflect.Float32, reflect.Float64:
		val := readFloat(data, b.ByteSize)
		output = strconv.FormatFloat(val, 'g', -1, 64)
	case reflect.Bool:
		if len(data) > 0 && data[0] != 0 {
			output = "true"
		} else {
			output = "false"
		}
	default:
		writeBoundedFallback(
			buf, limits, fmt.Sprintf("unsupported kind %d for type %s", kind, b.GetName()),
		)
		return nil
	}
	writeBoundedString(buf, limits, output)
	return nil
}

func (e *eventRootType) irType() ir.Type { return (*ir.EventRootType)(e) }
func (e *eventRootType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (e *eventRootType) formatValueFields(
	_ *encodingContext,
	buf *bytes.Buffer,
	_ []byte,
	limits *formatLimits,
) error {
	writeBoundedFallback(buf, limits, "unimplemented")
	return nil
}

func (m *goMapType) irType() ir.Type { return (*ir.GoMapType)(m) }
func (m *goMapType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	const encodeAddress = false
	return encodePointer(c, data, encodeAddress, m.HeaderType.GetID(), enc)
}

func (m *goMapType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	// Format maps similar to pointers - delegate to formatPointer
	return formatPointer(c, buf, data, m.HeaderType.GetID(), m.HeaderType, limits)
}

func (h *goHMapHeaderType) irType() ir.Type { return h.GoHMapHeaderType }
func (h *goHMapHeaderType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	maxOffset := max(h.countOffset+8, h.bucketsOffset+8, h.oldBucketsOffset+8)
	if maxOffset > uint32(len(data)) {
		return errors.New("data is too short to contain all fields")
	}
	count := binary.NativeEndian.Uint64(data[h.countOffset : h.countOffset+8])
	return encodeMapEntries(enc, count, func() (int, error) {
		encodeBuckets := func(dataItem output.DataItem) (encodedItems int, err error) {
			data, ok := dataItem.Data()
			if !ok {
				return 0, nil
			}
			numBuckets := len(data) / int(h.bucketByteSize)
			for i := range numBuckets {
				bucketOffset := uint32(i) * h.bucketByteSize
				bucketData := data[bucketOffset : bucketOffset+h.bucketByteSize]
				bucketItems, err := encodeHMapBucket(c, enc, h, bucketData)
				if err != nil {
					// Return items encoded so far, not 0, to match swiss map behavior
					// and preserve the count for pruned logic.
					return encodedItems, fmt.Errorf("error encoding bucket: %w", err)
				}
				encodedItems += bucketItems
			}
			return encodedItems, nil
		}
		var encodedItems int
		for _, offset := range []uint32{h.bucketsOffset, h.oldBucketsOffset} {
			addr := binary.NativeEndian.Uint64(data[offset : offset+8])
			if addr == 0 {
				continue
			}
			item, ok := c.getPtr(addr, h.bucketsTypeID)
			if !ok {
				continue
			}
			items, err := encodeBuckets(item)
			if err != nil {
				// Return items encoded so far, not 0, to match swiss map behavior
				// and preserve the count for pruned logic.
				return encodedItems, err
			}
			encodedItems += items
		}
		return encodedItems, nil
	})
}

func (h *goHMapHeaderType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	maxOffset := max(h.countOffset+8, h.bucketsOffset+8, h.oldBucketsOffset+8)
	count := binary.NativeEndian.Uint64(data[h.countOffset : h.countOffset+8])
	return formatMapEntries(buf, limits, count, "map", maxOffset, len(data), func() (int, error) {
		var formattedItems int
		maxItems := limits.maxCollectionItems
		formatBuckets := func(dataItem output.DataItem) (items int, err error) {
			if formattedItems >= maxItems {
				return items, nil
			}
			data, ok := dataItem.Data()
			if !ok {
				return 0, nil
			}
			numBuckets := len(data) / int(h.bucketByteSize)
			for i := range numBuckets {
				if formattedItems >= maxItems {
					break
				}
				bucketOffset := uint32(i) * h.bucketByteSize
				bucketData := data[bucketOffset : bucketOffset+h.bucketByteSize]
				bucketItems, err := formatHMapBucket(
					c, buf, h, bucketData, formattedItems > 0, limits,
				)
				if err != nil {
					return items, err
				}
				items += bucketItems
				formattedItems += bucketItems
				if formattedItems >= maxItems {
					break
				}
			}
			return items, nil
		}

		for _, offset := range []uint32{h.bucketsOffset, h.oldBucketsOffset} {
			if formattedItems >= maxItems {
				break
			}
			addr := binary.NativeEndian.Uint64(data[offset : offset+8])
			if addr == 0 {
				continue
			}
			item, ok := c.getPtr(addr, h.bucketsTypeID)
			if !ok {
				continue
			}
			_, err := formatBuckets(item)
			if err != nil {
				return 0, err
			}
			if formattedItems >= maxItems {
				break
			}
		}
		return formattedItems, nil
	})
}

// mapEntryCallback processes a single map entry (key/value pair).
// Returns true if processing should continue, false to stop early.
type mapEntryCallback func(
	keyData []byte,
	valueData []byte,
	_index int,
) (shouldContinue bool, err error)

// shouldStop checks if iteration should stop based on maxItems limit.
func shouldStop(maxItems, processed int) bool {
	return maxItems != unlimitedItems && processed >= maxItems
}

// encodeMapEntries wraps the common pattern for encoding map entries:
// writes size, entries BeginArray, calls iterateFn, writes EndArray,
// and writes pruned token if encoded items < count.
func encodeMapEntries(
	enc *jsontext.Encoder,
	count uint64,
	iterateFn func() (encodedItems int, err error),
) error {
	if err := writeTokens(enc,
		jsontext.String("size"),
		jsontext.String(strconv.FormatUint(count, 10)),
	); err != nil {
		return err
	}
	if err := writeTokens(
		enc, jsontext.String("entries"), jsontext.BeginArray,
	); err != nil {
		return err
	}
	encodedItems, err := iterateFn()
	if err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	if uint64(encodedItems) < count {
		if err := writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonPruned,
		); err != nil {
			return err
		}
	}
	return nil
}

// formatMapEntries wraps the common pattern for formatting map entries:
// validates bounds, checks empty, writes map[ prefix, calls iterateFn,
// writes ellipsis if truncated, and writes closing ].
func formatMapEntries(
	buf *bytes.Buffer,
	limits *formatLimits,
	count uint64,
	mapName string,
	maxOffset uint32,
	dataLen int,
	iterateFn func() (formattedItems int, err error),
) error {
	if maxOffset > uint32(dataLen) {
		writeBoundedError(buf, limits, mapName, "data too short")
		return nil
	}
	if count == 0 {
		writeBoundedString(buf, limits, formatEmptyMap)
		return nil
	}

	beforeLen := buf.Len()
	mapPrefix := "map["
	if !writeBoundedString(buf, limits, mapPrefix) {
		return nil
	}

	formattedItems, err := iterateFn()
	if err != nil {
		return err
	}

	if uint64(formattedItems) < count {
		writeBoundedString(buf, limits, formatEllipsisComma)
	}

	closing := "]"
	if !writeBoundedString(buf, limits, closing) {
		buf.Truncate(beforeLen)
	}
	return nil
}

// makeFormatMapEntryCallback creates a callback for formatting map entries.
func makeFormatMapEntryCallback(
	c *encodingContext,
	buf *bytes.Buffer,
	limits *formatLimits,
	needComma bool,
	keyType ir.Type,
	valueType ir.Type,
) mapEntryCallback {
	itemsBefore := 0
	return func(keyData []byte, valueData []byte, _index int) (bool, error) {
		if needComma || itemsBefore > 0 {
			if !writeBoundedString(buf, limits, formatCommaSpace) {
				return false, nil
			}
		}
		itemsBefore++
		keyBeforeLen := buf.Len()
		if err := formatType(c, buf, keyType, keyData, limits); err != nil {
			return false, err
		}
		keyWritten := buf.Len() - keyBeforeLen
		limits.consume(keyWritten)

		if !writeBoundedString(buf, limits, formatColonSpace) {
			return false, nil
		}

		valueBeforeLen := buf.Len()
		if err := formatType(c, buf, valueType, valueData, limits); err != nil {
			return false, err
		}
		valueWritten := buf.Len() - valueBeforeLen
		limits.consume(valueWritten)
		return true, nil
	}
}

// makeEncodeMapEntryCallback creates a callback for encoding map entries.
func makeEncodeMapEntryCallback(
	c *encodingContext,
	enc *jsontext.Encoder,
	keyTypeID ir.TypeID,
	keyTypeName string,
	valueTypeID ir.TypeID,
	valueTypeName string,
) mapEntryCallback {
	return func(keyData []byte, valueData []byte, _index int) (bool, error) {
		if err := writeTokens(enc, jsontext.BeginArray); err != nil {
			return false, err
		}
		if err := encodeValue(c, enc, keyTypeID, keyData, keyTypeName); err != nil {
			return false, err
		}
		if err := encodeValue(c, enc, valueTypeID, valueData, valueTypeName); err != nil {
			return false, err
		}
		if err := writeTokens(enc, jsontext.EndArray); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (b *goHMapBucketType) irType() ir.Type { return (*ir.GoHMapBucketType)(b) }
func (*goHMapBucketType) encodeValueFields(
	*encodingContext, *jsontext.Encoder, []byte,
) error {
	return errors.New("hmap bucket type is never directly encoded")
}

func (*goHMapBucketType) formatValueFields(
	*encodingContext, *bytes.Buffer, []byte, *formatLimits,
) error {
	return errors.New("hmap bucket type is never directly formatted")
}

func (s *goSwissMapHeaderType) irType() ir.Type { return s.GoSwissMapHeaderType }
func (s *goSwissMapHeaderType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	used := binary.NativeEndian.Uint64(data[s.usedOffset : s.usedOffset+uint32(s.usedSize)])
	dirLen := int64(binary.NativeEndian.Uint64(data[s.dirLenOffset : s.dirLenOffset+uint32(s.dirLenSize)]))
	dirPtr := binary.NativeEndian.Uint64(data[s.dirPtrOffset : s.dirPtrOffset+uint32(s.dirPtrSize)])
	return encodeMapEntries(enc, used, func() (int, error) {
		if dirLen == 0 {
			// Small swiss map with a single group.
			groupDataItem, ok := c.getPtr(dirPtr, s.groupTypeID)
			if !ok {
				// Write not captured reason inside entries array.
				if err := writeTokens(enc,
					tokenNotCapturedReason,
					tokenNotCapturedReasonDepth,
				); err != nil {
					return 0, err
				}
				return 0, nil
			}
			groupData, ok := groupDataItem.Data()
			if !ok {
				// Write not captured reason inside entries array.
				if err := writeTokens(enc,
					tokenNotCapturedReason,
					tokenNotCapturedReasonUnavailable,
				); err != nil {
					return 0, err
				}
				return 0, nil
			}
			return s.encodeSwissMapGroup(c, enc, groupData)
		}
		// Large swiss map with multiple groups.
		tablePtrSliceDataItem, ok := c.getPtr(dirPtr, s.TablePtrSliceType.GetID())
		if !ok {
			// Write not captured reason inside entries array.
			if err := writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonDepth,
			); err != nil {
				return 0, err
			}
			return 0, nil
		}
		tablePtrSliceData, ok := tablePtrSliceDataItem.Data()
		if !ok {
			// Write not captured reason inside entries array.
			if err := writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonUnavailable,
			); err != nil {
				return 0, err
			}
			return 0, nil
		}
		return s.encodeSwissMapTables(c, enc, tablePtrSliceData)
	})
}

func (s *goSwissMapHeaderType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	maxOffset := max(
		s.usedOffset+uint32(s.usedSize),
		s.dirLenOffset+uint32(s.dirLenSize),
		s.dirPtrOffset+uint32(s.dirPtrSize),
	)
	used := binary.NativeEndian.Uint64(
		data[s.usedOffset : s.usedOffset+uint32(s.usedSize)],
	)
	dirLen := binary.NativeEndian.Uint64(
		data[s.dirLenOffset : s.dirLenOffset+uint32(s.dirLenSize)],
	)
	dirPtr := binary.NativeEndian.Uint64(
		data[s.dirPtrOffset : s.dirPtrOffset+uint32(s.dirPtrSize)],
	)
	return formatMapEntries(buf, limits, used, "swiss map", maxOffset, len(data), func() (int, error) {
		if dirLen == 0 {
			// Small swiss map with a single group.
			groupDataItem, ok := c.getPtr(dirPtr, s.groupTypeID)
			if !ok {
				// formatMapEntries will handle truncation if we return 0 items.
				writeBoundedError(buf, limits, "swiss map", "failed to capture group")
				return 0, nil
			}
			groupData, ok := groupDataItem.Data()
			if !ok {
				writeBoundedError(buf, limits, "swiss map", "failed to read group")
				return 0, nil
			}
			return s.formatSwissMapGroup(c, buf, groupData, false, limits)
		}
		// Large swiss map with multiple groups.
		tablePtrSliceDataItem, ok := c.getPtr(dirPtr, s.TablePtrSliceType.GetID())
		if !ok {
			writeBoundedError(buf, limits, "swiss map", "failed to capture tables")
			return 0, nil
		}
		tablePtrSliceData, ok := tablePtrSliceDataItem.Data()
		if !ok {
			writeBoundedError(buf, limits, "swiss map", "failed to read tables")
			return 0, nil
		}
		return s.formatSwissMapTables(c, buf, tablePtrSliceData, limits)
	})
}

func (s *goSwissMapGroupsType) irType() ir.Type { return (*ir.GoSwissMapGroupsType)(s) }
func (s *goSwissMapGroupsType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (s *goSwissMapGroupsType) formatValueFields(
	_ *encodingContext,
	buf *bytes.Buffer,
	_ []byte,
	limits *formatLimits,
) error {
	writeBoundedFallback(buf, limits, "unimplemented")
	return nil
}

func (v *voidPointerType) irType() ir.Type { return (*ir.VoidPointerType)(v) }
func (v *voidPointerType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	if len(data) != 8 {
		return errors.New("passed data not long enough for void pointer")
	}
	return writeTokens(enc,
		jsontext.String("address"),
		jsontext.String("0x"+strconv.FormatUint(binary.NativeEndian.Uint64(data), 16)),
	)
}

func (v *voidPointerType) formatValueFields(
	_ *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	if len(data) < 8 {
		writeBoundedError(buf, limits, "void pointer", "truncated")
	}
	addr := binary.NativeEndian.Uint64(data)
	output := "0x" + strconv.FormatUint(addr, 16)
	writeBoundedString(buf, limits, output)
	return nil
}

func (p *pointerType) irType() ir.Type { return (*ir.PointerType)(p) }
func (p *pointerType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	// We only encode the address for non-pointer types to avoid collisions of the 'address' field
	// in cases of pointers to pointers. In a scenario like `**int`, only the final pointer that's
	// closest to the actual data will be encoded.
	//
	// For things like map buckets or channel internals, which we encode as pointers, we won't
	// find a go kind.
	goKind, ok := p.Pointee.GetGoKind()
	writeAddress := ok && goKind != reflect.Pointer
	return encodePointer(c, data, writeAddress, p.Pointee.GetID(), enc)
}

func (p *pointerType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	return formatPointer(c, buf, data, p.Pointee.GetID(), p.Pointee, limits)
}

func formatPointer(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	pointee ir.TypeID,
	pointeeType ir.Type,
	limits *formatLimits,
) error {
	if len(data) < 8 {
		writeBoundedError(buf, limits, "pointer", "truncated")
	}
	addr := binary.NativeEndian.Uint64(data)
	if addr == 0 {
		writeBoundedString(buf, limits, formatNil)
		return nil
	}

	// Use encodingContext.recordPointer for cycle detection.
	if release, ok := c.recordPointer(addr, pointee); ok {
		defer release()

		// Look up pointed-to data.
		item, ok := c.getPtr(addr, pointee)
		if !ok {
			msg := fmt.Sprintf("not captured at 0x%x", addr)
			writeBoundedError(buf, limits, "pointer", msg)
			return nil
		}

		pointeeData, ok := item.Data()
		if !ok {
			msg := fmt.Sprintf("read failed at 0x%x", addr)
			writeBoundedError(buf, limits, "pointer", msg)
			return nil
		}

		// Dereference and format.
		return formatType(c, buf, pointeeType, pointeeData, limits)
	}

	// Cycle detected.
	writeBoundedString(buf, limits, formatCycle)
	return nil
}

func encodePointer(
	c *encodingContext,
	data []byte,
	writeAddress bool,
	pointee ir.TypeID,
	enc *jsontext.Encoder,
) error {
	if len(data) < 8 {
		return errors.New("passed data not long enough for pointer: need 8 bytes")
	}
	addr := binary.NativeEndian.Uint64(data)
	pointeeKey := typeAndAddr{
		irType: uint32(pointee),
		addr:   addr,
	}
	if pointeeKey.addr == 0 {
		if err := writeTokens(enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		); err != nil {
			return err
		}
		return nil
	}

	pointeeType, ok := c.getType(pointee)
	if !ok {
		return fmt.Errorf("no decoder type found for pointee type (ID: %d)", pointee)
	}

	// If the pointee type has zero size, we don't expect there to be a data
	// item for it.
	var (
		pointedValue   output.DataItem
		dataItemExists bool
	)
	isZeroSized := pointeeType.irType().GetByteSize() == 0
	if !isZeroSized {
		pointedValue, dataItemExists = c.getPtr(addr, pointee)
	} else {
		dataItemExists = true
	}
	if !dataItemExists {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonDepth,
		)
	}
	if writeAddress {
		if err := writeTokens(enc,
			jsontext.String("address"),
			jsontext.String("0x"+strconv.FormatUint(addr, 16)),
		); err != nil {
			return err
		}
	}

	if release, ok := c.recordPointer(addr, pointee); ok {
		defer release()
		var pointedData []byte
		if !isZeroSized {
			if pointedData, ok = pointedValue.Data(); !ok {
				return writeTokens(enc,
					tokenNotCapturedReason,
					tokenNotCapturedReasonUnavailable,
				)
			}
		}
		if err := pointeeType.encodeValueFields(c, enc, pointedData); err != nil {
			return fmt.Errorf("could not encode referenced value: %w", err)
		}
	} else {
		// If we're already encoding this value, we've hit a cycle and want to write a not captured reason
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonCycle,
		)
	}
	return nil
}

func (s *structureType) irType() ir.Type { return (*ir.StructureType)(s) }
func (s *structureType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	if err := writeTokens(enc,
		jsontext.String("fields"),
		jsontext.BeginObject); err != nil {
		return err
	}
	for field := range s.irType().(*ir.StructureType).Fields() {
		if err := writeTokens(enc, jsontext.String(field.Name)); err != nil {
			return err
		}
		fieldEnd := field.Offset + field.Type.GetByteSize()
		if fieldEnd > uint32(len(data)) {
			return fmt.Errorf(
				"field %s extends beyond data bounds: need %d bytes, have %d",
				field.Name, fieldEnd, len(data),
			)
		}

		fieldData := data[field.Offset : field.Offset+field.Type.GetByteSize()]
		if err := encodeValue(
			c, enc, field.Type.GetID(), fieldData, field.Type.GetName(),
		); err != nil {
			return err
		}
	}
	return writeTokens(enc, jsontext.EndObject)
}

func (s *structureType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	beforeLen := buf.Len()
	if !limits.canWrite(5) {
		return nil
	}
	buf.WriteByte('{')

	fieldCount := 0
	first := true
	for field := range s.irType().(*ir.StructureType).Fields() {
		if fieldCount >= limits.maxFields {
			// Check if we can write ellipsis.
			writeBoundedString(buf, limits, formatEllipsisCommaRB)
			break
		}

		if !first {
			if !writeBoundedString(buf, limits, formatCommaSpace) {
				buf.Truncate(beforeLen)
				return nil
			}
		}
		first = false

		fieldName := field.Name + ": "
		if !limits.canWrite(len(fieldName)) {
			buf.Truncate(beforeLen)
			return nil
		}
		buf.WriteString(fieldName)
		limits.consume(len(fieldName))

		fieldEnd := field.Offset + field.Type.GetByteSize()
		if fieldEnd > uint32(len(data)) {
			if !writeBoundedString(buf, limits, formatTruncated) {
				buf.Truncate(beforeLen)
				return nil
			}
			fieldCount++
			continue
		}

		fieldData := data[field.Offset:fieldEnd]
		fieldBeforeLen := buf.Len()
		if err := formatType(
			c, buf, field.Type, fieldData, limits,
		); err != nil {
			return err
		}
		fieldWritten := buf.Len() - fieldBeforeLen
		limits.consume(fieldWritten)
		fieldCount++
	}

	if !limits.canWrite(1) {
		buf.Truncate(beforeLen)
		return nil
	}
	buf.WriteByte('}')
	limits.consume(1)
	return nil
}

func (a *arrayType) irType() ir.Type { return (*ir.ArrayType)(a) }
func (a *arrayType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	var err error
	elementSize := int(a.Element.GetByteSize())
	numElements := int(a.Count)
	if err = writeTokens(enc,
		jsontext.String("size"),
		jsontext.String(strconv.FormatInt(int64(numElements), 10)),
		jsontext.String("elements"),
		jsontext.BeginArray); err != nil {
		return err
	}

	var notCaptured = false
	elementID := a.Element.GetID()
	elementName := a.Element.GetName()
	for i := range numElements {
		offset := i * elementSize
		endIdx := offset + elementSize
		if endIdx > len(data) {
			notCaptured = true
			break
		}
		if err := encodeValue(
			c, enc, elementID, data[offset:endIdx], elementName,
		); err != nil {
			return err
		}
	}
	if err := writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	if notCaptured {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonPruned,
		)
	}
	return nil
}

func (a *arrayType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	elementSize := int(a.Element.GetByteSize())
	numElements := int(a.Count)

	beforeLen := buf.Len()
	if !limits.canWrite(1) {
		return nil
	}
	buf.WriteByte('[')
	limits.consume(1)

	maxItems := limits.maxCollectionItems
	if maxItems > numElements {
		maxItems = numElements
	}

	for i := 0; i < maxItems; i++ {
		if i > 0 {
			if !writeBoundedString(buf, limits, formatCommaSpace) {
				buf.Truncate(beforeLen)
				return nil
			}
		}
		offset := i * elementSize
		endIdx := offset + elementSize
		if endIdx > len(data) {
			if !writeBoundedString(buf, limits, "...") {
				buf.Truncate(beforeLen)
				return nil
			}
			break
		}
		itemBeforeLen := buf.Len()
		if err := formatType(
			c, buf, a.Element, data[offset:endIdx], limits,
		); err != nil {
			return err
		}
		itemWritten := buf.Len() - itemBeforeLen
		limits.consume(itemWritten)
	}

	if numElements > maxItems {
		writeBoundedString(buf, limits, formatEllipsisComma)
	}

	if !limits.canWrite(1) {
		buf.Truncate(beforeLen)
		return nil
	}
	buf.WriteByte(']')
	limits.consume(1)
	return nil
}

func (s *goSliceHeaderType) irType() ir.Type { return (*ir.GoSliceHeaderType)(s) }
func (s *goSliceHeaderType) encodeValueFields(
	c *encodingContext, enc *jsontext.Encoder, data []byte,
) error {
	if len(data) < int(s.ByteSize) {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonPruned,
		)
	}
	if len(data) < 16 {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonPruned,
		)
	}
	address := binary.NativeEndian.Uint64(data[0:8])
	if address == 0 {
		return writeTokens(enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		)
	}
	length := binary.NativeEndian.Uint64(data[8:16])
	if err := writeTokens(enc,
		jsontext.String("size"),
		jsontext.String(strconv.FormatInt(int64(length), 10)),
	); err != nil {
		return err
	}
	if length == 0 {
		return writeTokens(enc,
			jsontext.String("elements"),
			jsontext.BeginArray,
			jsontext.EndArray)
	}

	elementSize := int(s.Data.Element.GetByteSize())
	var sliceData []byte
	var displayLen int
	if elementSize > 0 {
		sliceDataItem, ok := c.getPtr(address, s.Data.GetID())
		if !ok {
			return writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonPruned,
			)
		}
		sliceData, ok = sliceDataItem.Data()
		if !ok {
			return writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonUnavailable,
			)
		}
		// We might have captured less data then the length, due to max capture limits.
		// We might have captured more data then the length, due to multiple variables
		// aliasing the same underlying buffer (for now we capture as much data as the length
		// of the first variable pointing to the buffer).
		displayLen = min(int(len(sliceData))/elementSize, int(length))
	} else {
		displayLen = int(length)
	}
	if err := writeTokens(enc,
		jsontext.String("elements"),
		jsontext.BeginArray); err != nil {
		return err
	}
	elementByteSize := int(s.Data.Element.GetByteSize())
	elementName := s.Data.Element.GetName()
	elementID := s.Data.Element.GetID()
	for i := range int(displayLen) {
		var elementData []byte
		if elementSize > 0 {
			elementData = sliceData[i*elementByteSize : (i+1)*elementByteSize]
		}
		if err := encodeValue(
			c, enc, elementID, elementData, elementName,
		); err != nil {
			return fmt.Errorf(
				"could not encode %s slice element of %s: %w",
				humanize.Ordinal(i+1), elementName, err,
			)
		}
	}

	if err := writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	if length > uint64(displayLen) {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonCollectionSize,
		)
	}
	return nil
}

func (s *goSliceHeaderType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	if len(data) < 24 {
		writeBoundedError(buf, limits, "slice header", "data too short")
		return nil
	}

	// Read slice header: ptr, len, cap.
	ptr := binary.NativeEndian.Uint64(data[0:8])
	length := binary.NativeEndian.Uint64(data[8:16])

	if length == 0 {
		writeBoundedString(buf, limits, formatEmptySlice)
		return nil
	}

	// Limit display length to maxCollectionItems.
	displayLen := length
	maxItems := uint64(limits.maxCollectionItems)
	if displayLen > maxItems {
		displayLen = maxItems
	}

	elemSize := s.Data.Element.GetByteSize()
	var sliceData []byte
	if elemSize > 0 {
		// Look up slice data using encodingContext.
		item, ok := c.getPtr(ptr, s.Data.GetID())
		if !ok {
			msg := fmt.Sprintf("failed to capture slice: %d elements", length)
			writeBoundedError(buf, limits, "", msg)
			return nil
		}

		sliceData, ok = item.Data()
		if !ok {
			writeBoundedError(buf, limits, "failed to capture slice data", "read failed")
			return nil
		}
	}

	beforeLen := buf.Len()
	if !limits.canWrite(1) {
		return nil
	}
	buf.WriteByte('[')
	limits.consume(1)

	for i := uint64(0); i < displayLen; i++ {
		if i > 0 {
			if !writeBoundedString(buf, limits, formatCommaSpace) {
				buf.Truncate(beforeLen)
				return nil
			}
		}

		var elemData []byte
		if elemSize == 0 {
			if !writeBoundedString(buf, limits, formatEmptyElement) {
				buf.Truncate(beforeLen)
				return nil
			}
		} else {
			// Check for overflow before multiplication.
			if elemSize > 0 && i > math.MaxUint64/uint64(elemSize) {
				if !writeBoundedString(buf, limits, formatEllipsis) {
					buf.Truncate(beforeLen)
					return nil
				}
				break
			}
			elemStart := i * uint64(elemSize)
			elemEnd := elemStart + uint64(elemSize)
			if elemEnd > uint64(len(sliceData)) || elemEnd < elemStart {
				if !writeBoundedString(buf, limits, formatEllipsis) {
					buf.Truncate(beforeLen)
					return nil
				}
				break
			}
			elemData = sliceData[elemStart:elemEnd]
		}
		if elemSize > 0 {
			itemBeforeLen := buf.Len()
			if err := formatType(
				c, buf, s.Data.Element, elemData, limits,
			); err != nil {
				return err
			}
			itemWritten := buf.Len() - itemBeforeLen
			limits.consume(itemWritten)
		}
	}

	if length > displayLen {
		if limits.canWrite(len(formatEllipsisComma)) {
			buf.WriteString(formatEllipsisComma)
			limits.consume(len(formatEllipsisComma))
		}
	}

	if !limits.canWrite(1) {
		buf.Truncate(beforeLen)
		return nil
	}
	buf.WriteByte(']')
	limits.consume(1)
	return nil
}

func (s *goSliceDataType) irType() ir.Type { return (*ir.GoSliceDataType)(s) }
func (s *goSliceDataType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (s *goSliceDataType) formatValueFields(
	_ *encodingContext,
	buf *bytes.Buffer,
	_ []byte,
	limits *formatLimits,
) error {
	writeBoundedFallback(buf, limits, "unimplemented")
	return nil
}

func (s *goStringHeaderType) irType() ir.Type { return s.GoStringHeaderType }
func (s *goStringHeaderType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	fieldEnd := s.strFieldOffset + uint32(s.strFieldSize)
	if fieldEnd >= uint32(len(data)) {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonLength,
		)
	}
	strLen := binary.NativeEndian.Uint64(data[s.lenFieldOffset : s.lenFieldOffset+uint32(s.lenFieldSize)])
	address := binary.NativeEndian.Uint64(data[s.strFieldOffset : s.strFieldOffset+uint32(s.strFieldSize)])
	if address == 0 || strLen == 0 {
		return writeTokens(enc,
			jsontext.String("value"),
			jsontext.String(""),
		)
	}
	stringValue, ok := c.getPtr(address, s.Data.GetID())
	if !ok {
		return writeTokens(enc,
			jsontext.String("size"),
			jsontext.String(strconv.FormatInt(int64(strLen), 10)),
			tokenNotCapturedReason,
			tokenNotCapturedReasonDepth,
		)
	}
	// See notes about slice serialization for possible differences between captured and actual length.
	stringData, ok := stringValue.Data()
	if !ok {
		// The string data was corrupted, report it as unavailable.
		return writeTokens(enc,
			jsontext.String("size"),
			jsontext.String(strconv.FormatInt(int64(strLen), 10)),
			tokenNotCapturedReason,
			tokenNotCapturedReasonUnavailable,
		)
	}
	length := stringValue.Header().Length
	if strLen > uint64(length) {
		// We captured partial data for the string, report truncation.
		if err := writeTokens(enc,
			jsontext.String("size"),
			jsontext.String(strconv.FormatInt(int64(strLen), 10)),
			tokenTruncated,
			jsontext.Bool(true),
		); err != nil {
			return err
		}
	}
	if err := writeTokens(enc, jsontext.String("value")); err != nil {
		return err
	}
	str := unsafe.String(unsafe.SliceData(stringData), min(int(length), int(strLen)))
	return writeTokens(enc, jsontext.String(str))
}

func (s *goStringHeaderType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	if len(data) < 16 {
		writeBoundedError(buf, limits, "string header", "data too short")
		return nil
	}

	// Read string pointer and length from header.
	ptr := binary.NativeEndian.Uint64(data[0:8])
	length := binary.NativeEndian.Uint64(data[8:16])

	// Empty string is formatted as literally just 0 bytes.
	if ptr == 0 || length == 0 {
		return nil
	}

	// Look up string data using encodingContext.
	item, ok := c.getPtr(ptr, s.Data.GetID())
	if !ok {
		writeBoundedError(buf, limits, "string", "failed to capture string data")
		return nil
	}

	strData, ok := item.Data()
	if !ok {
		writeBoundedError(buf, limits, "string", "failed to capture string data")
		return nil
	}

	// We can only display as much data as was collected, and up to the limits.
	displayLen := min(int(length), len(strData), limits.maxBytes)
	if displayLen == int(length) {
		// We can just display the whole string.
		writeBoundedString(buf, limits, string(strData[:displayLen]))
		return nil
	}
	// We display truncated string with ellipsis if possible, nothing otherwise.
	if limits.maxBytes > len(formatEllipsis) {
		str := string(strData[:min(displayLen, limits.maxBytes-len(formatEllipsis))]) + formatEllipsis
		writeBoundedString(buf, limits, str)
	}
	return nil
}

func (s *goStringDataType) irType() ir.Type { return (*ir.GoStringDataType)(s) }
func (s *goStringDataType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}
func (s *goStringDataType) formatValueFields(
	*encodingContext, *bytes.Buffer, []byte, *formatLimits,
) error {
	return errors.New("string data is not formatted")
}

func (c *goChannelType) irType() ir.Type { return (*ir.GoChannelType)(c) }
func (c *goChannelType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}
func (c *goChannelType) formatValueFields(
	_ *encodingContext, buf *bytes.Buffer, _ []byte, limits *formatLimits,
) error {
	writeBoundedString(buf, limits, "{chan}")
	return nil
}

const goRuntimeTypeOffset = 0x00
const goInterfaceDataOffset = 0x08

func (e *goEmptyInterfaceType) irType() ir.Type { return (*ir.GoEmptyInterfaceType)(e) }
func (e *goEmptyInterfaceType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	return encodeInterface(c, enc, data)
}
func (e *goEmptyInterfaceType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	return formatInterface(c, buf, data, limits)
}

func (i *goInterfaceType) irType() ir.Type { return (*ir.GoInterfaceType)(i) }
func (i *goInterfaceType) encodeValueFields(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	return encodeInterface(c, enc, data)
}

func (i *goInterfaceType) formatValueFields(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	return formatInterface(c, buf, data, limits)
}

func encodeInterface(
	c *encodingContext,
	enc *jsontext.Encoder,
	data []byte,
) error {
	if len(data) != 16 {
		return fmt.Errorf("go interface data must be 16 bytes, got %d", len(data))
	}

	runtimeTypeData := data[goRuntimeTypeOffset : goRuntimeTypeOffset+8]
	runtimeType := binary.NativeEndian.Uint64(runtimeTypeData)
	if runtimeType == 0 {
		return writeTokens(enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		)
	}

	if err := writeTokens(enc,
		jsontext.String("fields"),
		jsontext.BeginObject,
		jsontext.String("data"),
		jsontext.BeginObject,
	); err != nil {
		return err
	}

	typeID, ok := c.getTypeIDByGoRuntimeType(uint32(runtimeType))
	if !ok {
		name, err := c.ResolveTypeName(gotype.TypeID(runtimeType))
		if err != nil {
			name = fmt.Sprintf(
				"UnknownType(0x%x): %v", runtimeType, err,
			)
		} else if c.missingTypeCollector != nil {
			c.missingTypeCollector.RecordMissingType(name)
		}
		if err := writeTokens(enc,
			jsontext.String("type"),
			jsontext.String(name),
			tokenNotCapturedReason,
			tokenNotCapturedReasonMissingTypeInfo,
			jsontext.EndObject,
			jsontext.EndObject,
		); err != nil {
			return err
		}
		return nil
	}
	// We know the concrete type; include it even for dynamic interfaces.
	t, ok := c.getType(typeID)
	if !ok {
		return fmt.Errorf("no type found for type ID: %d", typeID)
	}
	tt := t.irType()
	if err := writeTokens(
		enc, jsontext.String("type"), jsontext.String(tt.GetName()),
	); err != nil {
		return err
	}
	ptrData := data[goInterfaceDataOffset : goInterfaceDataOffset+8]
	var err error
	if pt, ok := tt.(*ir.PointerType); ok {
		err = (*pointerType)(pt).encodeValueFields(c, enc, ptrData)
	} else {
		switch t := tt.(type) {
		// Reference types need to be indirected appropriately.
		case *ir.GoMapType /* *ir.GoChannelType, *ir.GoSubroutineType */ :
			typeID = t.HeaderType.GetID()
		}
		err = encodePointer(c, ptrData, false, typeID, enc)
	}
	if err != nil {
		return err
	}
	return writeTokens(enc, jsontext.EndObject, jsontext.EndObject)
}

func formatInterface(
	c *encodingContext,
	buf *bytes.Buffer,
	data []byte,
	limits *formatLimits,
) error {
	if len(data) != 16 {
		writeBoundedError(buf, limits, "interface", "invalid data")
		return nil
	}

	runtimeTypeData := data[goRuntimeTypeOffset : goRuntimeTypeOffset+8]
	runtimeType := binary.NativeEndian.Uint64(runtimeTypeData)
	if runtimeType == 0 {
		writeBoundedString(buf, limits, formatNil)
		return nil
	}

	typeID, ok := c.getTypeIDByGoRuntimeType(uint32(runtimeType))
	if !ok {
		name, err := c.ResolveTypeName(gotype.TypeID(runtimeType))
		if err != nil {
			name = fmt.Sprintf(
				"UnknownType(0x%x): %v", runtimeType, err,
			)
		} else if c.missingTypeCollector != nil {
			c.missingTypeCollector.RecordMissingType(name)
		}
		msg := "unknown type " + name
		writeBoundedError(buf, limits, "interface", msg)
		return nil
	}

	t, ok := c.getType(typeID)
	if !ok {
		writeBoundedError(buf, limits, "interface", "type not found")
		return nil
	}

	tt := t.irType()
	ptrData := data[goInterfaceDataOffset : goInterfaceDataOffset+8]
	if pt, ok := tt.(*ir.PointerType); ok {
		return (*pointerType)(pt).formatValueFields(c, buf, ptrData, limits)
	}

	// For non-pointer types, we need to format the pointed-to value.
	// Handle map types specially.
	switch t := tt.(type) {
	case *ir.GoMapType:
		typeID = t.HeaderType.GetID()
		tt = t.HeaderType
	}

	return formatPointer(c, buf, ptrData, typeID, tt, limits)
}

func (s *goSubroutineType) irType() ir.Type { return (*ir.GoSubroutineType)(s) }
func (s *goSubroutineType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (s *goSubroutineType) formatValueFields(
	_ *encodingContext, buf *bytes.Buffer, _ []byte, limits *formatLimits,
) error {
	writeBoundedString(buf, limits, "{func}")
	return nil
}

func (u *unresolvedPointeeType) irType() ir.Type { return (*ir.UnresolvedPointeeType)(u) }
func (u *unresolvedPointeeType) encodeValueFields(
	_ *encodingContext,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc, tokenNotCapturedReason, tokenNotCapturedReasonDepth)
}

func (u *unresolvedPointeeType) formatValueFields(
	*encodingContext, *bytes.Buffer, []byte, *formatLimits,
) error {
	return errors.New("depth limit reached")
}

func getFieldByName(fields []ir.Field, name string) (*ir.Field, error) {
	for _, f := range fields {
		if f.Name == name {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("field %s not found", name)
}
