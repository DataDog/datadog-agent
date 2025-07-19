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
	"reflect"
	"strconv"
	"unsafe"

	"github.com/go-json-experiment/json/jsontext"

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
		decoder *Decoder,
		enc *jsontext.Encoder,
		dataItems map[typeAndAddr]output.DataItem,
		currentlyEncoding map[typeAndAddr]struct{},
		data []byte,
		valueType string) error
}

// Type equivalent definitions
type baseType ir.BaseType
type pointerType struct {
	*ir.PointerType
	pointedDecoderType decoderType
}
type structureType ir.StructureType
type arrayType ir.ArrayType
type voidPointerType ir.VoidPointerType
type goSliceHeaderType struct {
	*ir.GoSliceHeaderType
	elementDecoderType decoderType
}
type goSliceDataType ir.GoSliceDataType
type goStringHeaderType ir.GoStringHeaderType
type goStringDataType ir.GoStringDataType
type goMapType ir.GoMapType
type goHMapHeaderType ir.GoHMapHeaderType
type goHMapBucketType ir.GoHMapBucketType
type goSwissMapHeaderType struct {
	*ir.GoSwissMapHeaderType
	keyType   decoderType
	valueType decoderType
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

func (d *Decoder) getDecoderType(irType ir.Type) (decoderType, error) {
	switch s := irType.(type) {
	case *ir.PointerType:
		pointedTypeID := s.Pointee.GetID()
		pointedType, ok := d.program.Types[pointedTypeID]
		if !ok {
			return nil, errors.New("pointer pointed type not found in types")
		}
		pointedDecoderType, err := d.getDecoderType(pointedType)
		if err != nil {
			return nil, err
		}
		return &pointerType{
			PointerType:        s,
			pointedDecoderType: pointedDecoderType,
		}, nil
	case *ir.GoSliceHeaderType:
		elementTypeID := s.Data.Element.GetID()
		elementType, ok := d.program.Types[elementTypeID]
		if !ok {
			return nil, errors.New("slice element type not found in types")
		}
		elementDecoderType, err := d.getDecoderType(elementType)
		if err != nil {
			return nil, err
		}
		return &goSliceHeaderType{
			GoSliceHeaderType:  s,
			elementDecoderType: elementDecoderType,
		}, nil
	case *ir.GoSwissMapHeaderType:
		if len(s.GroupType.Fields) < 2 {
			return nil, errors.New("type map group has less than 2 fields: " + s.GroupType.Name)
		}
		slotsField := s.GroupType.Fields[1]
		slotsFieldType, ok := d.program.Types[slotsField.Type.GetID()]
		if !ok {
			return nil, errors.New("type map slot field not found in types: " + s.GroupType.Name)
		}
		entryArray, ok := slotsFieldType.(*ir.ArrayType)
		if !ok {
			return nil, errors.New("type map slot field is not an array type: " + slotsFieldType.GetName())
		}
		noalgstructType, ok := entryArray.Element.(*ir.StructureType)
		if !ok {
			return nil, errors.New("type map entry array element is not a structure type: " + entryArray.Element.GetName())
		}

		if len(noalgstructType.Fields) < 2 {
			return nil, errors.New("type map entry array element has less than 2 fields: " + entryArray.Element.GetName())
		}
		keyField := noalgstructType.Fields[0]
		keyIrType, ok := d.program.Types[keyField.Type.GetID()]
		if !ok {
			return nil, fmt.Errorf("key type %s not found in types", keyField.Type.GetName())
		}
		keyType, err := d.getDecoderType(keyIrType)
		if err != nil {
			return nil, err
		}

		valueField := noalgstructType.Fields[1]
		valueIrType, ok := d.program.Types[valueField.Type.GetID()]
		if !ok {
			return nil, fmt.Errorf("value type %s not found in types", valueField.Type.GetName())
		}
		valueType, err := d.getDecoderType(valueIrType)
		if err != nil {
			return nil, err
		}
		return &goSwissMapHeaderType{
			GoSwissMapHeaderType: s,
			keyType:              keyType,
			valueType:            valueType,
		}, nil

	case *ir.BaseType:
		return (*baseType)(s), nil
	case *ir.StructureType:
		return (*structureType)(s), nil
	case *ir.ArrayType:
		return (*arrayType)(s), nil
	case *ir.VoidPointerType:
		return (*voidPointerType)(s), nil
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
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {

	if err := writeTokens(enc,
		jsontext.String("value"),
	); err != nil {
		return err
	}
	return encodeBaseTypeValue(enc, b, data)
}

func (e *eventRootType) irType() ir.Type { return (*ir.EventRootType)(e) }
func (e *eventRootType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (m *goMapType) irType() ir.Type { return (*ir.GoMapType)(m) }
func (m *goMapType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {

	if len(data) < int(m.GetByteSize()) {
		return errors.New("passed data not long enough for pointer")
	}
	addr := binary.NativeEndian.Uint64(data)
	key := typeAndAddr{
		irType: uint32(m.HeaderType.GetID()),
		addr:   addr,
	}
	if key.addr == 0 {
		if err := writeTokens(enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		); err != nil {
			return err
		}
		return nil
	}
	var address uint64
	pointedValue, dataItemExists := dataItems[key]
	if !dataItemExists {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("depth"), //TODO: can we distinguish if it's depth or ran out of buffer space?
		)
	}
	address = pointedValue.Header().Address
	headerDataItem := dataItems[typeAndAddr{
		irType: uint32(m.HeaderType.GetID()),
		addr:   address,
	}]
	headerType, ok := decoder.program.Types[ir.TypeID(headerDataItem.Header().Type)]
	if !ok {
		return fmt.Errorf("no type for header type (ID: %d)", headerDataItem.Header().Type)
	}
	headerDecoderType, err := decoder.getDecoderType(headerType)
	if err != nil {
		return err
	}
	return headerDecoderType.encodeValueFields(
		decoder,
		enc,
		dataItems,
		currentlyEncoding,
		headerDataItem.Data(),
		headerType.GetName(),
	)
}

func (h *goHMapHeaderType) irType() ir.Type { return (*ir.GoHMapHeaderType)(h) }
func (h *goHMapHeaderType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (b *goHMapBucketType) irType() ir.Type { return (*ir.GoHMapBucketType)(b) }
func (b *goHMapBucketType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (s *goSwissMapHeaderType) irType() ir.Type { return s.GoSwissMapHeaderType }
func (s *goSwissMapHeaderType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {

	var (
		dirLen int64
		dirPtr uint64
	)
	for _, f := range s.Fields {
		fieldEnd := f.Offset + f.Type.GetByteSize()
		if fieldEnd < f.Offset {
			return fmt.Errorf("overflow in field %s offset calculation", f.Name)
		}
		if fieldEnd > uint32(len(data)) {
			return fmt.Errorf("field %s extends beyond data bounds: need %d bytes, have %d", f.Name, fieldEnd, len(data))
		}
		switch f.Name {
		case "dirLen":
			dirLen = int64(binary.NativeEndian.Uint64(data[f.Offset : f.Offset+f.Type.GetByteSize()]))
		case "dirPtr":
			dirPtr = binary.NativeEndian.Uint64(data[f.Offset : f.Offset+f.Type.GetByteSize()])
		}
	}

	if err := writeTokens(enc,
		jsontext.String("entries"),
		jsontext.BeginArray); err != nil {
		return err
	}
	if dirLen == 0 {
		// This is a 'small' swiss map where there's only one group.
		// We can collect the data item for the group directly.
		groupDataItem := dataItems[typeAndAddr{
			irType: uint32(s.GroupType.GetID()),
			addr:   dirPtr,
		}]

		err := decoder.collectSwissMapGroup(
			enc,
			dataItems,
			currentlyEncoding,
			s,
			groupDataItem.Data(),
			s.keyType,
			s.valueType,
		)
		if err != nil {
			return err
		}
	} else {
		// This is a 'large' swiss map where there are multiple groups of data/control words
		// We need to collect the data items for the table pointers first.
		tablePtrSliceDataItemPtr, ok := dataItems[typeAndAddr{
			irType: uint32(s.TablePtrSliceType.GetID()),
			addr:   dirPtr,
		}]
		if !ok {
			return fmt.Errorf("table ptr slice data item pointer not found for addr %x", dirPtr)
		}
		tablePtrSliceDataItem, ok := dataItems[typeAndAddr{
			irType: tablePtrSliceDataItemPtr.Header().Type,
			addr:   tablePtrSliceDataItemPtr.Header().Address,
		}]
		if !ok {
			return fmt.Errorf("table ptr slice data item not found for addr %x", tablePtrSliceDataItemPtr.Header().Address)
		}
		err := decoder.collectSwissMapTables(
			enc,
			dataItems,
			currentlyEncoding,
			s,
			tablePtrSliceDataItem,
		)
		if err != nil {
			return err
		}
	}
	if err := writeTokens(enc,
		jsontext.EndArray); err != nil {
		return err
	}
	return nil
}

func (s *goSwissMapGroupsType) irType() ir.Type { return (*ir.GoSwissMapGroupsType)(s) }
func (s *goSwissMapGroupsType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (v *voidPointerType) irType() ir.Type { return (*ir.VoidPointerType)(v) }
func (v *voidPointerType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	if len(data) != 8 {
		return errors.New("passed data not long enough for void pointer")
	}
	return writeTokens(enc,
		jsontext.String("address"),
		jsontext.String("0x"+strconv.FormatUint(binary.NativeEndian.Uint64(data), 16)),
	)
}

func (p *pointerType) irType() ir.Type { return p.PointerType }
func (p *pointerType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	if len(data) < 8 {
		return errors.New("passed data not long enough for pointer: need 8 bytes")
	}
	addr := binary.NativeEndian.Uint64(data)
	pointeeKey := typeAndAddr{
		irType: uint32(p.Pointee.GetID()),
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

	var address uint64
	pointedValue, dataItemExists := dataItems[pointeeKey]
	if !dataItemExists {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("depth"), //TODO: can we distinguish if it's depth or ran out of buffer space?
		)
	}
	address = pointedValue.Header().Address

	var err error
	pointedType, ok := decoder.program.Types[ir.TypeID(pointedValue.Header().Type)]
	if !ok {
		return fmt.Errorf("no type for pointed type (ID: %d)", pointedValue.Header().Type)
	}
	goKind, ok := pointedType.GetGoKind()
	if !ok {
		return fmt.Errorf("no go kind for type %s (ID: %d)", pointedType.GetName(), pointedType.GetID())
	}
	if goKind != reflect.Pointer {
		if err = writeTokens(enc,
			jsontext.String("address"),
			jsontext.String("0x"+strconv.FormatInt(int64(address), 16)),
		); err != nil {
			return err
		}
	}

	if _, alreadyEncoding := currentlyEncoding[pointeeKey]; !alreadyEncoding && dataItemExists {
		currentlyEncoding[pointeeKey] = struct{}{}
		defer delete(currentlyEncoding, pointeeKey)
		if err = p.pointedDecoderType.encodeValueFields(
			decoder,
			enc,
			dataItems,
			currentlyEncoding,
			pointedValue.Data(),
			valueType,
		); err != nil {
			return fmt.Errorf("could not encode referenced value: %w", err)
		}
	}
	return nil
}

func (s *structureType) irType() ir.Type { return (*ir.StructureType)(s) }
func (s *structureType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	var err error
	if err = writeTokens(enc,
		jsontext.String("fields"),
		jsontext.BeginObject); err != nil {
		return err
	}
	for _, field := range s.Fields {
		if err = writeTokens(enc, jsontext.String(field.Name)); err != nil {
			return err
		}
		fieldType, err := decoder.getDecoderType(field.Type)
		if err != nil {
			return err
		}
		fieldEnd := field.Offset + field.Type.GetByteSize()
		if fieldEnd < field.Offset {
			return fmt.Errorf("overflow in field %s offset calculation", field.Name)
		}
		if fieldEnd > uint32(len(data)) {
			return fmt.Errorf("field %s extends beyond data bounds: need %d bytes, have %d", field.Name, fieldEnd, len(data))
		}

		if err = decoder.encodeValue(enc,
			dataItems,
			currentlyEncoding,
			fieldType,
			data[field.Offset:field.Offset+field.Type.GetByteSize()],
			field.Type.GetName(),
		); err != nil {
			return err
		}
	}
	if err = writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}
	return nil
}

func (a *arrayType) irType() ir.Type { return (*ir.ArrayType)(a) }
func (a *arrayType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	var err error
	elementDecoderType, err := decoder.getDecoderType(a.Element)
	if err != nil {
		return err
	}
	elementSize := int(a.Element.GetByteSize())
	numElements := int(a.Count)
	if err = writeTokens(enc,
		jsontext.String("size"),
		jsontext.String(strconv.Itoa(numElements)),
		jsontext.String("elements"),
		jsontext.BeginArray); err != nil {
		return err
	}

	for i := range numElements {
		offset := i * elementSize
		endIdx := offset + elementSize
		if endIdx < offset {
			return fmt.Errorf("overflow in array element %d offset calculation", i)
		}
		if endIdx > len(data) {
			return fmt.Errorf("array element %d extends beyond data bounds: need %d bytes, have %d", i, endIdx, len(data))
		}
		elementData := data[offset:endIdx]
		if err := decoder.encodeValue(enc,
			dataItems,
			currentlyEncoding,
			elementDecoderType,
			elementData,
			elementDecoderType.irType().GetName(),
		); err != nil {
			return err
		}
	}
	if err = writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	return nil
}

func (s *goSliceHeaderType) irType() ir.Type { return s.GoSliceHeaderType }
func (s *goSliceHeaderType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {

	if len(data) < int(s.ByteSize) {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("no buffer space"),
		)
	}
	if len(data) < 16 {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("insufficient data for slice header: need 16 bytes"),
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
	elementSize := int(s.elementDecoderType.irType().GetByteSize())
	taa := typeAndAddr{
		addr:   address,
		irType: uint32(s.Data.GetID()),
	}
	sliceDataItem, ok := dataItems[taa]
	if !ok {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("no buffer space"),
		)
	}
	var err error
	if err = writeTokens(enc,
		jsontext.String("elements"),
		jsontext.BeginArray); err != nil {
		return err
	}
	sliceLength := int(sliceDataItem.Header().Length) / elementSize
	sliceData := sliceDataItem.Data()
	for i := range int(sliceLength) {
		elementData := sliceData[i*int(s.elementDecoderType.irType().GetByteSize()) : (i+1)*int(s.elementDecoderType.irType().GetByteSize())]
		if err := decoder.encodeValue(enc,
			dataItems,
			currentlyEncoding,
			s.elementDecoderType,
			elementData,
			s.elementDecoderType.irType().GetName(),
		); err != nil {
			return err
		}
	}
	if err = writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	if length > uint64(sliceLength) {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("collectionSize"),
		)
	}
	return nil
}

func (s *goSliceDataType) irType() ir.Type { return (*ir.GoSliceDataType)(s) }
func (s *goSliceDataType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {

	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (s *goStringHeaderType) irType() ir.Type { return (*ir.GoStringHeaderType)(s) }
func (s *goStringHeaderType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {

	var (
		address       uint64
		fieldByteSize uint32
	)
	for _, field := range s.Fields {
		if field.Name == "str" {
			fieldByteSize = field.Type.GetByteSize()
			fieldEnd := field.Offset + fieldByteSize
			if fieldByteSize != 8 {
				return fmt.Errorf("malformed string field 'str': expected 8 bytes, got %d", fieldByteSize)
			}
			if fieldEnd > uint32(len(data)) {
				return fmt.Errorf("string field 'str' extends beyond data bounds: need %d bytes, have %d", fieldEnd, len(data))
			}
			address = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+fieldByteSize])
		}
	}
	if address == 0 {
		return writeTokens(enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		)
	}

	stringValue, ok := dataItems[typeAndAddr{
		irType: uint32(s.Data.GetID()),
		addr:   address,
	}]
	if !ok {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("depth"),
		)
	}
	stringData := stringValue.Data()
	length := stringValue.Header().Length
	if len(stringData) < int(length) {
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("no buffer space"),
		)
	}
	if err := writeTokens(enc,
		jsontext.String("value"),
	); err != nil {
		return err
	}
	str := unsafe.String(unsafe.SliceData(stringData), int(length))
	if err := writeTokens(enc, jsontext.String(str)); err != nil {
		return err
	}
	return nil
}

func (s *goStringDataType) irType() ir.Type { return (*ir.GoStringDataType)(s) }
func (s *goStringDataType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (c *goChannelType) irType() ir.Type { return (*ir.GoChannelType)(c) }
func (c *goChannelType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (e *goEmptyInterfaceType) irType() ir.Type { return (*ir.GoEmptyInterfaceType)(e) }
func (e *goEmptyInterfaceType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (i *goInterfaceType) irType() ir.Type { return (*ir.GoInterfaceType)(i) }
func (i *goInterfaceType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}

func (s *goSubroutineType) irType() ir.Type { return (*ir.GoSubroutineType)(s) }
func (s *goSubroutineType) encodeValueFields(
	decoder *Decoder,
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	data []byte,
	valueType string) error {
	return writeTokens(enc,
		jsontext.String("notCapturedReason"),
		jsontext.String("unimplemented"),
	)
}
