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

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// decoderType is a decoder-specific representation of an ir.Type
// It is used to so that specific types can implement their own
// encoding methods. We can track these types in the decoder as a
// way of caching type-specific information such as map key and
// value types.
type decoderType interface {
	irType() ir.Type
	encodeValueFields(
		ctx *decoderContext,
		data []byte,
	) error
}
type decoderContext struct {
	decoder           *Decoder
	enc               *jsontext.Encoder
	currentlyEncoding map[typeAndAddr]struct{}
}

// Type equivalent definitions
type baseType ir.BaseType
type pointerType ir.PointerType
type structureType ir.StructureType
type arrayType ir.ArrayType
type voidPointerType ir.VoidPointerType
type goSliceHeaderType ir.GoSliceHeaderType
type goSliceDataType ir.GoSliceDataType
type goStringHeaderType ir.GoStringHeaderType
type goStringDataType ir.GoStringDataType
type goMapType ir.GoMapType
type goHMapHeaderType ir.GoHMapHeaderType
type goHMapBucketType ir.GoHMapBucketType
type goSwissMapHeaderType struct {
	*ir.GoSwissMapHeaderType
	keyTypeID       uint8
	valueTypeID     uint8
	dirLenOffset    uint8
	dirLenSize      uint8
	dirPtrOffset    uint8
	dirPtrSize      uint8
	ctrlOffset      uint8
	ctrlSize        uint8
	slotsOffset     uint8
	slotsSize       uint8
	tableType       *ir.PointerType
	tableStructType *ir.StructureType
	groupType       *ir.GoSwissMapGroupsType
}
type goSwissMapGroupsType ir.GoSwissMapGroupsType
type goChannelType ir.GoChannelType
type goEmptyInterfaceType ir.GoEmptyInterfaceType
type goInterfaceType ir.GoInterfaceType
type goSubroutineType ir.GoSubroutineType
type eventRootType ir.EventRootType

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

		slotsField, err := getFieldByName(s.GroupType.RawFields, "slots")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		slotsFieldType, ok := types[slotsField.Type.GetID()]
		if !ok {
			return nil, errors.New("type map slot field not found in types: " + s.GroupType.Name)
		}
		ctrlField, err := getFieldByName(s.GroupType.RawFields, "ctrl")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		ctrlOffset := ctrlField.Offset
		ctrlSize := ctrlField.Type.GetByteSize()
		entryArray, ok := slotsFieldType.(*ir.ArrayType)
		if !ok {
			return nil, errors.New("type map slot field is not an array type: " + slotsFieldType.GetName())
		}
		noalgstructType, ok := entryArray.Element.(*ir.StructureType)
		if !ok {
			return nil, errors.New("type map entry array element is not a structure type: " + entryArray.Element.GetName())
		}
		keyField, err := getFieldByName(noalgstructType.RawFields, "key")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		if keyField == nil {
			return nil, errors.New("type map entry array element has no key field: " + entryArray.Element.GetName())
		}
		elem, err := getFieldByName(noalgstructType.RawFields, "elem")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}

		tableType := s.TablePtrSliceType.Element.(*ir.PointerType)
		tableStructType := tableType.Pointee.(*ir.StructureType)
		groupField, err := getFieldByName(tableStructType.RawFields, "groups")
		if err != nil {
			return nil, fmt.Errorf("malformed swiss map header type: %w", err)
		}
		groupType, ok := groupField.Type.(*ir.GoSwissMapGroupsType)
		if !ok {
			return nil, fmt.Errorf("group field type is not a swiss map groups type: %s", groupField.Type.GetName())
		}
		return &goSwissMapHeaderType{
			GoSwissMapHeaderType: s,
			keyTypeID:            uint8(keyField.Type.GetID()),
			valueTypeID:          uint8(elem.Type.GetID()),
			dirLenOffset:         uint8(dirLenOffset),
			dirLenSize:           uint8(dirLenSize),
			dirPtrOffset:         uint8(dirPtrOffset),
			dirPtrSize:           uint8(dirPtrSize),
			ctrlOffset:           uint8(ctrlOffset),
			ctrlSize:             uint8(ctrlSize),
			slotsOffset:          uint8(slotsField.Offset),
			slotsSize:            uint8(slotsFieldType.GetByteSize()),
			tableType:            tableType,
			tableStructType:      tableStructType,
			groupType:            groupType,
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
		return (*goStringHeaderType)(s), nil
	case *ir.GoStringDataType:
		return (*goStringDataType)(s), nil
	case *ir.GoMapType:
		return (*goMapType)(s), nil
	case *ir.GoHMapHeaderType:
		return (*goHMapHeaderType)(s), nil
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
	default:
		return nil, fmt.Errorf("unknown type %s", irType.GetName())
	}
}

func (b *baseType) irType() ir.Type { return (*ir.BaseType)(b) }
func (b *baseType) encodeValueFields(
	ctx *decoderContext,
	data []byte,
) error {
	if err := writeTokens(ctx.enc,
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
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatBool(data[0] == 1)))
	case reflect.Int:
		if len(data) < 8 {
			return errors.New("passed data not long enough for int")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint64(data)), 10)))
	case reflect.Int8:
		if len(data) < 1 {
			return errors.New("passed data not long enough for int8")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatInt(int64(int8(data[0])), 10)))
	case reflect.Int16:
		if len(data) < 2 {
			return errors.New("passed data not long enough for int16")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint16(data)), 10)))
	case reflect.Int32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for int32")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint32(data)), 10)))
	case reflect.Int64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for int64")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint64(data)), 10)))
	case reflect.Uint:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatUint(binary.NativeEndian.Uint64(data), 10)))
	case reflect.Uint8:
		if len(data) != 1 {
			return errors.New("passed data not long enough for uint8")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatUint(uint64(data[0]), 10)))
	case reflect.Uint16:
		if len(data) != 2 {
			return errors.New("passed data not long enough for uint16")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatUint(uint64(binary.NativeEndian.Uint16(data)), 10)))
	case reflect.Uint32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for uint32")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatUint(uint64(binary.NativeEndian.Uint32(data)), 10)))
	case reflect.Uint64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint64")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatUint(binary.NativeEndian.Uint64(data), 10)))
	case reflect.Uintptr:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uintptr")
		}
		return writeTokens(ctx.enc, jsontext.String("0x"+strconv.FormatUint(binary.NativeEndian.Uint64(data), 16)))
	case reflect.Float32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for float32")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatFloat(float64(math.Float32frombits(binary.NativeEndian.Uint32(data))), 'f', -1, 64)))
	case reflect.Float64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for float64")
		}
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatFloat(math.Float64frombits(binary.NativeEndian.Uint64(data)), 'f', -1, 64)))
	case reflect.Complex64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for complex64")
		}
		realBits := math.Float32frombits(binary.NativeEndian.Uint32(data[0:4]))
		imagBits := math.Float32frombits(binary.NativeEndian.Uint32(data[4:8]))
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatComplex(complex(float64(realBits), float64(imagBits)), 'f', -1, 64)))
	case reflect.Complex128:
		if len(data) != 16 {
			return errors.New("passed data not long enough for complex128")
		}
		realBits := math.Float64frombits(binary.NativeEndian.Uint64(data[0:8]))
		imagBits := math.Float64frombits(binary.NativeEndian.Uint64(data[8:16]))
		return writeTokens(ctx.enc, jsontext.String(strconv.FormatComplex(complex(realBits, imagBits), 'f', -1, 64)))
	default:
		return fmt.Errorf("%s is not a base type", kind)
	}
}

func (e *eventRootType) irType() ir.Type { return (*ir.EventRootType)(e) }
func (e *eventRootType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (m *goMapType) irType() ir.Type { return (*ir.GoMapType)(m) }
func (m *goMapType) encodeValueFields(
	ctx *decoderContext,
	data []byte,
) error {
	if len(data) < int(m.GetByteSize()) {
		return errors.New("passed data not long enough for map")
	}
	addr := binary.NativeEndian.Uint64(data)
	key := typeAndAddr{
		irType: uint32(m.HeaderType.GetID()),
		addr:   addr,
	}
	if key.addr == 0 {
		if err := writeTokens(ctx.enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		); err != nil {
			return err
		}
		return nil
	}
	keyValue, dataItemExists := ctx.decoder.dataItemReferences[key]
	if !dataItemExists {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	keyValueHeader := keyValue.Header()
	if keyValueHeader == nil {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	headerType, ok := ctx.decoder.program.Types[ir.TypeID(keyValueHeader.Type)]
	if !ok {
		return fmt.Errorf("no type for header type (ID: %d)", keyValueHeader.Type)
	}
	headerDecoderType, ok := ctx.decoder.decoderTypes[headerType.GetID()]
	if !ok {
		return fmt.Errorf("no decoder type found for header type (ID: %d)", keyValue.Header().Type)
	}
	return headerDecoderType.encodeValueFields(
		ctx,
		keyValue.Data(),
	)
}

func (h *goHMapHeaderType) irType() ir.Type { return (*ir.GoHMapHeaderType)(h) }
func (h *goHMapHeaderType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (b *goHMapBucketType) irType() ir.Type { return (*ir.GoHMapBucketType)(b) }
func (b *goHMapBucketType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (s *goSwissMapHeaderType) irType() ir.Type { return s.GoSwissMapHeaderType }
func (s *goSwissMapHeaderType) encodeValueFields(
	ctx *decoderContext,
	data []byte,
) error {
	var (
		dirLen      int64
		dirPtr      uint64
		notCaptured bool
	)
	dirLen = int64(binary.NativeEndian.Uint64(data[s.dirLenOffset : s.dirLenOffset+s.dirLenSize]))
	dirPtr = binary.NativeEndian.Uint64(data[s.dirPtrOffset : s.dirPtrOffset+s.dirPtrSize])
	if err := writeTokens(
		ctx.enc, jsontext.String("entries"), jsontext.BeginArray,
	); err != nil {
		return err
	}
	if dirLen == 0 {
		// This is a 'small' swiss map where there's only one group.
		// We can collect the data item for the group directly.
		groupDataItem, ok := ctx.decoder.dataItemReferences[typeAndAddr{
			irType: uint32(s.GroupType.GetID()),
			addr:   dirPtr,
		}]
		if !ok {
			return fmt.Errorf("group data item not found for addr %x", dirPtr)
		}
		err := ctx.decoder.encodeSwissMapGroup(
			ctx.enc,
			ctx.currentlyEncoding,
			s,
			groupDataItem.Data(),
			ir.TypeID(s.keyTypeID),
			ir.TypeID(s.valueTypeID),
		)
		if err != nil {
			return err
		}
	} else {
		// This is a 'large' swiss map where there are multiple groups of data/control words
		// We need to collect the data items for the table pointers first.
		tablePtrSliceDataItemPtr, ok := ctx.decoder.dataItemReferences[typeAndAddr{
			irType: uint32(s.TablePtrSliceType.GetID()),
			addr:   dirPtr,
		}]
		if !ok {
			log.Tracef("table ptr slice data item pointer not found for addr %x", dirPtr)
			notCaptured = true
		}
		tablePtrSliceDataItem, ok := ctx.decoder.dataItemReferences[typeAndAddr{
			irType: tablePtrSliceDataItemPtr.Header().Type,
			addr:   tablePtrSliceDataItemPtr.Header().Address,
		}]
		if !ok {
			log.Tracef("table ptr slice data item not found for addr %x", tablePtrSliceDataItemPtr.Header().Address)
			notCaptured = true
		}
		err := ctx.decoder.encodeSwissMapTables(
			ctx,
			s,
			tablePtrSliceDataItem,
		)
		if err != nil {
			return err
		}
	}
	if err := writeTokens(ctx.enc, jsontext.EndArray); err != nil {
		return err
	}
	if notCaptured {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	return nil
}

func (s *goSwissMapGroupsType) irType() ir.Type { return (*ir.GoSwissMapGroupsType)(s) }
func (s *goSwissMapGroupsType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (v *voidPointerType) irType() ir.Type { return (*ir.VoidPointerType)(v) }
func (v *voidPointerType) encodeValueFields(
	ctx *decoderContext,
	data []byte,
) error {
	if len(data) != 8 {
		return errors.New("passed data not long enough for void pointer")
	}
	return writeTokens(ctx.enc,
		jsontext.String("address"),
		jsontext.String("0x"+strconv.FormatUint(binary.NativeEndian.Uint64(data), 16)),
	)
}

func (p *pointerType) irType() ir.Type { return (*ir.PointerType)(p) }
func (p *pointerType) encodeValueFields(
	ctx *decoderContext,
	data []byte,
) error {
	if len(data) < 8 {
		return errors.New("passed data not long enough for pointer: need 8 bytes")
	}
	addr := binary.NativeEndian.Uint64(data)
	pointeeKey := typeAndAddr{
		irType: uint32(p.Pointee.GetID()),
		addr:   addr,
	}
	if pointeeKey.addr == 0 {
		if err := writeTokens(ctx.enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		); err != nil {
			return err
		}
		return nil
	}

	var address uint64
	pointedValue, dataItemExists := ctx.decoder.dataItemReferences[pointeeKey]
	if !dataItemExists {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	address = pointedValue.Header().Address
	if err := writeTokens(ctx.enc,
		jsontext.String("address"),
		jsontext.String("0x"+strconv.FormatInt(int64(address), 16)),
	); err != nil {
		return err
	}
	if _, alreadyEncoding := ctx.currentlyEncoding[pointeeKey]; !alreadyEncoding && dataItemExists {
		ctx.currentlyEncoding[pointeeKey] = struct{}{}
		defer delete(ctx.currentlyEncoding, pointeeKey)
		if err := writeTokens(ctx.enc,
			jsontext.String("value"),
		); err != nil {
			return err
		}
		if err := ctx.decoder.encodeValue(
			ctx.enc,
			ctx.currentlyEncoding,
			ir.TypeID(p.Pointee.GetID()),
			pointedValue.Data(),
			p.Pointee.GetName(),
		); err != nil {
			return fmt.Errorf("could not encode referenced value: %w", err)
		}
	}
	return nil
}

func (s *structureType) irType() ir.Type { return (*ir.StructureType)(s) }
func (s *structureType) encodeValueFields(
	ctx *decoderContext,
	data []byte,
) error {
	var err error
	if err = writeTokens(ctx.enc,
		jsontext.String("fields"),
		jsontext.BeginObject); err != nil {
		return err
	}
	for _, field := range s.RawFields {
		if err = writeTokens(ctx.enc, jsontext.String(field.Name)); err != nil {
			return err
		}
		fieldEnd := field.Offset + field.Type.GetByteSize()
		if fieldEnd > uint32(len(data)) {
			return fmt.Errorf("field %s extends beyond data bounds: need %d bytes, have %d", field.Name, fieldEnd, len(data))
		}

		if err = ctx.decoder.encodeValue(ctx.enc,
			ctx.currentlyEncoding,
			field.Type.GetID(),
			data[field.Offset:field.Offset+field.Type.GetByteSize()],
			field.Type.GetName(),
		); err != nil {
			return err
		}
	}
	return writeTokens(ctx.enc, jsontext.EndObject)
}

func (a *arrayType) irType() ir.Type { return (*ir.ArrayType)(a) }
func (a *arrayType) encodeValueFields(
	ctx *decoderContext,
	data []byte,
) error {
	var err error
	elementSize := int(a.Element.GetByteSize())
	numElements := int(a.Count)
	if err = writeTokens(ctx.enc,
		jsontext.String("size"),
		jsontext.String(strconv.Itoa(numElements)),
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
		if err := ctx.decoder.encodeValue(ctx.enc,
			ctx.currentlyEncoding,
			a.Element.GetID(),
			elementData,
			a.Element.GetName(),
		); err != nil {
			return err
		}
	}
	if err := writeTokens(ctx.enc, jsontext.EndArray); err != nil {
		return err
	}
	if notCaptured {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	return nil
}

func (s *goSliceHeaderType) irType() ir.Type { return (*ir.GoSliceHeaderType)(s) }
func (s *goSliceHeaderType) encodeValueFields(
	ctx *decoderContext,
	data []byte) error {

	if len(data) < int(s.ByteSize) {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	if len(data) < 16 {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	address := binary.NativeEndian.Uint64(data[0:8])
	if address == 0 {
		return writeTokens(ctx.enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		)
	}
	length := binary.NativeEndian.Uint64(data[8:16])
	if length == 0 {
		return writeTokens(ctx.enc,
			jsontext.String("elements"),
			jsontext.BeginArray,
			jsontext.EndArray)
	}
	elementSize := int(s.Data.Element.GetByteSize())
	taa := typeAndAddr{
		addr:   address,
		irType: uint32(s.Data.GetID()),
	}
	sliceDataItem, ok := ctx.decoder.dataItemReferences[taa]
	if !ok {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	if err := writeTokens(ctx.enc,
		jsontext.String("elements"),
		jsontext.BeginArray); err != nil {
		return err
	}
	sliceLength := int(sliceDataItem.Header().Length) / elementSize
	sliceData := sliceDataItem.Data()
	var notCaptured = false
	for i := range int(sliceLength) {
		elementData := sliceData[i*int(s.Data.Element.GetByteSize()) : (i+1)*int(s.Data.Element.GetByteSize())]
		if err := ctx.decoder.encodeValue(ctx.enc,
			ctx.currentlyEncoding,
			s.Data.Element.GetID(),
			elementData,
			s.Data.Element.GetName(),
		); err != nil {
			log.Tracef("could not encode slice element: %v", err)
			notCapturedReason = notCapturedReasonPruned
			break
		}
	}
	if err := writeTokens(ctx.enc, jsontext.EndArray); err != nil {
		return err
	}
	if length > uint64(sliceLength) {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonCollectionSize,
		)
	} else if notCaptured {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	return nil
}

func (s *goSliceDataType) irType() ir.Type { return (*ir.GoSliceDataType)(s) }
func (s *goSliceDataType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {

	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (s *goStringHeaderType) irType() ir.Type { return (*ir.GoStringHeaderType)(s) }
func (s *goStringHeaderType) encodeValueFields(
	ctx *decoderContext,
	data []byte) error {

	var (
		address       uint64
		fieldByteSize uint32
	)
	field, err := getFieldByName(s.RawFields, "str")
	if err != nil {
		return fmt.Errorf("malformed string field 'str': %w", err)
	}
	lenField, err := getFieldByName(s.RawFields, "len")
	if err != nil {
		return fmt.Errorf("malformed string field 'str': %w", err)
	}
	realLength := binary.NativeEndian.Uint64(data[lenField.Offset : lenField.Offset+lenField.Type.GetByteSize()])

	fieldByteSize = field.Type.GetByteSize()
	fieldEnd := field.Offset + fieldByteSize
	if fieldEnd >= uint32(len(data)) {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonLength,
		)
	}
	address = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+fieldByteSize])
	if address == 0 {
		return writeTokens(ctx.enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		)
	}
	stringValue, ok := ctx.decoder.dataItemReferences[typeAndAddr{
		irType: uint32(s.Data.GetID()),
		addr:   address,
	}]
	if !ok {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	stringData := stringValue.Data()
	length := stringValue.Header().Length
	if len(stringData) < int(length) {
		return writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	if realLength > uint64(len(stringData)) {
		// We captured partial data for the string, report truncation
		if err := writeTokens(ctx.enc,
			notCapturedReason,
			notCapturedReasonPruned,
		); err != nil {
			return err
		}
	}
	if err := writeTokens(ctx.enc,
		jsontext.String("value"),
	); err != nil {
		return err
	}
	str := unsafe.String(unsafe.SliceData(stringData), int(length))
	return writeTokens(ctx.enc, jsontext.String(str))
}

func (s *goStringDataType) irType() ir.Type { return (*ir.GoStringDataType)(s) }
func (s *goStringDataType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (c *goChannelType) irType() ir.Type { return (*ir.GoChannelType)(c) }
func (c *goChannelType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (e *goEmptyInterfaceType) irType() ir.Type { return (*ir.GoEmptyInterfaceType)(e) }
func (e *goEmptyInterfaceType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (i *goInterfaceType) irType() ir.Type { return (*ir.GoInterfaceType)(i) }
func (i *goInterfaceType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (s *goSubroutineType) irType() ir.Type { return (*ir.GoSubroutineType)(s) }
func (s *goSubroutineType) encodeValueFields(
	ctx *decoderContext,
	_ []byte,
) error {
	return writeTokens(ctx.enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func getFieldByName(fields []ir.Field, name string) (*ir.Field, error) {
	for _, f := range fields {
		if f.Name == name {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("field %s not found", name)
}
