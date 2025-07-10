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

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

type logger struct {
	Name       string `json:"name"`
	Method     string `json:"method"`
	Version    int    `json:"version"`
	ThreadID   int    `json:"thread_id"`
	ThreadName string `json:"thread_name"`
}

type debuggerData struct {
	Snapshot snapshotData `json:"snapshot"`
}

type snapshotData struct {
	decoder *Decoder

	// static fields:
	ID        uuid.UUID `json:"id"`
	Timestamp int       `json:"timestamp"`
	Language  string    `json:"language"`

	// dynamic fields:
	Stack    stackData   `json:"stack"`
	Probe    probeData   `json:"probe"`
	Captures captureData `json:"captures"`
}

type probeData struct {
	ID       string       `json:"id,omitempty"`
	Location locationData `json:"location"`
}

type locationData struct {
	Method string `json:"method,omitempty"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitzero"`
	Type   string `json:"type,omitempty"`
}

type captureData struct {
	Entry capturePointData `json:"entry"`
}

type capturePointData struct {
	Arguments argumentsData `json:"arguments"`
}

type argumentsData struct {
	rootData []byte
	rootType *ir.EventRootType
	event    Event
	decoder  *Decoder
}

func (ad *argumentsData) MarshalJSONTo(enc *jsontext.Encoder) error {
	var err error
	currentlyEncoding := map[typeAndAddr]struct{}{}

	if err = writeTokens(enc,
		jsontext.BeginObject,
	); err != nil {
		return err
	}

	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	for _, expr := range ad.rootType.Expressions {
		parameterType := expr.Expression.Type
		parameterData := ad.rootData[expr.Offset : expr.Offset+parameterType.GetByteSize()]
		if err = writeTokens(enc, jsontext.String(expr.Name)); err != nil {
			return err
		}
		err = ad.decoder.encodeValue(enc,
			ad.decoder.addressReferenceCount,
			currentlyEncoding,
			parameterType,
			parameterData,
			parameterType.GetName(),
		)
		if err != nil {
			return fmt.Errorf("error parsing data for field %s: %w", ad.rootType.Name, err)
		}
	}
	if err = writeTokens(enc,
		jsontext.EndObject,
	); err != nil {
		return err
	}
	return nil
}

type stackData struct {
	frames []symbol.StackFrame
}

func (sd *stackData) MarshalJSONTo(enc *jsontext.Encoder) error {
	var err error
	if err = writeTokens(enc, jsontext.BeginArray); err != nil {
		return err
	}

	for i := range sd.frames {
		for j := range sd.frames[i].Lines {
			if err = json.MarshalEncode(
				enc, (*stackLine)(&sd.frames[i].Lines[j]),
			); err != nil {
				return err
			}
		}
	}
	if err = writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	return nil
}

type stackLine gosym.GoLocation

func (sl *stackLine) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := writeTokens(enc,
		jsontext.BeginObject,
		jsontext.String("function"),
		jsontext.String(sl.Function),
		jsontext.String("fileName"),
		jsontext.String(sl.File),
		jsontext.String("lineNumber"),
		jsontext.Int(int64(sl.Line)),
		jsontext.EndObject,
	); err != nil {
		return err
	}
	return nil
}

func (d *Decoder) encodeValue(
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	irType ir.Type,
	data []byte,
	valueType string,
) error {
	if err := writeTokens(enc,
		jsontext.BeginObject,
		jsontext.String("type"),
		jsontext.String(valueType),
	); err != nil {
		return err
	}
	if err := d.encodeValueFields(enc,
		dataItems,
		currentlyEncoding,
		irType,
		data,
		valueType,
	); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}

	return nil
}

func (d *Decoder) encodeValueFields(
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	irType ir.Type,
	data []byte,
	valueType string,
) error {
	switch v := irType.(type) {
	case *ir.BaseType:
		if err := writeTokens(enc,
			jsontext.String("value"),
		); err != nil {
			return err
		}
		return encodeBaseTypeValue(enc, v, data)
	case *ir.PointerType:
		if len(data) < int(v.GetByteSize()) {
			return errors.New("passed data not long enough for pointer")
		}
		addr := binary.NativeEndian.Uint64(data)
		key := typeAndAddr{
			irType: uint32(v.Pointee.GetID()),
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

		var err error
		pointedType, ok := d.program.Types[ir.TypeID(pointedValue.Header().Type)]
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

		if _, alreadyEncoding := currentlyEncoding[key]; !alreadyEncoding && dataItemExists {
			currentlyEncoding[key] = struct{}{}
			defer delete(currentlyEncoding, key)
			if err = d.encodeValueFields(enc,
				dataItems,
				currentlyEncoding,
				d.program.Types[ir.TypeID(pointedValue.Header().Type)],
				pointedValue.Data(),
				valueType,
			); err != nil {
				return fmt.Errorf("could not encode referenced value: %w", err)
			}
		}
		return nil
	case *ir.StructureType:
		var err error
		if err = writeTokens(enc,
			jsontext.String("fields"),
			jsontext.BeginObject); err != nil {
			return err
		}
		structure := irType.(*ir.StructureType)
		for _, field := range structure.Fields {
			if err = writeTokens(enc, jsontext.String(field.Name)); err != nil {
				return err
			}
			if err = d.encodeValue(enc,
				dataItems,
				currentlyEncoding,
				field.Type,
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
	case *ir.ArrayType:
		var err error
		if err = writeTokens(enc,
			jsontext.String("elements"),
			jsontext.BeginArray); err != nil {
			return err
		}

		elementType := v.Element
		elementSize := int(elementType.GetByteSize())
		numElements := int(v.Count)
		for i := range numElements {
			elementData := data[i*elementSize : (i+1)*elementSize]
			if err := d.encodeValue(enc,
				dataItems,
				currentlyEncoding,
				v.Element,
				elementData,
				"",
			); err != nil {
				return err
			}
		}
		if err = writeTokens(enc, jsontext.EndArray); err != nil {
			return err
		}
		return nil
	case *ir.GoEmptyInterfaceType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoInterfaceType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoSliceHeaderType:
		if len(data) < int(v.ByteSize) {
			return writeTokens(enc,
				jsontext.String("notCapturedReason"),
				jsontext.String("no buffer space"),
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
		elementType := v.Data.ID
		elementSize := int(v.Data.Element.GetByteSize())
		sliceDataItem, ok := dataItems[typeAndAddr{
			addr:   address,
			irType: uint32(elementType),
		}]
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
			elementData := sliceData[i*elementSize : (i+1)*elementSize]
			if err := d.encodeValue(enc,
				dataItems,
				currentlyEncoding,
				v.Data.Element,
				elementData,
				v.Data.Element.GetName(),
			); err != nil {
				return err
			}
		}
		if err = writeTokens(enc, jsontext.EndArray); err != nil {
			return err
		}
		return nil
	case *ir.GoChannelType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoStringHeaderType:
		var (
			address       uint64
			fieldByteSize uint32
		)
		for _, field := range v.Fields {
			if field.Name == "str" {
				fieldByteSize = field.Type.GetByteSize()
				if fieldByteSize != 8 || len(data) < int(field.Offset+fieldByteSize) {
					return fmt.Errorf("malformed string field 'str': field size %d != 8 or data length %d < required %d", fieldByteSize, len(data), field.Offset+fieldByteSize)
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
			irType: uint32(v.Data.GetID()),
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
	case *ir.GoMapType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoHMapHeaderType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoHMapBucketType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoSwissMapHeaderType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoSwissMapGroupsType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.EventRootType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoSliceDataType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoStringDataType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	case *ir.GoSubroutineType:
		return writeTokens(enc,
			jsontext.String("notCapturedReason"),
			jsontext.String("unimplemented"),
		)
	}
	return fmt.Errorf("invalid type %s (ID: %d)", irType.GetName(), irType.GetID())
}

func encodeBaseTypeValue(enc *jsontext.Encoder, irType *ir.BaseType, data []byte) error {
	kind, ok := irType.GetGoKind()
	if !ok {
		return fmt.Errorf("no go kind for type %s (ID: %d)", irType.GetName(), irType.GetID())
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

func writeTokens(enc *jsontext.Encoder, tokens ...jsontext.Token) error {
	var err error
	for i := range tokens {
		err = enc.WriteToken(tokens[i])
		if err != nil {
			return err
		}
	}
	return nil
}
