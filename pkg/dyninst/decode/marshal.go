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
	"slices"
	"strconv"

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

// In the root data item, before the expressions, there is a bitset
// which conveys if expression values are present in the data.
// The rootType.PresenceBitsetSize conveys the size of the bitset in
// bytes, and presence bits in the bitset correspond with index of
// the expression in the root ir.
func expressionIsPresent(bitset []byte, expressionIndex int) bool {
	idx, bit := expressionIndex/8, expressionIndex%8
	return idx < len(bitset) && bitset[idx]&(1<<byte(bit)) != 0
}

func (ad *argumentsData) MarshalJSONTo(enc *jsontext.Encoder) error {
	var err error
	currentlyEncoding := map[typeAndAddr]struct{}{}

	if err = writeTokens(enc,
		jsontext.BeginObject,
	); err != nil {
		return err
	}

	presenceBitSet := ad.rootData[:ad.rootType.PresenceBitsetSize]
	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	for i, expr := range ad.rootType.Expressions {
		parameterType := expr.Expression.Type
		parameterData := ad.rootData[expr.Offset : expr.Offset+parameterType.GetByteSize()]

		if err = writeTokens(enc,
			jsontext.String(expr.Name)); err != nil {
			return err
		}
		if !expressionIsPresent(presenceBitSet, i) && parameterType.GetByteSize() != 0 {
			// Set not capture reason
			if err = writeTokens(enc,
				jsontext.BeginObject,
				jsontext.String("type"),
				jsontext.String(parameterType.GetName()),
				jsontext.String("notCapturedReason"),
				jsontext.String("unavailable"),
				jsontext.EndObject,
			); err != nil {
				return err
			}
			continue
		}
		parameterDecoderType, err := ad.decoder.getDecoderType(parameterType)
		if err != nil {
			return err
		}
		err = ad.decoder.encodeValue(enc,
			ad.decoder.addressReferenceCount,
			currentlyEncoding,
			parameterDecoderType,
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
	decoderType decoderType,
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

	if err := decoderType.encodeValueFields(
		d,
		enc,
		dataItems,
		currentlyEncoding,
		data,
	); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}

	return nil
}

func encodeBaseTypeValue(enc *jsontext.Encoder, t *baseType, data []byte) error {
	kind, ok := t.GetGoKind()
	if !ok {
		return fmt.Errorf("no go kind for type %s (ID: %d)", t.GetName(), t.GetID())
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

// encodeSwissMapTables collects the data items for the swiss map tables.
// It traverses the table pointer slice and collects the data items for each table.
func (d *Decoder) encodeSwissMapTables(
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	s *goSwissMapHeaderType,
	tablePtrSliceDataItem output.DataItem,
) error {
	tablePointers := tablePtrSliceDataItem.Data()
	addrs := []uint64{}

	for i := range tablePtrSliceDataItem.Header().Length / 8 {
		startIdx := i * 8
		endIdx := startIdx + 8
		if endIdx > uint32(len(tablePointers)) {
			return fmt.Errorf("table pointer %d extends beyond data bounds: need %d bytes, have %d", i, endIdx, len(tablePointers))
		}
		addrs = append(addrs, binary.NativeEndian.Uint64(tablePointers[startIdx:endIdx]))
	}
	// Deduplicate addrs by sorting and then removing duplicates.
	// Go swiss maps may have multiple table pointers for the same group.
	slices.Sort(addrs)
	addrs = slices.Compact(addrs)
	tableType, ok := s.TablePtrSliceType.Element.(*ir.PointerType)
	if !ok {
		return fmt.Errorf("table ptr slice type element is not a pointer type: %s", s.TablePtrSliceType.Element.GetName())
	}
	for _, addr := range addrs {
		tableDataItem, ok := dataItems[typeAndAddr{
			irType: uint32(tableType.Pointee.GetID()),
			addr:   addr,
		}]
		if !ok {
			return fmt.Errorf("table data item not found for addr %x", addr)
		}

		tableStructType, ok := tableType.Pointee.(*ir.StructureType)
		if !ok {
			return fmt.Errorf("table type pointee is not a structure type: %s", tableType.Pointee.GetName())
		}

		var (
			groupField ir.Field
		)
		for field := range tableStructType.Fields() {
			if field.Name == "groups" {
				groupField = field
			}
		}

		groupData := tableDataItem.Data()[groupField.Offset : groupField.Offset+groupField.Type.GetByteSize()]
		groupType, ok := groupField.Type.(*ir.GoSwissMapGroupsType)
		if !ok {
			return fmt.Errorf("group field type is not a swiss map groups type: %s", groupField.Type.GetName())
		}
		var dataField ir.Field
		for field := range groupType.Fields() {
			if field.Name == "data" {
				dataField = field
			}
		}
		groupAddress := groupData[dataField.Offset : dataField.Offset+dataField.Type.GetByteSize()]
		groupDataItem, ok := dataItems[typeAndAddr{
			irType: uint32(groupType.GroupSliceType.GetID()),
			addr:   binary.NativeEndian.Uint64(groupAddress),
		}]
		if !ok {
			return fmt.Errorf("group data item not found for addr %x", binary.NativeEndian.Uint64(groupAddress))
		}

		elementType := groupType.GroupSliceType.Element
		numberOfGroups := groupDataItem.Header().Length / elementType.GetByteSize()
		for i := range numberOfGroups {
			singleGroupData := groupDataItem.Data()[groupType.GroupSliceType.Element.GetByteSize()*i : groupType.GroupSliceType.Element.GetByteSize()*(i+1)]
			err := d.collectSwissMapGroup(enc, dataItems, currentlyEncoding, s, singleGroupData, s.keyType, s.valueType)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Decoder) collectSwissMapGroup(
	enc *jsontext.Encoder,
	dataItems map[typeAndAddr]output.DataItem,
	currentlyEncoding map[typeAndAddr]struct{},
	s *goSwissMapHeaderType,
	groupData []byte,
	keyType decoderType,
	valueType decoderType,
) error {
	keySize := keyType.irType().GetByteSize()
	valueSize := valueType.irType().GetByteSize()
	var (
		controlWord uint64
		slotsData   []byte
	)
	for groupField := range s.GroupType.Fields() {
		fieldEnd := groupField.Offset + groupField.Type.GetByteSize()
		if fieldEnd > uint32(len(groupData)) {
			return fmt.Errorf("group field %s extends beyond data bounds: need %d bytes, have %d", groupField.Name, fieldEnd, len(groupData))
		}

		switch groupField.Name {
		case "slots":
			slotsData = groupData[groupField.Offset : groupField.Offset+groupField.Type.GetByteSize()]
		case "ctrl":
			controlWord = binary.LittleEndian.Uint64(groupData[groupField.Offset : groupField.Offset+groupField.Type.GetByteSize()])
		}
	}
	entrySize := keySize + valueSize
	for i := range 8 {
		if controlWord&(1<<(7+(8*i))) != 0 {
			// slot is empty or deleted
			continue
		}
		offset := entrySize * uint32(i)
		entryEnd := offset + entrySize
		if entryEnd > uint32(len(slotsData)) {
			return fmt.Errorf("entry %d extends beyond slots data bounds: need %d bytes, have %d", i, entryEnd, len(slotsData))
		}

		entryData := slotsData[offset:entryEnd]
		if uint32(len(entryData)) < keySize+valueSize {
			return fmt.Errorf("entry %d data insufficient for key+value: need %d bytes, have %d", i, keySize+valueSize, len(entryData))
		}

		keyData := entryData[0:keySize]
		valueData := entryData[keySize : keySize+valueSize]
		if err := writeTokens(enc,
			jsontext.BeginArray,
		); err != nil {
			return err
		}
		err := d.encodeValue(enc, dataItems, currentlyEncoding,
			keyType, keyData, keyType.irType().GetName(),
		)
		if err != nil {
			return err
		}
		err = d.encodeValue(enc, dataItems, currentlyEncoding,
			valueType, valueData, valueType.irType().GetName(),
		)
		if err != nil {
			return err
		}
		if err := writeTokens(enc, jsontext.EndArray); err != nil {
			return err
		}
	}
	return nil
}
