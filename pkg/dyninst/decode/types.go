// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"unsafe"

	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// decoderType is a decoder-specific representation of an ir.Type
// It is used to so that specific types can implement their own
// encoding methods. We can track these types in the decoder as a
// way of caching type-specific information such as map key and
// value types.
type decoderType interface {
	irType() ir.Type
	encodeValueFields(
		d *Decoder,
		enc *jsontext.Encoder,
		data []byte,
	) error
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
	_ *Decoder,
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

func (e *eventRootType) irType() ir.Type { return (*ir.EventRootType)(e) }
func (e *eventRootType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (m *goMapType) irType() ir.Type { return (*ir.GoMapType)(m) }
func (m *goMapType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte,
) error {
	const encodeAddress = false
	return encodePointer(data, encodeAddress, m.HeaderType.GetID(), enc, d)
}

func (h *goHMapHeaderType) irType() ir.Type { return h.GoHMapHeaderType }
func (h *goHMapHeaderType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte,
) error {
	maxOffset := max(h.countOffset+8, h.bucketsOffset+8, h.oldBucketsOffset+8)
	if maxOffset > uint32(len(data)) {
		return fmt.Errorf("data is too short to contain all fields")
	}
	count := binary.NativeEndian.Uint64(data[h.countOffset : h.countOffset+8])
	if err := writeTokens(enc,
		jsontext.String("size"),
		jsontext.String(strconv.FormatInt(int64(count), 10)),
	); err != nil {
		return err
	}
	if err := writeTokens(
		enc, jsontext.String("entries"), jsontext.BeginArray,
	); err != nil {
		return err
	}
	encodeBuckets := func(dataItem output.DataItem) (encodedItems int, err error) {
		data, ok := dataItem.Data()
		if !ok {
			// Should we tell the user about this fault?
			return 0, nil
		}
		numBuckets := len(data) / int(h.bucketByteSize)
		for i := range numBuckets {
			bucketOffset := uint32(i) * h.bucketByteSize
			bucketData := data[bucketOffset : bucketOffset+h.bucketByteSize]
			bucketItems, err := encodeHMapBucket(d, enc, h, bucketData)
			if err != nil {
				return 0, fmt.Errorf("error encoding bucket: %w", err)
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
		item, ok := d.dataItems[typeAndAddr{
			irType: uint32(h.bucketsTypeID),
			addr:   addr,
		}]
		if !ok {
			continue
		}
		items, err := encodeBuckets(item)
		if err != nil {
			return err
		}
		encodedItems += items
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

func encodeHMapBucket(
	d *Decoder,
	enc *jsontext.Encoder,
	h *goHMapHeaderType,
	bucketData []byte,
) (encodedItems int, err error) {
	// See https://github.com/golang/go/blob/66d34c7d/src/runtime/map.go#L90-L99
	const (
		emptyRest      = 0 // this cell is empty, and there are no more non-empty cells
		emptyOne       = 1 // this cell is empty
		evacuatedX     = 2 // key/elem is valid.  Entry has been evacuated to first half of larger table.
		evacuatedEmpty = 4 // cell is empty, bucket is evacuated.
		topHashSize    = 8
	)
	upperBound := max(
		h.keysOffset+h.keyTypeSize*topHashSize,
		h.valuesOffset+h.valueTypeSize*topHashSize,
		h.tophashOfset+topHashSize,
		h.overflowOffset+8,
	)
	if upperBound > uint32(len(bucketData)) {
		return encodedItems, fmt.Errorf(
			"hmap bucket data for %q is too short to contain all fields: %d > %d",
			h.Name, upperBound, len(bucketData),
		)
	}
	topHash := bucketData[h.tophashOfset : h.tophashOfset+topHashSize]
	for i, b := range topHash {
		if b == emptyRest || (b >= evacuatedX && b <= evacuatedEmpty) {
			break
		}
		if b == emptyOne {
			continue
		}
		encodedItems++
		keyOffset := h.keysOffset + uint32(i)*h.keyTypeSize
		valueOffset := h.valuesOffset + uint32(i)*h.valueTypeSize
		if err := writeTokens(enc, jsontext.BeginArray); err != nil {
			return encodedItems, err
		}
		keyData := bucketData[keyOffset : keyOffset+h.keyTypeSize]
		if err := d.encodeValue(enc, h.keyTypeID, keyData, h.keyTypeName); err != nil {
			return encodedItems, err
		}
		valueData := bucketData[valueOffset : valueOffset+h.valueTypeSize]
		if err := d.encodeValue(enc, h.valueTypeID, valueData, h.valueTypeName); err != nil {
			return encodedItems, err
		}
		if err := writeTokens(enc, jsontext.EndArray); err != nil {
			return encodedItems, err
		}
	}
	overflowAddr := binary.NativeEndian.Uint64(bucketData[h.overflowOffset : h.overflowOffset+8])
	if overflowAddr != 0 {
		overflowDataItem, ok := d.dataItems[typeAndAddr{
			irType: uint32(h.bucketTypeID),
			addr:   overflowAddr,
		}]
		var overflowData []byte
		if ok {
			overflowData, ok = overflowDataItem.Data()
		}
		if ok {
			overflowItems, err := encodeHMapBucket(d, enc, h, overflowData)
			if err != nil {
				return encodedItems, err
			}
			encodedItems += overflowItems
		}
	}
	return encodedItems, nil
}

func (b *goHMapBucketType) irType() ir.Type { return (*ir.GoHMapBucketType)(b) }
func (*goHMapBucketType) encodeValueFields(
	*Decoder, *jsontext.Encoder, []byte,
) error {
	return fmt.Errorf("hmap bucket type is never directly encoded")
}

func (s *goSwissMapHeaderType) irType() ir.Type { return s.GoSwissMapHeaderType }
func (s *goSwissMapHeaderType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte,
) error {
	used := int64(binary.NativeEndian.Uint64(data[s.usedOffset : s.usedOffset+uint32(s.usedSize)]))
	if err := writeTokens(enc,
		jsontext.String("size"),
		jsontext.String(strconv.FormatInt(used, 10)),
	); err != nil {
		return err
	}
	dirLen := int64(binary.NativeEndian.Uint64(data[s.dirLenOffset : s.dirLenOffset+uint32(s.dirLenSize)]))
	dirPtr := binary.NativeEndian.Uint64(data[s.dirPtrOffset : s.dirPtrOffset+uint32(s.dirPtrSize)])
	if dirLen == 0 {
		// This is a 'small' swiss map where there's only one group.
		// We can collect the data item for the group directly.
		groupDataItem, ok := d.dataItems[typeAndAddr{
			irType: uint32(s.groupTypeID),
			addr:   dirPtr,
		}]
		if !ok {
			return writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonDepth,
			)
		}
		groupData, ok := groupDataItem.Data()
		if !ok {
			// The attempt to dereference the group data item failed. This can
			// happen due to paging.
			return writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonUnavailable,
			)
		}
		if err := writeTokens(
			enc, jsontext.String("entries"), jsontext.BeginArray,
		); err != nil {
			return err
		}
		totalElementsEncoded, err := s.encodeSwissMapGroup(d, enc, groupData)
		if err != nil {
			return err
		}
		if used > int64(totalElementsEncoded) {
			if err := writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonPruned,
			); err != nil {
				return err
			}
		}
	} else {
		// This is a 'large' swiss map where there are multiple groups of data/control words
		// We need to collect the data items for the table pointers first.
		tablePtrSliceDataItem, ok := d.dataItems[typeAndAddr{
			irType: uint32(s.TablePtrSliceType.GetID()),
			addr:   dirPtr,
		}]
		if !ok {
			return writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonDepth,
			)
		}
		tablePtrSliceData, ok := tablePtrSliceDataItem.Data()
		if !ok {
			return writeTokens(enc,
				tokenNotCapturedReason,
				tokenNotCapturedReasonUnavailable,
			)
		}
		if err := writeTokens(
			enc, jsontext.String("entries"), jsontext.BeginArray,
		); err != nil {
			return err
		}
		totalElementsEncoded, err := s.encodeSwissMapTables(d, enc, tablePtrSliceData)
		if err != nil {
			return err
		}
		if used > int64(totalElementsEncoded) {
			return writeTokens(enc,
				jsontext.EndArray,
				tokenNotCapturedReason,
				tokenNotCapturedReasonPruned,
			)
		}
	}
	return writeTokens(enc, jsontext.EndArray)
}

func (s *goSwissMapGroupsType) irType() ir.Type { return (*ir.GoSwissMapGroupsType)(s) }
func (s *goSwissMapGroupsType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (v *voidPointerType) irType() ir.Type { return (*ir.VoidPointerType)(v) }
func (v *voidPointerType) encodeValueFields(
	_ *Decoder,
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

func (p *pointerType) irType() ir.Type { return (*ir.PointerType)(p) }
func (p *pointerType) encodeValueFields(
	d *Decoder,
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
	return encodePointer(data, writeAddress, p.Pointee.GetID(), enc, d)
}

func encodePointer(
	data []byte,
	writeAddress bool,
	pointee ir.TypeID,
	enc *jsontext.Encoder,
	d *Decoder,
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

	pointeeDecoderType, ok := d.decoderTypes[pointee]
	if !ok {
		return fmt.Errorf("no decoder type found for pointee type (ID: %d)", pointee)
	}

	// If the pointee type has zero size, we don't expect there to be a data
	// item for it.
	var (
		pointedValue   output.DataItem
		dataItemExists bool
	)
	isZeroSized := pointeeDecoderType.irType().GetByteSize() == 0
	if !isZeroSized {
		pointedValue, dataItemExists = d.dataItems[pointeeKey]
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

	if _, alreadyEncoding := d.currentlyEncoding[pointeeKey]; !alreadyEncoding {
		d.currentlyEncoding[pointeeKey] = struct{}{}
		defer delete(d.currentlyEncoding, pointeeKey)
		var pointedData []byte
		if !isZeroSized {
			if pointedData, ok = pointedValue.Data(); !ok {
				return writeTokens(enc,
					tokenNotCapturedReason,
					tokenNotCapturedReasonUnavailable,
				)
			}
		}
		if err := pointeeDecoderType.encodeValueFields(
			d, enc, pointedData,
		); err != nil {
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
	d *Decoder,
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
			return fmt.Errorf("field %s extends beyond data bounds: need %d bytes, have %d", field.Name, fieldEnd, len(data))
		}

		if err := d.encodeValue(enc,
			field.Type.GetID(),
			data[field.Offset:field.Offset+field.Type.GetByteSize()],
			field.Type.GetName(),
		); err != nil {
			return err
		}
	}
	return writeTokens(enc, jsontext.EndObject)
}

func (a *arrayType) irType() ir.Type { return (*ir.ArrayType)(a) }
func (a *arrayType) encodeValueFields(
	d *Decoder,
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
	for i := range numElements {
		offset := i * elementSize
		endIdx := offset + elementSize
		if endIdx > len(data) {
			notCaptured = true
			break
		}
		elementData := data[offset:endIdx]
		if err := d.encodeValue(enc,
			a.Element.GetID(),
			elementData,
			a.Element.GetName(),
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

func (s *goSliceHeaderType) irType() ir.Type { return (*ir.GoSliceHeaderType)(s) }
func (s *goSliceHeaderType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte) error {

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
	if length == 0 {
		return writeTokens(enc,
			jsontext.String("elements"),
			jsontext.BeginArray,
			jsontext.EndArray)
	}
	if err := writeTokens(enc,
		jsontext.String("size"),
		jsontext.String(strconv.FormatInt(int64(length), 10)),
	); err != nil {
		return err
	}

	elementSize := int(s.Data.Element.GetByteSize())
	taa := typeAndAddr{
		addr:   address,
		irType: uint32(s.Data.GetID()),
	}
	sliceDataItem, ok := d.dataItems[taa]
	if !ok {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonPruned,
		)
	}
	if err := writeTokens(enc,
		jsontext.String("elements"),
		jsontext.BeginArray); err != nil {
		return err
	}
	sliceData, ok := sliceDataItem.Data()
	if !ok {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonUnavailable,
		)
	}
	sliceLength := int(len(sliceData)) / elementSize
	elementByteSize := int(s.Data.Element.GetByteSize())
	var notCaptured = false
	for i := range int(sliceLength) {
		elementData := sliceData[i*elementByteSize : (i+1)*elementByteSize]
		if err := d.encodeValue(enc,
			s.Data.Element.GetID(),
			elementData,
			s.Data.Element.GetName(),
		); err != nil {
			notCaptured = true
			break
		}
	}
	if err := writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	if length > uint64(sliceLength) {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonCollectionSize,
		)
	} else if notCaptured {
		return writeTokens(enc,
			tokenNotCapturedReason,
			tokenNotCapturedReasonPruned,
		)
	}
	return nil
}

func (s *goSliceDataType) irType() ir.Type { return (*ir.GoSliceDataType)(s) }
func (s *goSliceDataType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (s *goStringHeaderType) irType() ir.Type { return s.GoStringHeaderType }
func (s *goStringHeaderType) encodeValueFields(
	d *Decoder,
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
	stringValue, ok := d.dataItems[typeAndAddr{
		irType: uint32(s.Data.GetID()),
		addr:   address,
	}]
	if !ok {
		return writeTokens(enc,
			jsontext.String("size"),
			jsontext.String(strconv.FormatInt(int64(strLen), 10)),
			tokenNotCapturedReason,
			tokenNotCapturedReasonDepth,
		)
	}
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
		// We captured partial data for the string, report truncation
		if err := writeTokens(enc,
			jsontext.String("size"),
			jsontext.String(strconv.FormatInt(int64(strLen), 10)),
			tokenTruncated,
			jsontext.Bool(true),
		); err != nil {
			return err
		}
	}
	if err := writeTokens(enc,
		jsontext.String("value"),
	); err != nil {
		return err
	}
	str := unsafe.String(unsafe.SliceData(stringData), int(length))
	return writeTokens(enc, jsontext.String(str))
}

func (s *goStringDataType) irType() ir.Type { return (*ir.GoStringDataType)(s) }
func (s *goStringDataType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (c *goChannelType) irType() ir.Type { return (*ir.GoChannelType)(c) }
func (c *goChannelType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

const goRuntimeTypeOffset = 0x00
const goInterfaceDataOffset = 0x08

func (e *goEmptyInterfaceType) irType() ir.Type { return (*ir.GoEmptyInterfaceType)(e) }
func (e *goEmptyInterfaceType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte,
) error {
	return encodeInterface(d, enc, data)
}

func (i *goInterfaceType) irType() ir.Type { return (*ir.GoInterfaceType)(i) }
func (i *goInterfaceType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte,
) error {
	return encodeInterface(d, enc, data)
}

func encodeInterface(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte,
) error {
	if len(data) != 16 {
		return fmt.Errorf("go interface data must be 16 bytes, got %d", len(data))
	}

	if err := writeTokens(enc,
		jsontext.String("fields"),
		jsontext.BeginObject,
		jsontext.String("data"),
		jsontext.BeginObject,
	); err != nil {
		return err
	}

	runtimeType := binary.NativeEndian.Uint64(data[goRuntimeTypeOffset : goRuntimeTypeOffset+8])
	if runtimeType == 0 {
		return writeTokens(enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
			jsontext.EndObject,
			jsontext.EndObject,
		)
	}

	typeID, ok := d.typesByGoRuntimeType[uint32(runtimeType)]
	if !ok {
		name, err := d.typeNameResolver.ResolveTypeName(gotype.TypeID(runtimeType))
		if err != nil {
			name = fmt.Sprintf("UnknownType(0x%x): %v", runtimeType, err)
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
	t, ok := d.program.Types[typeID]
	if !ok {
		return fmt.Errorf("no type found for type ID: %d", typeID)
	}
	if err := writeTokens(
		enc, jsontext.String("type"), jsontext.String(t.GetName()),
	); err != nil {
		return err
	}
	ptrData := data[goInterfaceDataOffset : goInterfaceDataOffset+8]
	var err error
	if pt, ok := t.(*ir.PointerType); ok {
		err = (*pointerType)(pt).encodeValueFields(d, enc, ptrData)
	} else {
		switch t := t.(type) {
		// Reference types need to be indirected appropriately
		case *ir.GoMapType /* *ir.GoChannelType, *ir.GoSubroutineType */ :
			typeID = t.HeaderType.GetID()
		}
		err = encodePointer(ptrData, false, typeID, enc, d)
	}
	if err != nil {
		return err
	}
	return writeTokens(enc, jsontext.EndObject, jsontext.EndObject)
}

func (s *goSubroutineType) irType() ir.Type { return (*ir.GoSubroutineType)(s) }
func (s *goSubroutineType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		tokenNotCapturedReason,
		tokenNotCapturedReasonUnimplemented,
	)
}

func (u *unresolvedPointeeType) irType() ir.Type { return (*ir.UnresolvedPointeeType)(u) }
func (u *unresolvedPointeeType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc, tokenNotCapturedReason, tokenNotCapturedReasonDepth)
}

func getFieldByName(fields []ir.Field, name string) (*ir.Field, error) {
	for _, f := range fields {
		if f.Name == name {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("field %s not found", name)
}
