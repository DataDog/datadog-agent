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
	strFieldSize   uint8
	lenFieldOffset uint32
	lenFieldSize   uint8
}
type goStringDataType ir.GoStringDataType
type goMapType ir.GoMapType
type goHMapHeaderType ir.GoHMapHeaderType
type goHMapBucketType ir.GoHMapBucketType
type goSwissMapHeaderType struct {
	*ir.GoSwissMapHeaderType
	// Fields related to user defined key and value types
	keyTypeID     ir.TypeID
	valueTypeID   ir.TypeID
	keyTypeSize   uint32
	valueTypeSize uint32
	keyTypeName   string
	valueTypeName string

	// Fields in go swiss map internal representation
	dirLenOffset     uint32
	dirLenSize       uint8
	dirPtrOffset     uint32
	dirPtrSize       uint8
	ctrlOffset       uint32
	ctrlSize         uint8
	slotsOffset      uint32
	slotsSize        uint32
	groupFieldOffset uint32
	groupFieldSize   uint8
	dataFieldOffset  uint32
	dataFieldSize    uint8
	tableTypeID      ir.TypeID
	groupTypeID      ir.TypeID
	elementTypeSize  uint32
	usedOffset       uint32
	usedSize         uint8
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
		var sizeChecks = []struct {
			name  string
			value uint32
		}{
			{"dirLenSize", dirLenSize},
			{"dirPtrSize", dirPtrSize},
			{"ctrlSize", ctrlSize},
			{"groupFieldSize", groupFieldSize},
			{"dataFieldSize", dataFieldSize},
			{"usedSize", usedSize},
		}
		for _, check := range sizeChecks {
			// We cast to uint8 expecting that the values are going to be small enough, given they
			// are known to go swiss map implementation, but check just in case.
			if check.value > math.MaxUint8 {
				return nil, fmt.Errorf("%s is too large: %d", check.name, check.value)
			}
		}
		return &goSwissMapHeaderType{
			GoSwissMapHeaderType: s,
			// Fields related to user defined key and value types
			keyTypeID:     keyField.Type.GetID(),
			valueTypeID:   elem.Type.GetID(),
			keyTypeSize:   keyField.Type.GetByteSize(),
			valueTypeSize: elem.Type.GetByteSize(),
			keyTypeName:   keyField.Type.GetName(),
			valueTypeName: elem.Type.GetName(),

			// Fields in go swiss map internal representation
			// Seehttps://github.com/golang/go/blob/cd3655a8243b5f52b6a274a0aba5e01d998906c0/src/internal/runtime/maps/map.go#L195
			dirLenOffset:     dirLenOffset,
			dirLenSize:       uint8(dirLenSize),
			dirPtrOffset:     dirPtrOffset,
			dirPtrSize:       uint8(dirPtrSize),
			ctrlOffset:       ctrlOffset,
			ctrlSize:         uint8(ctrlSize),
			slotsOffset:      slotsField.Offset,
			slotsSize:        slotsFieldType.GetByteSize(),
			groupFieldOffset: groupFieldOffset,
			groupFieldSize:   uint8(groupFieldSize),
			dataFieldOffset:  dataFieldOffset,
			dataFieldSize:    uint8(dataFieldSize),
			tableTypeID:      tablePtrType.Pointee.GetID(),
			groupTypeID:      s.GroupType.GetID(),
			elementTypeSize:  uint32(groupType.GroupSliceType.Element.GetByteSize()),
			usedOffset:       usedOffset,
			usedSize:         uint8(usedSize),
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
			strFieldSize:       uint8(strField.Type.GetByteSize()),
			lenFieldOffset:     lenField.Offset,
			lenFieldSize:       uint8(lenField.Type.GetByteSize()),
		}, nil
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
		return writeTokens(enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint16(data)), 10)))
	case reflect.Int32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for int32")
		}
		return writeTokens(enc, jsontext.String(strconv.FormatInt(int64(binary.NativeEndian.Uint32(data)), 10)))
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
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (m *goMapType) irType() ir.Type { return (*ir.GoMapType)(m) }
func (m *goMapType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
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
		if err := writeTokens(enc,
			jsontext.String("isNull"),
			jsontext.Bool(true),
		); err != nil {
			return err
		}
		return nil
	}
	keyValue, dataItemExists := d.dataItems[key]
	if !dataItemExists {
		return writeTokens(enc,
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	keyValueHeader := keyValue.Header()
	if keyValueHeader == nil {
		return writeTokens(enc,
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	headerType, ok := d.program.Types[ir.TypeID(keyValueHeader.Type)]
	if !ok {
		return fmt.Errorf("no type for header type (ID: %d)", keyValueHeader.Type)
	}
	headerDecoderType, ok := d.decoderTypes[headerType.GetID()]
	if !ok {
		return fmt.Errorf("no decoder type found for header type (ID: %d)", keyValue.Header().Type)
	}
	return headerDecoderType.encodeValueFields(
		d,
		enc,
		keyValue.Data(),
	)
}

func (h *goHMapHeaderType) irType() ir.Type { return (*ir.GoHMapHeaderType)(h) }
func (h *goHMapHeaderType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (b *goHMapBucketType) irType() ir.Type { return (*ir.GoHMapBucketType)(b) }
func (b *goHMapBucketType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
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
		jsontext.Int(used),
	); err != nil {
		return err
	}
	dirLen := int64(binary.NativeEndian.Uint64(data[s.dirLenOffset : s.dirLenOffset+uint32(s.dirLenSize)]))
	dirPtr := binary.NativeEndian.Uint64(data[s.dirPtrOffset : s.dirPtrOffset+uint32(s.dirPtrSize)])
	if err := writeTokens(
		enc, jsontext.String("entries"), jsontext.BeginArray,
	); err != nil {
		return err
	}
	if dirLen == 0 {
		// This is a 'small' swiss map where there's only one group.
		// We can collect the data item for the group directly.
		groupDataItem, ok := d.dataItems[typeAndAddr{
			irType: uint32(s.groupTypeID),
			addr:   dirPtr,
		}]
		if !ok {
			return writeTokens(enc,
				jsontext.EndArray,
				notCapturedReason,
				notCapturedReasonDepth,
			)
		}
		totalElementsEncoded, err := s.encodeSwissMapGroup(d, enc, groupDataItem.Data())
		if err != nil {
			return err
		}
		if used > int64(totalElementsEncoded) {
			if err := writeTokens(enc,
				notCapturedReason,
				notCapturedReasonPruned,
			); err != nil {
				return err
			}
		}
	} else {
		// This is a 'large' swiss map where there are multiple groups of data/control words
		// We need to collect the data items for the table pointers first.
		tablePtrSliceDataItemPtr, ok := d.dataItems[typeAndAddr{
			irType: uint32(s.TablePtrSliceType.GetID()),
			addr:   dirPtr,
		}]
		if !ok {
			return writeTokens(enc,
				jsontext.EndArray,
				notCapturedReason,
				notCapturedReasonDepth,
			)
		}
		tablePtrSliceDataItem, ok := d.dataItems[typeAndAddr{
			irType: tablePtrSliceDataItemPtr.Header().Type,
			addr:   tablePtrSliceDataItemPtr.Header().Address,
		}]
		if !ok {
			return writeTokens(enc,
				jsontext.EndArray,
				notCapturedReason,
				notCapturedReasonDepth,
			)
		}
		totalElementsEncoded, err := s.encodeSwissMapTables(d, enc, tablePtrSliceDataItem)
		if err != nil {
			return err
		}
		if used > int64(totalElementsEncoded) {
			if err := writeTokens(enc,
				notCapturedReason,
				notCapturedReasonPruned,
			); err != nil {
				return err
			}
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
		notCapturedReason,
		notCapturedReasonUnimplemented,
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
	pointedValue, dataItemExists := d.dataItems[pointeeKey]
	if !dataItemExists {
		return writeTokens(enc,
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	address = pointedValue.Header().Address
	goKind, ok := p.Pointee.GetGoKind()
	if !ok {
		return fmt.Errorf("no go kind for type %s (ID: %d)", p.Pointee.GetName(), p.Pointee.GetID())
	}
	// We only encode the address for non-pointer types to avoid collisions of the 'address' field
	// in cases of pointers to pointers. In a scenario like `**int`, only the final pointer that's
	// closest to the actual data will be encoded.
	if goKind != reflect.Pointer {
		if err := writeTokens(enc,
			jsontext.String("address"),
			jsontext.String("0x"+strconv.FormatInt(int64(address), 16)),
		); err != nil {
			return err
		}
	}

	if _, alreadyEncoding := d.currentlyEncoding[pointeeKey]; !alreadyEncoding && dataItemExists {
		d.currentlyEncoding[pointeeKey] = struct{}{}
		defer delete(d.currentlyEncoding, pointeeKey)
		pointeeDecoderType, ok := d.decoderTypes[p.Pointee.GetID()]
		if !ok {
			return fmt.Errorf("no decoder type found for pointee type (ID: %d)", p.Pointee.GetID())
		}
		if err := pointeeDecoderType.encodeValueFields(
			d,
			enc,
			pointedValue.Data(),
		); err != nil {
			return fmt.Errorf("could not encode referenced value: %w", err)
		}
	}
	return nil
}

func (s *structureType) irType() ir.Type { return (*ir.StructureType)(s) }
func (s *structureType) encodeValueFields(
	d *Decoder,
	enc *jsontext.Encoder,
	data []byte,
) error {
	var err error
	if err = writeTokens(enc,
		jsontext.String("fields"),
		jsontext.BeginObject); err != nil {
		return err
	}
	for field := range s.irType().(*ir.StructureType).Fields() {
		if err = writeTokens(enc, jsontext.String(field.Name)); err != nil {
			return err
		}
		fieldEnd := field.Offset + field.Type.GetByteSize()
		if fieldEnd > uint32(len(data)) {
			return fmt.Errorf("field %s extends beyond data bounds: need %d bytes, have %d", field.Name, fieldEnd, len(data))
		}

		if err = d.encodeValue(enc,
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
			notCapturedReason,
			notCapturedReasonPruned,
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
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	if len(data) < 16 {
		return writeTokens(enc,
			notCapturedReason,
			notCapturedReasonPruned,
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
		jsontext.Uint(length)); err != nil {
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
			notCapturedReason,
			notCapturedReasonPruned,
		)
	}
	if err := writeTokens(enc,
		jsontext.String("elements"),
		jsontext.BeginArray); err != nil {
		return err
	}
	sliceLength := int(sliceDataItem.Header().Length) / elementSize
	sliceData := sliceDataItem.Data()
	var notCaptured = false
	for i := range int(sliceLength) {
		elementData := sliceData[i*int(s.Data.Element.GetByteSize()) : (i+1)*int(s.Data.Element.GetByteSize())]
		if err := d.encodeValue(enc,
			s.Data.Element.GetID(),
			elementData,
			s.Data.Element.GetName(),
		); err != nil {
			log.Tracef("could not encode slice element: %v", err)
			notCapturedReason = notCapturedReasonPruned
			break
		}
	}
	if err := writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	if length > uint64(sliceLength) {
		return writeTokens(enc,
			notCapturedReason,
			notCapturedReasonCollectionSize,
		)
	} else if notCaptured {
		return writeTokens(enc,
			notCapturedReason,
			notCapturedReasonPruned,
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
		notCapturedReason,
		notCapturedReasonUnimplemented,
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
			notCapturedReason,
			notCapturedReasonLength,
		)
	}
	address := binary.NativeEndian.Uint64(data[s.strFieldOffset : s.strFieldOffset+uint32(s.strFieldSize)])
	if address == 0 {
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
			notCapturedReason,
			notCapturedReasonDepth,
		)
	}
	stringData := stringValue.Data()
	length := stringValue.Header().Length
	realLength := binary.NativeEndian.Uint64(data[s.lenFieldOffset : s.lenFieldOffset+uint32(s.lenFieldSize)])
	if realLength > uint64(length) {
		// We captured partial data for the string, report truncation
		if err := writeTokens(enc,
			jsontext.String("size"),
			jsontext.Uint(realLength),
			truncated,
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
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (c *goChannelType) irType() ir.Type { return (*ir.GoChannelType)(c) }
func (c *goChannelType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (e *goEmptyInterfaceType) irType() ir.Type { return (*ir.GoEmptyInterfaceType)(e) }
func (e *goEmptyInterfaceType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (i *goInterfaceType) irType() ir.Type { return (*ir.GoInterfaceType)(i) }
func (i *goInterfaceType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
		notCapturedReason,
		notCapturedReasonUnimplemented,
	)
}

func (s *goSubroutineType) irType() ir.Type { return (*ir.GoSubroutineType)(s) }
func (s *goSubroutineType) encodeValueFields(
	_ *Decoder,
	enc *jsontext.Encoder,
	_ []byte,
) error {
	return writeTokens(enc,
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
