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
	"io"
	"log"
	"math"
	"reflect"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

type probeEvent struct {
	event *ir.Event
	probe *ir.Probe
}

type Decoder struct {
	uploader               *jsontext.Encoder
	program                *ir.Program
	typesToProbesAndEvents map[ir.TypeID]probeEvent
}

func NewDecoder(program *ir.Program, out io.Writer) (*Decoder, error) {
	decoder := &Decoder{
		program:                program,
		uploader:               jsontext.NewEncoder(out),
		typesToProbesAndEvents: make(map[ir.TypeID]probeEvent),
	}
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			decoder.typesToProbesAndEvents[event.Type.ID] = probeEvent{
				event: event,
				probe: probe,
			}
		}
	}
	return decoder, nil
}

func (d *Decoder) Decode(buf []byte) error {
	iterator, err := d.newDataEventIterator(buf)
	if err != nil {
		return err
	}

	if iterator.eventHeader.Prog_id != uint32(d.program.ID) {
		log.Printf("expected program ID did not match ID in buffer: %d != %d", iterator.eventHeader.Prog_id, d.program.ID)
	}

	// The first data item in the buffer will always be an EventRoot
	// which lays out the structure of the event.
	var firstDataItem *dataItem
	firstDataItem, err = iterator.nextDataItem()
	if err != nil && !errors.Is(err, finishedIterating) {
		return fmt.Errorf("could not get first root data item from buffer: %s", err)
	}

	eventRoot, ok := d.program.Types[ir.TypeID(firstDataItem.header.Type)].(*ir.EventRootType)
	if !ok {
		return errors.New("expected event of type root first")
	}

	err = d.uploader.WriteToken(jsontext.BeginObject)
	if err != nil {
		return err
	}
	defer d.uploader.WriteToken(jsontext.EndObject)

	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	for _, expr := range eventRoot.Expressions {
		parameterType := expr.Expression.Type
		parameterData := firstDataItem.data[expr.Offset : expr.Offset+parameterType.GetByteSize()]
		out, err := d.parseDataToGoValue(iterator.addressPassMap, parameterType, parameterData)
		if err != nil {
			return fmt.Errorf("error parsing data for field %s: %w", eventRoot.Name, err)
		}
		err = d.uploader.WriteToken(jsontext.String(expr.Name))
		if err != nil {
			return err
		}
		err = json.MarshalEncode(d.uploader, out)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) parseDataToGoValue(addressPassMap map[uint64]*dataItem, irType ir.Type, data []byte) (any, error) {
	switch v := irType.(type) {
	case *ir.BaseType:
		return d.parseDataToGoBaseTypeValue(v, data)
	case *ir.PointerType:
		if len(data) < int(v.GetByteSize()) {
			return nil, errors.New("passed data not long enough for pointer")
		}
		addr := binary.NativeEndian.Uint64(data)
		pointedValue, ok := addressPassMap[addr]
		if !ok {
			return nil, errors.New("pointer not found in address pass map")
		}
		pointeeValue, err := d.parseDataToGoValue(addressPassMap, d.program.Types[ir.TypeID(pointedValue.header.Type)], pointedValue.data)
		if err != nil {
			return nil, fmt.Errorf("could not get pointed-at value: %s", err)
		}
		m := make(map[string]any, 1)
		m[strconv.FormatUint(addr, 10)] = pointeeValue
		return m, nil
	case *ir.StructureType:
		structFields := map[string]any{}
		structure := irType.(*ir.StructureType)
		for _, field := range structure.Fields {
			v, err := d.parseDataToGoValue(addressPassMap, field.Type, data[field.Offset:field.Offset+uint32(field.Type.GetByteSize())])
			if err != nil {
				return nil, err
			}
			structFields[field.Name] = v
		}
		return structFields, nil
	case *ir.ArrayType:
		//TODO
	case *ir.GoEmptyInterfaceType:
		//TODO
	case *ir.GoInterfaceType:
		//TODO
	case *ir.GoSliceHeaderType:
		sliceHeader := irType.(*ir.GoSliceHeaderType)
		var array, length, cap uint64
		for _, field := range sliceHeader.Fields {
			if field.Name == "array" {
				if uint32(field.Type.GetByteSize()) != 8 || len(data) < int(field.Offset+field.Type.GetByteSize()) {
					return nil, errors.New("malformed string field 'str'")
				}
				array = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else if field.Name == "len" {
				if field.Type.GetByteSize() != 8 {
					return nil, errors.New("malformed string field 'len'")
				}
				length = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else if field.Name == "cap" {
				if field.Type.GetByteSize() != 8 {
					return nil, errors.New("malformed string field 'len'")
				}
				cap = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else {
				fmt.Println(field.Name)
			}
		}

		//TODO
		fmt.Println(array, length, cap)

	case *ir.GoSliceDataType:
		//TODO
	case *ir.GoChannelType:
		//TODO
	case *ir.GoStringHeaderType:
		stringHeader := irType.(*ir.GoStringHeaderType)
		var address, length uint64
		for _, field := range stringHeader.Fields {
			if field.Name == "str" {
				if uint32(field.Type.GetByteSize()) != 8 || len(data) < int(field.Offset+field.Type.GetByteSize()) {
					return nil, errors.New("malformed string field 'str'")
				}
				address = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else if field.Name == "len" {
				if field.Type.GetByteSize() != 8 {
					return nil, errors.New("malformed string field 'len'")
				}
				length = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			}
		}

		// FIXME: Once string captures actually work it may be correct to call parseDataToGoValue on the type from addressPassMap
		// and have the reading of GoStringDataType handled in this switch instead
		stringValue, ok := addressPassMap[address]
		if !ok {
			return nil, fmt.Errorf("string content not present in data items")
		}
		if len(stringValue.data) < int(length) {
			return nil, errors.New("string content not long enough for known length")
		}
		return string(stringValue.data[:length]), nil
	case *ir.GoStringDataType:
		//TODO
	case *ir.GoMapType:
		//TODO
	case *ir.GoHMapHeaderType:
		//TODO
	case *ir.GoHMapBucketType:
		//TODO
	case *ir.GoSwissMapHeaderType:
		//TODO
	case *ir.GoSwissMapGroupsType:
		//TODO
	case *ir.EventRootType:
		//TODO
	default:
		return nil, errors.New("invalid type")
	}
	return nil, nil
}

func (d *Decoder) parseDataToGoBaseTypeValue(irType *ir.BaseType, data []byte) (any, error) {
	kind, ok := irType.GetGoKind()
	if !ok {
		return nil, errors.New("No go kind")
	}
	switch kind {
	case reflect.Bool:
		if len(data) < 1 {
			return nil, errors.New("passed data not long enough for bool")
		}
		return data[0] == 1, nil
	case reflect.Int:
		if len(data) < 8 {
			return nil, errors.New("passed data not long enough for int")
		}
		return int(binary.NativeEndian.Uint64(data)), nil
	case reflect.Int8:
		if len(data) < 1 {
			return nil, errors.New("passed data not long enough for int8")
		}
		return int8(data[0]), nil
	case reflect.Int16:
		if len(data) < 2 {
			return nil, errors.New("passed data not long enough for int16")
		}
		return int16(binary.NativeEndian.Uint16(data)), nil
	case reflect.Int32:
		if len(data) != 4 {
			return nil, errors.New("passed data not long enough for int32")
		}
		return int32(binary.NativeEndian.Uint32(data)), nil
	case reflect.Int64:
		if len(data) != 8 {
			return nil, errors.New("passed data not long enough for int64")
		}
		return int64(binary.NativeEndian.Uint64(data)), nil
	case reflect.Uint:
		if len(data) != 8 {
			return nil, errors.New("passed data not long enough for uint")
		}
		return uint(binary.NativeEndian.Uint64(data)), nil
	case reflect.Uint8:
		if len(data) != 1 {
			return nil, errors.New("passed data not long enough for uint8")
		}
		return uint8(data[0]), nil
	case reflect.Uint16:
		if len(data) != 2 {
			return nil, errors.New("passed data not long enough for uint16")
		}
		return uint16(binary.NativeEndian.Uint16(data)), nil
	case reflect.Uint32:
		if len(data) != 4 {
			return nil, errors.New("passed data not long enough for uint32")
		}
		return uint32(binary.NativeEndian.Uint32(data)), nil
	case reflect.Uint64:
		if len(data) != 8 {
			return nil, errors.New("passed data not long enough for uint64")
		}
		return binary.NativeEndian.Uint64(data), nil
	case reflect.Uintptr:
		if len(data) != 8 {
			return nil, errors.New("passed data not long enough for uintptr")
		}
		return uintptr(binary.NativeEndian.Uint64(data)), nil
	case reflect.Float32:
		if len(data) != 4 {
			return nil, errors.New("passed data not long enough for float32")
		}
		return math.Float32frombits(binary.NativeEndian.Uint32(data)), nil
	case reflect.Float64:
		if len(data) != 8 {
			return nil, errors.New("passed data not long enough for float64")
		}
		return math.Float64frombits(binary.NativeEndian.Uint64(data)), nil
	case reflect.Complex64:
		if len(data) != 8 {
			return nil, errors.New("passed data not long enough for complex64")
		}
		real := math.Float32frombits(binary.NativeEndian.Uint32(data[0:4]))
		imag := math.Float32frombits(binary.NativeEndian.Uint32(data[4:8]))
		return complex(real, imag), nil
	case reflect.Complex128:
		if len(data) != 16 {
			return nil, errors.New("passed data not long enough for complex128")
		}
		real := math.Float64frombits(binary.NativeEndian.Uint64(data[0:8]))
		imag := math.Float64frombits(binary.NativeEndian.Uint64(data[8:16]))
		return complex(real, imag), nil
	case reflect.Array:
		return nil, errors.New("arrays are not base types")
	case reflect.Chan:
		return nil, errors.New("channels are not base types")
	case reflect.Func:
		return nil, errors.New("funcs are not base types")
	case reflect.Interface:
		return nil, errors.New("interfaces are not base types")
	case reflect.Map:
		return nil, errors.New("maps are not base types")
	case reflect.Ptr:
		return nil, errors.New("ptrs are not base types")
	case reflect.Slice:
		return nil, errors.New("slices are not base types")
	case reflect.String:
		return nil, errors.New("strings are not base types")
	case reflect.Struct:
		return nil, errors.New("structs are not base types")
	case reflect.UnsafePointer:
		return nil, errors.New("unsafe pointers are not base types")
	default:
		return nil, errors.New("invalid base type")
	}
}
