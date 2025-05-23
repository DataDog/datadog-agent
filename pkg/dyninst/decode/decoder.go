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
	"math"
	"os"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"

	"github.com/go-json-experiment/json"
)

type probeEvent struct {
	event *ir.Event
	probe *ir.Probe
}

type uploader interface {
	io.Writer
}

type Decoder struct {
	uploader               uploader
	program                *ir.Program
	typesToProbesAndEvents map[ir.TypeID]probeEvent
}

func NewDecoder(program *ir.Program) *Decoder {
	decoder := &Decoder{
		program:                program,
		uploader:               os.Stdout,
		typesToProbesAndEvents: make(map[ir.TypeID]probeEvent),
	}
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			decoder.typesToProbesAndEvents[event.Type.ID] = probeEvent{
				event: &event,
				probe: probe,
			}
		}
	}
	return decoder
}

func (d *Decoder) Decode(buf []byte) error {
	iterator, err := newDataEventIterator(buf)
	if err != nil {
		return err
	}

	if iterator.eventHeader.Prog_id != uint32(d.program.ID) {
		return fmt.Errorf("expected program ID did not match ID in buffer: %d != %d", iterator.eventHeader.Prog_id, d.program.ID)
	}

	// The first data item in the buffer will always be an EventRoot
	// which lays out the structure of the event.
	var firstDataItem *dataItem
	firstDataItem, err = iterator.nextDataItem()
	if err != nil && !errors.Is(err, finishedIterating) {
		return fmt.Errorf("could not get first root data item from buffer: %s", err)
	}

	eventRoot, ok := d.program.Types[firstDataItem.header.Type].(*ir.EventRootType)
	if !ok {
		return errors.New("expected event of type root first")
	}

	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	output := map[string]any{}
	for _, expr := range eventRoot.Expressions {
		parameterType := expr.Expression.Type
		parameterData := firstDataItem.data[expr.Offset : expr.Offset+parameterType.GetByteSize()]
		output[eventRoot.Name], err = d.parseDataToGoValue(iterator.addressPassMap, parameterType, parameterData)
		if err != nil {
			return fmt.Errorf("error parsing data for field %s: %w", eventRoot.Name, err)
		}
	}
	// Write the collected data as json to the uploader
	err = json.MarshalWrite(d.uploader, output)
	if err != nil {
		return err
	}
	return nil
}

func (d *Decoder) parseDataToGoValue(addressPassMap map[uint64]*dataItem, irType ir.Type, data []byte) (any, error) {
	switch v := irType.(type) {
	case *ir.BaseType:
		return d.parseDataToGoBaseTypeValue(v, data)
	case *ir.PointerType:
		// For pointers, we just return the raw address as a uint64
		if len(data) < v.GetByteSize() {
			return nil, errors.New("passed data not long enough for pointer")
		}
		addr := binary.NativeEndian.Uint64(data)
		pointedValue, ok := addressPassMap[addr]
		if !ok {
			return nil, errors.New("pointer not found in address pass map")
		}

		return d.parseDataToGoValue(addressPassMap, d.program.Types[pointedValue.header.Type], pointedValue.data)
	case *ir.StructureType:
		structFields := map[string]any{}
		structure := irType.(*ir.StructureType)
		for _, field := range structure.Fields {
			fieldType := d.program.Types[field.Type]
			v, err := d.parseDataToGoValue(addressPassMap, fieldType, data[field.Offset:field.Offset+uint32(fieldType.GetByteSize())])
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
		sliceHeaderFields := map[string]any{}
		sliceHeader := irType.(*ir.StructureType)
		for _, field := range sliceHeader.Fields {
			fieldType := d.program.Types[field.Type]
			v, err := d.parseDataToGoValue(addressPassMap, fieldType, data[field.Offset:field.Offset+uint32(fieldType.GetByteSize())])
			if err != nil {
				return nil, err
			}
			sliceHeaderFields[field.Name] = v
		}

		fmt.Println(sliceHeaderFields["ptr"])
		fmt.Println(sliceHeaderFields["len"])
		fmt.Println(sliceHeaderFields["cap"])
		// addressPassMap[sliceHeaderFields["ptr"]]
		return sliceHeaderFields, nil
	case *ir.GoSliceDataType:
		//TODO
	case *ir.GoChannelType:
		//TODO
	case *ir.GoStringHeaderType:
		stringHeaderFields := map[string]any{}
		stringHeader := irType.(*ir.StructureType)
		for _, field := range stringHeader.Fields {
			fieldType := d.program.Types[field.Type]
			v, err := d.parseDataToGoValue(addressPassMap, fieldType, data[field.Offset:field.Offset+uint32(fieldType.GetByteSize())])
			if err != nil {
				return nil, err
			}
			stringHeaderFields[field.Name] = v
		}

		fmt.Println(stringHeaderFields["ptr"])
		fmt.Println(stringHeaderFields["len"])
		// addressPassMap[stringHeaderFields["ptr"]]
		return stringHeaderFields, nil
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

	// // None of these would be from GoBaseType, right?
	// case reflect.Array:
	// 	return nil, errors.New("arrays are not base types")
	// case reflect.Chan:
	// 	return nil, errors.New("channels are not base types")
	// case reflect.Func:
	// 	return nil, errors.New("funcs are not base types")
	// case reflect.Interface:
	// 	return nil, errors.New("interfaces are not base types")
	// case reflect.Map:
	// 	return nil, errors.New("maps are not base types")
	// case reflect.Ptr:
	// 	return nil, errors.New("ptrs are not base types")
	// case reflect.Slice:
	// 	return nil, errors.New("slices are not base types")
	// case reflect.String:
	// 	return nil, errors.New("strings are not base types")
	// case reflect.Struct:
	// 	return nil, errors.New("structs are not base types")
	// case reflect.UnsafePointer:
	// 	return nil, errors.New("unsafe pointers are not base types")

	default:
		return nil, errors.New("invalid base type")
	}
}
