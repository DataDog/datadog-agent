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
	"strconv"

	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

type probeEvent struct {
	event *ir.Event
	probe *ir.Probe
}

// Decoder decodes the output of the BPF program into a JSON format.
// It is not guaranteed to be thread-safe.
type Decoder struct {
	program     *ir.Program
	stackHashes map[uint64][]uint64
	probeEvents map[ir.TypeID]probeEvent
}

var errorUnimplemented = errors.New("errorUnimplemented type")

// NewDecoder creates a new Decoder for the given program.
func NewDecoder(program *ir.Program) (*Decoder, error) {
	decoder := &Decoder{
		program:     program,
		stackHashes: make(map[uint64][]uint64),
		probeEvents: make(map[ir.TypeID]probeEvent),
	}
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			decoder.probeEvents[event.Type.ID] = probeEvent{
				event: event,
				probe: probe,
			}
		}
	}
	return decoder, nil
}

// typeAndAddr is a type and address pair. It is used to uniquely identify a data item in the data items map.
// Addresses may not be unique to a type, for example if an address is taken for the first field of a struct.
type typeAndAddr struct {
	irType uint32
	addr   uint64
}

// Decode decodes the given event into the given writer.
func (d *Decoder) Decode(event output.Event, out io.Writer) error {
	enc := jsontext.NewEncoder(out)
	err := enc.WriteToken(jsontext.BeginObject)
	if err != nil {
		return err
	}

	header, err := event.Header()
	if err != nil {
		return err
	}

	// bpf will only upload the pc's for a given hash once.
	// We cache the frames for each hash accordingly.
	var frames []uint64
	frames, ok := d.stackHashes[header.Stack_hash]
	if !ok {
		frames, err = event.StackPCs()
		if err != nil {
			return err
		}
		d.stackHashes[header.Stack_hash] = frames
	}

	err = enc.WriteToken(jsontext.String("stack_frames"))
	if err != nil {
		return err
	}
	err = enc.WriteToken(jsontext.BeginArray)
	if err != nil {
		return err
	}
	for i := range frames {
		err = enc.WriteToken(jsontext.Uint(frames[i]))
		if err != nil {
			return err
		}
	}
	err = enc.WriteToken(jsontext.EndArray)
	if err != nil {
		return err
	}

	err = enc.WriteToken(jsontext.String("captures"))
	if err != nil {
		return err
	}
	err = enc.WriteToken(jsontext.BeginObject)
	if err != nil {
		return err
	}
	addressReferenceCount := map[typeAndAddr]output.DataItem{}

	var (
		rootData []byte
		rootType *ir.EventRootType
	)
	for item, err := range event.DataItems() {
		if err != nil {
			return err
		}
		if rootType == nil {
			rootData = item.Data()
			var ok bool
			rootType, ok = d.program.Types[ir.TypeID(item.Header().Type)].(*ir.EventRootType)
			if !ok {
				return errors.New("expected event of type root first")
			}
			continue
		}

		// We need to keep track of the address reference count for each data item.
		// This is used to avoid infinite recursion when encoding pointers.
		// We use a map to store the address reference count for each data item.
		// The key is a type and address pair.
		// The value is a data item with a counter of how many times it has been referenced.
		// If the counter is greater than 1, we know that the data item is a pointer to another data item.
		// We can then encode the pointer as a string and not as an object.
		addressReferenceCount[typeAndAddr{
			irType: uint32(item.Header().Type),
			addr:   item.Header().Address,
		}] = item
	}

	p, ok := d.probeEvents[rootType.ID]
	if !ok {
		return errors.New("no probe event found for root type")
	}
	err = enc.WriteToken(jsontext.String("probe_id"))
	if err != nil {
		return err
	}
	err = enc.WriteToken(jsontext.String(p.probe.ID))
	if err != nil {
		return err
	}

	// This map is used to avoid infinite recursion when encoding pointer values.
	// When an address has already been encountered it is added to this map, and
	// removed when the recursive function call chain returns. This way unique
	// branches of an object graph contain the value of the object, and not a
	// pointer to the object.
	currentlyEncoding := map[typeAndAddr]struct{}{}

	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	for _, expr := range rootType.Expressions {
		parameterType := expr.Expression.Type
		parameterData := rootData[expr.Offset : expr.Offset+parameterType.GetByteSize()]
		err = enc.WriteToken(jsontext.String(expr.Name))
		if err != nil {
			return err
		}
		err = d.encodeValue(enc, addressReferenceCount, currentlyEncoding, parameterType, parameterData)
		if err != nil {
			return fmt.Errorf("error parsing data for field %s: %w", rootType.Name, err)
		}
	}

	err = enc.WriteToken(jsontext.EndObject) // captures
	if err != nil {
		return err
	}
	err = enc.WriteToken(jsontext.EndObject) // full object
	if err != nil {
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
) error {
	switch v := irType.(type) {
	case *ir.BaseType:
		return d.encodeBaseTypeValue(enc, v, data)
	case *ir.PointerType:
		if len(data) < int(v.GetByteSize()) {
			return errors.New("passed data not long enough for pointer")
		}
		addr := binary.NativeEndian.Uint64(data)
		if addr == 0 { // nil pointers are encoded as 0.
			err := enc.WriteToken(jsontext.String("nil"))
			if err != nil {
				return err
			}
			return nil
		}
		key := typeAndAddr{
			irType: uint32(v.Pointee.GetID()),
			addr:   addr,
		}
		pointedValue, ok := dataItems[key]
		if !ok {
			return errors.New("pointer not found in address pass map")
		}
		header := pointedValue.Header()
		if pointedValue.Header().Address == 0 {
			return enc.WriteToken(jsontext.String(fmt.Sprintf("0x%x", header.Address)))
		}
		err := enc.WriteToken(jsontext.BeginObject)
		if err != nil {
			return err
		}
		err = enc.WriteToken(jsontext.String("Address"))
		if err != nil {
			return err
		}
		err = enc.WriteToken(jsontext.String(fmt.Sprintf("0x%x", header.Address)))
		if err != nil {
			return err
		}

		if _, ok := currentlyEncoding[key]; !ok {
			currentlyEncoding[key] = struct{}{}
			defer delete(currentlyEncoding, key)

			err = enc.WriteToken(jsontext.String("Value"))
			if err != nil {
				return err
			}
			err = d.encodeValue(
				enc,
				dataItems,
				currentlyEncoding,
				d.program.Types[ir.TypeID(header.Type)],
				pointedValue.Data(),
			)
			if err != nil {
				return fmt.Errorf("could not get pointed-at value: %s", err)
			}
		}
		err = enc.WriteToken(jsontext.EndObject)
		if err != nil {
			return err
		}
	case *ir.StructureType:
		err := enc.WriteToken(jsontext.BeginObject)
		if err != nil {
			return err
		}
		structure := irType.(*ir.StructureType)
		for _, field := range structure.Fields {
			err = enc.WriteToken(jsontext.String(field.Name))
			if err != nil {
				return err
			}
			err = d.encodeValue(
				enc,
				dataItems,
				currentlyEncoding,
				field.Type,
				data[field.Offset:field.Offset+field.Type.GetByteSize()],
			)
			if err != nil {
				return err
			}
		}
		err = enc.WriteToken(jsontext.EndObject)
		if err != nil {
			return err
		}
	case *ir.ArrayType:
		err := enc.WriteToken(jsontext.BeginArray)
		if err != nil {
			return err
		}

		elementType := v.Element
		elementSize := int(elementType.GetByteSize())
		numElements := int(v.Count)
		for i := range numElements {
			elementData := data[i*elementSize : (i+1)*elementSize]
			err := d.encodeValue(enc, dataItems, currentlyEncoding, v.Element, elementData)
			if err != nil {
				return err
			}
		}
		err = enc.WriteToken(jsontext.EndArray)
		if err != nil {
			return err
		}
		return nil
	case *ir.GoEmptyInterfaceType:
		return errorUnimplemented
	case *ir.GoInterfaceType:
		return errorUnimplemented
	case *ir.GoSliceHeaderType:
		if len(data) < int(v.ByteSize) {
			return errors.New("passed data not long enough for slice header")
		}
		address := binary.NativeEndian.Uint64(data[0:8])
		length := binary.NativeEndian.Uint64(data[8:16])
		if length == 0 {
			err := enc.WriteToken(jsontext.BeginArray)
			if err != nil {
				return err
			}
			err = enc.WriteToken(jsontext.EndArray)
			if err != nil {
				return err
			}
			return nil
		}
		elementType := v.Data.ID
		elementSize := int(v.Data.Element.GetByteSize())
		sliceDataItem, ok := dataItems[typeAndAddr{
			addr:   address,
			irType: uint32(elementType),
		}]
		if !ok {
			return errors.New("slice data item not found in data items")
		}
		err := enc.WriteToken(jsontext.BeginArray)
		if err != nil {
			return err
		}
		sliceLength := int(sliceDataItem.Header().Length) / elementSize
		sliceData := sliceDataItem.Data()
		for i := range int(sliceLength) {
			elementData := sliceData[i*elementSize : (i+1)*elementSize]
			err := d.encodeValue(enc, dataItems, currentlyEncoding, v.Data.Element, elementData)
			if err != nil {
				return err
			}
		}
		err = enc.WriteToken(jsontext.EndArray)
		if err != nil {
			return err
		}
		return nil
	case *ir.GoChannelType:
		return errorUnimplemented
	case *ir.GoStringHeaderType:
		var address uint64
		for _, field := range v.Fields {
			if field.Name == "str" {
				if uint32(field.Type.GetByteSize()) != 8 || len(data) < int(field.Offset+field.Type.GetByteSize()) {
					return errors.New("malformed string field 'str'")
				}
				address = binary.NativeEndian.Uint64(data[field.Offset : field.Offset+uint32(field.Type.GetByteSize())])
			}
		}
		stringValue, ok := dataItems[typeAndAddr{
			irType: uint32(v.Data.GetID()),
			addr:   address,
		}]
		if !ok {
			return fmt.Errorf("string content not present in data items")
		}
		length := stringValue.Header().Length
		if len(stringValue.Data()) < int(length) {
			return errors.New("string content not long enough for known length")
		}
		err := enc.WriteToken(jsontext.String(string(stringValue.Data()[:length])))
		if err != nil {
			return err
		}
		return nil
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

func (d *Decoder) encodeBaseTypeValue(enc *jsontext.Encoder, irType *ir.BaseType, data []byte) error {
	kind, ok := irType.GetGoKind()
	if !ok {
		return errors.New("No go kind")
	}
	switch kind {
	case reflect.Bool:
		if len(data) < 1 {
			return errors.New("passed data not long enough for bool")
		}
		return enc.WriteToken(jsontext.Bool(data[0] == 1))
	case reflect.Int:
		if len(data) < 8 {
			return errors.New("passed data not long enough for int")
		}
		return enc.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Int8:
		if len(data) < 1 {
			return errors.New("passed data not long enough for int8")
		}
		return enc.WriteToken(jsontext.Int(int64(int8(data[0]))))
	case reflect.Int16:
		if len(data) < 2 {
			return errors.New("passed data not long enough for int16")
		}
		return enc.WriteToken(jsontext.Int(int64(int16(binary.NativeEndian.Uint16(data)))))
	case reflect.Int32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for int32")
		}
		return enc.WriteToken(jsontext.Int(int64(int32(binary.NativeEndian.Uint32(data)))))
	case reflect.Int64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for int64")
		}
		return enc.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Uint:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint")
		}
		return enc.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Uint8:
		if len(data) != 1 {
			return errors.New("passed data not long enough for uint8")
		}
		return enc.WriteToken(jsontext.Int(int64(uint8(data[0]))))
	case reflect.Uint16:
		if len(data) != 2 {
			return errors.New("passed data not long enough for uint16")
		}
		return enc.WriteToken(jsontext.Int(int64(uint16(binary.NativeEndian.Uint16(data)))))
	case reflect.Uint32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for uint32")
		}
		return enc.WriteToken(jsontext.Int(int64(uint32(binary.NativeEndian.Uint32(data)))))
	case reflect.Uint64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uint64")
		}
		return enc.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Uintptr:
		if len(data) != 8 {
			return errors.New("passed data not long enough for uintptr")
		}
		return enc.WriteToken(jsontext.Int(int64(binary.NativeEndian.Uint64(data))))
	case reflect.Float32:
		if len(data) != 4 {
			return errors.New("passed data not long enough for float32")
		}
		return enc.WriteToken(jsontext.Float(float64(math.Float32frombits(binary.NativeEndian.Uint32(data)))))
	case reflect.Float64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for float64")
		}
		return enc.WriteToken(jsontext.Float(math.Float64frombits(binary.NativeEndian.Uint64(data))))
	case reflect.Complex64:
		if len(data) != 8 {
			return errors.New("passed data not long enough for complex64")
		}
		realBits := math.Float32frombits(binary.NativeEndian.Uint32(data[0:4]))
		imagBits := math.Float32frombits(binary.NativeEndian.Uint32(data[4:8]))
		return enc.WriteToken(jsontext.String(strconv.FormatComplex(complex(float64(realBits), float64(imagBits)), 'f', -1, 64)))
	case reflect.Complex128:
		if len(data) != 16 {
			return errors.New("passed data not long enough for complex128")
		}
		realBits := math.Float64frombits(binary.NativeEndian.Uint64(data[0:8]))
		imagBits := math.Float64frombits(binary.NativeEndian.Uint64(data[8:16]))
		err := enc.WriteToken(jsontext.Float(float64(realBits)))
		if err != nil {
			return err
		}
		err = enc.WriteToken(jsontext.Float(float64(imagBits)))
		if err != nil {
			return err
		}
		return nil
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
}
