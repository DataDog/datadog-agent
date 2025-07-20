// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"fmt"
	"slices"

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
				notCapturedReason,
				notCapturedReasonPruned,
				jsontext.EndObject,
			); err != nil {
				return err
			}
			continue
		}
		err = ad.decoder.encodeValue(enc,
			currentlyEncoding,
			parameterType.GetID(),
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
	currentlyEncoding map[typeAndAddr]struct{},
	typeID ir.TypeID,
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
	decoderType, ok := d.decoderTypes[typeID]
	if !ok {
		return fmt.Errorf("no decoder type found for type %s", decoderType.irType().GetName())
	}
	if err := decoderType.encodeValueFields(
		&decoderContext{
			decoder:           d,
			enc:               enc,
			currentlyEncoding: currentlyEncoding,
		},
		data,
	); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}
	return nil
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
	ctx *decoderContext,
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
	tableType := s.tableType
	for _, addr := range addrs {
		tableDataItem, ok := ctx.decoder.dataItemReferences[typeAndAddr{
			irType: uint32(tableType.Pointee.GetID()),
			addr:   addr,
		}]
		if !ok {
			return fmt.Errorf("table data item not found for addr %x", addr)
		}
		tableStructType := s.tableStructType
		groupField, err := getFieldByName(tableStructType.Fields, "groups")
		if err != nil {
			return err
		}
		groupData := tableDataItem.Data()[groupField.Offset : groupField.Offset+groupField.Type.GetByteSize()]
		dataField, err := getFieldByName(s.groupType.RawFields, "data")
		if err != nil {
			return err
		}
		groupAddress := groupData[dataField.Offset : dataField.Offset+dataField.Type.GetByteSize()]
		groupDataItem, ok := ctx.decoder.dataItemReferences[typeAndAddr{
			irType: uint32(s.groupType.GroupSliceType.GetID()),
			addr:   binary.NativeEndian.Uint64(groupAddress),
		}]
		if !ok {
			return fmt.Errorf("group data item not found for addr %x", binary.NativeEndian.Uint64(groupAddress))
		}
		elementType := s.groupType.GroupSliceType.Element
		numberOfGroups := groupDataItem.Header().Length / elementType.GetByteSize()
		for i := range numberOfGroups {
			singleGroupData := groupDataItem.Data()[s.groupType.GroupSliceType.Element.GetByteSize()*i : s.groupType.GroupSliceType.Element.GetByteSize()*(i+1)]
			err := d.encodeSwissMapGroup(ctx.enc, ctx.currentlyEncoding, s, singleGroupData, ir.TypeID(s.keyTypeID), ir.TypeID(s.valueTypeID))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Decoder) encodeSwissMapGroup(
	enc *jsontext.Encoder,
	currentlyEncoding map[typeAndAddr]struct{},
	s *goSwissMapHeaderType,
	groupData []byte,
	keyTypeID ir.TypeID,
	valueTypeID ir.TypeID,
) error {
	keyType, ok := d.program.Types[keyTypeID]
	if !ok {
		return fmt.Errorf("key type not found for key type ID %d", keyTypeID)
	}
	valueType, ok := d.program.Types[valueTypeID]
	if !ok {
		return fmt.Errorf("value type not found for value type ID %d", valueTypeID)
	}
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
	entrySize := keyType.GetByteSize() + valueType.GetByteSize()
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
		if uint32(len(entryData)) < keyType.GetByteSize()+valueType.GetByteSize() {
			return fmt.Errorf("entry %d data insufficient for key+value: need %d bytes, have %d", i, keyType.GetByteSize()+valueType.GetByteSize(), len(entryData))
		}
		keyData := entryData[0:keyType.GetByteSize()]
		valueData := entryData[keyType.GetByteSize() : keyType.GetByteSize()+valueType.GetByteSize()]
		if err := writeTokens(enc,
			jsontext.BeginArray,
		); err != nil {
			return err
		}
		err := d.encodeValue(enc, currentlyEncoding,
			keyTypeID, keyData, keyType.GetName(),
		)
		if err != nil {
			return err
		}
		err = d.encodeValue(enc, currentlyEncoding,
			valueTypeID, valueData, valueType.GetName(),
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
