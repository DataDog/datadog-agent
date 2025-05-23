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
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"

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

var errorUnimplemented = errors.New("errorUnimplemented type")

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

func (d *Decoder) Decode(event Event) error {
	counter := 0
	var (
		rootData  []byte
		eventRoot *ir.EventRootType
		ok        bool
		err       error
		frames    []uint64
	)

	frames, err = event.stackFrames()
	if err != nil {
		return err
	}

	err = d.uploader.WriteToken(jsontext.BeginObject)
	if err != nil {
		return err
	}

	d.uploader.WriteToken(jsontext.String("stack_frames"))
	d.uploader.WriteToken(jsontext.BeginArray)
	for i := range frames {
		d.uploader.WriteToken(jsontext.Uint(frames[i]))
	}
	d.uploader.WriteToken(jsontext.EndArray)

	err = d.uploader.WriteToken(jsontext.String("captures"))
	if err != nil {
		return err
	}
	err = d.uploader.WriteToken(jsontext.BeginObject)
	if err != nil {
		return err
	}
	itemsInMemory := map[uint64]*dataItem{}
	for item, err := range event.dataItems() {
		if err != nil && errors.Is(err, finishedIterating) {
			return nil
		} else if err != nil {
			return err
		}
		if counter == 0 {
			// item is EventRoot
			rootData = item.data
			eventRoot, ok = d.program.Types[ir.TypeID(item.header.Type)].(*ir.EventRootType)
			if !ok {
				return errors.New("expected event of type root first")
			}
			counter++
			continue
		}
		itemsInMemory[item.header.Address] = &item
	}

	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	for _, expr := range eventRoot.Expressions {
		parameterType := expr.Expression.Type
		parameterData := rootData[expr.Offset : expr.Offset+parameterType.GetByteSize()]
		err = d.uploader.WriteToken(jsontext.String(expr.Name))
		if err != nil {
			return err
		}
		err = d.parseDataToGoValue(itemsInMemory, parameterType, parameterData)
		if err != nil {
			return fmt.Errorf("error parsing data for field %s: %w", eventRoot.Name, err)
		}
	}

	err = d.uploader.WriteToken(jsontext.EndObject) // captures
	if err != nil {
		return err
	}
	err = d.uploader.WriteToken(jsontext.EndObject) // full object
	if err != nil {
		return err
	}

	return nil

}

func (d *Decoder) parseDataToGoValue(dataItemsInMemory map[uint64]*dataItem, irType ir.Type, data []byte) error {
	switch v := irType.(type) {
	case *ir.BaseType:
		return d.parseDataToGoBaseTypeValue(v, data)
	case *ir.PointerType:
		if len(data) < int(v.GetByteSize()) {
			return errors.New("passed data not long enough for pointer")
		}
		addr := binary.NativeEndian.Uint64(data)
		pointedValue, ok := dataItemsInMemory[addr]
		if !ok {
			return errors.New("pointer not found in address pass map")
		}
		err := d.parseDataToGoValue(dataItemsInMemory, d.program.Types[ir.TypeID(pointedValue.header.Type)], pointedValue.data)
		if err != nil {
			return fmt.Errorf("could not get pointed-at value: %s", err)
		}
	case *ir.StructureType:
		err := d.uploader.WriteToken(jsontext.BeginObject)
		if err != nil {
			return err
		}
		structure := irType.(*ir.StructureType)
		for _, field := range structure.Fields {
			err = d.uploader.WriteToken(jsontext.String(field.Name))
			if err != nil {
				return err
			}
			err = d.parseDataToGoValue(dataItemsInMemory, field.Type, data[field.Offset:field.Offset+uint32(field.Type.GetByteSize())])
			if err != nil {
				return err
			}
		}
		err = d.uploader.WriteToken(jsontext.EndObject)
		if err != nil {
			return err
		}
	case *ir.ArrayType:
		return errorUnimplemented
	case *ir.GoEmptyInterfaceType:
		return errorUnimplemented
	case *ir.GoInterfaceType:
		return errorUnimplemented
	case *ir.GoSliceHeaderType:
		sliceHeader := irType.(*ir.GoSliceHeaderType)
		var array, length, cap uint64
		for _, field := range sliceHeader.Fields {
			if field.Name == "array" {
				if uint32(field.Type.GetByteSize()) != 8 || len(data) < int(field.Offset+field.Type.GetByteSize()) {
					return errors.New("malformed string field 'str'")
				}
				array = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else if field.Name == "len" {
				if field.Type.GetByteSize() != 8 {
					return errors.New("malformed string field 'len'")
				}
				length = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else if field.Name == "cap" {
				if field.Type.GetByteSize() != 8 {
					return errors.New("malformed string field 'len'")
				}
				cap = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else {
				fmt.Println(field.Name)
			}
		}
		fmt.Println(array, length, cap)
		return errorUnimplemented
	case *ir.GoSliceDataType:
		return errorUnimplemented
	case *ir.GoChannelType:
		return errorUnimplemented
	case *ir.GoStringHeaderType:
		stringHeader := irType.(*ir.GoStringHeaderType)
		var address, length uint64
		for _, field := range stringHeader.Fields {
			if field.Name == "str" {
				if uint32(field.Type.GetByteSize()) != 8 || len(data) < int(field.Offset+field.Type.GetByteSize()) {
					return errors.New("malformed string field 'str'")
				}
				address = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			} else if field.Name == "len" {
				if field.Type.GetByteSize() != 8 {
					return errors.New("malformed string field 'len'")
				}
				length = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			}
		}

		// FIXME: Once string captures actually work it may be correct to call parseDataToGoValue on the type from dataItemsInMemory
		// and have the reading of GoStringDataType handled in this switch instead
		stringValue, ok := dataItemsInMemory[address]
		if !ok {
			return fmt.Errorf("string content not present in data items")
		}
		if len(stringValue.data) < int(length) {
			return errors.New("string content not long enough for known length")
		}
		err := d.uploader.WriteToken(jsontext.String(string(stringValue.data[:length])))
		if err != nil {
			return err
		}
		return nil
	case *ir.GoStringDataType:
		return errorUnimplemented
	case *ir.GoMapType:
		return errorUnimplemented
	case *ir.GoHMapHeaderType:
		return errorUnimplemented
	case *ir.GoHMapBucketType:
		return errorUnimplemented
	case *ir.GoSwissMapHeaderType:
		return errorUnimplemented
	case *ir.GoSwissMapGroupsType:
		return errorUnimplemented
	case *ir.EventRootType:
		return errorUnimplemented
	default:
		return errors.New("invalid type")
	}
	return nil
}

func (d *Decoder) parseDataToGoBaseTypeValue(irType *ir.BaseType, data []byte) error {
	kind, ok := irType.GetGoKind()
	if !ok {
		return errors.New("No go kind")
	}
	switch kind {
	case reflect.Bool:
		if len(data) < 1 {
			return errors.New("passed data not long enough for bool")
		}
		return d.uploader.WriteToken(jsontext.Bool(data[0] == 1))
	case reflect.Int:
		if len(data) < 8 {
			return errors.New("passed data not long enough for int")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Int8:
		if len(data) < 1 {
			return errors.New("passed data not long enough for int8")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(int8(data[0]))))
	case reflect.Int16:
		if len(data) < 2 {
			return errors.New("passed data not long enough for int16")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(int16(binary.NativeEndian.Uint16(data)))))
	case reflect.Int32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for int32")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(int32(binary.NativeEndian.Uint32(data)))))
	case reflect.Int64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for int64")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Uint:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Uint8:
		if len(data) != 1 {
			return errors.New("passed data not long enough for uint8")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(uint8(data[0]))))
	case reflect.Uint16:
		if len(data) != 2 {
			return errors.New("passed data not long enough for uint16")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(uint16(binary.NativeEndian.Uint16(data)))))
	case reflect.Uint32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for uint32")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(uint32(binary.NativeEndian.Uint32(data)))))
	case reflect.Uint64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint64")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Uintptr:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uintptr")
		}
		return d.uploader.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Float32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for float32")
		}
		return d.uploader.WriteToken(jsontext.Float(float64(math.Float32frombits(binary.NativeEndian.Uint32(data)))))
	case reflect.Float64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for float64")
		}
		return d.uploader.WriteToken(jsontext.Float(float64(math.Float64frombits(binary.NativeEndian.Uint64(data)))))
	case reflect.Complex64:
		// if len(data) != 8 {
		// 	return  errors.New("passed data not long enough for complex64")
		// }
		// real := math.Float64frombits(binary.NativeEndian.Uint32(data[0:4]))
		// imag := math.Float32frombits(binary.NativeEndian.Uint32(data[4:8]))
		// return d.uploader.WriteValue(jsontext.Value(strconv.FormatComplex(complex(real, imag), 'f', -1, 64)))
	case reflect.Complex128:
		if len(data) != 16 {
			return errors.New("passed data not long enough for complex128")
		}
		real := math.Float64frombits(binary.NativeEndian.Uint64(data[0:8]))
		imag := math.Float64frombits(binary.NativeEndian.Uint64(data[8:16]))
		err := d.uploader.WriteToken(jsontext.Float(float64(real)))
		if err != nil {
			return err
		}
		return d.uploader.WriteToken(jsontext.Float(float64(imag)))
	case reflect.Array:
		return errors.New("arrays are not base types")
	case reflect.Chan:
		return errors.New("channels are not base types")
	case reflect.Func:
		return errors.New("funcs are not base types")
	case reflect.Interface:
		return errors.New("interfaces are not base types")
	case reflect.Map:
		return errors.New("maps are not base types")
	case reflect.Ptr:
		return errors.New("ptrs are not base types")
	case reflect.Slice:
		return errors.New("slices are not base types")
	case reflect.String:
		return errors.New("strings are not base types")
	case reflect.Struct:
		return errors.New("structs are not base types")
	case reflect.UnsafePointer:
		return errors.New("unsafe pointers are not base types")
	default:
		return errors.New("invalid base type")
	}
	return nil
}
