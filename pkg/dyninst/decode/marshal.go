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
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	if err = writeTokens(enc, jsontext.BeginObject); err != nil {
		return err
	}

	presenceBitSet := ad.rootData[:ad.rootType.PresenceBitsetSize]
	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	for i, expr := range ad.rootType.Expressions {
		parameterType := expr.Expression.Type
		parameterData := ad.rootData[expr.Offset : expr.Offset+parameterType.GetByteSize()]

		if err = writeTokens(enc, jsontext.String(expr.Name)); err != nil {
			return err
		}
		if !expressionIsPresent(presenceBitSet, i) && parameterType.GetByteSize() != 0 {
			// Set not capture reason
			if err = writeTokens(enc,
				jsontext.BeginObject,
				jsontext.String("type"),
				jsontext.String(parameterType.GetName()),
				notCapturedReason,
				notCapturedReasonUnavailable,
				jsontext.EndObject,
			); err != nil {
				return err
			}
			continue
		}
		err = ad.decoder.encodeValue(enc,
			parameterType.GetID(),
			parameterData,
			parameterType.GetName(),
		)
		if err != nil {
			return fmt.Errorf("error parsing data for field %s: %w", ad.rootType.Name, err)
		}
	}
	if err = writeTokens(enc, jsontext.EndObject); err != nil {
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
	if err := decoderType.encodeValueFields(d, enc, data); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}
	return nil
}

func writeTokens(enc *jsontext.Encoder, tokens ...jsontext.Token) error {
	for i := range tokens {
		err := enc.WriteToken(tokens[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// encodeSwissMapTables traverses the table pointer slice and encodes the data items for each table.
func (s *goSwissMapHeaderType) encodeSwissMapTables(
	d *Decoder,
	enc *jsontext.Encoder,
	tablePtrSliceDataItem output.DataItem,
) (totalElementsEncoded int, err error) {
	tablePointers := tablePtrSliceDataItem.Data()
	addrs := []uint64{}
	for i := range tablePtrSliceDataItem.Header().Length / 8 {
		startIdx := i * 8
		endIdx := startIdx + 8
		if endIdx > uint32(len(tablePointers)) {
			return totalElementsEncoded, fmt.Errorf("table pointer %d extends beyond data bounds: need %d bytes, have %d",
				i, endIdx, len(tablePointers))
		}
		addrs = append(addrs, binary.NativeEndian.Uint64(tablePointers[startIdx:endIdx]))
	}
	// Deduplicate addrs by sorting and then removing duplicates.
	// Go swiss maps may have multiple table pointers for the same group.
	slices.Sort(addrs)
	addrs = slices.Compact(addrs)
	for _, addr := range addrs {
		tableDataItem, ok := d.dataItems[typeAndAddr{
			irType: uint32(s.tableTypeID),
			addr:   addr,
		}]
		if !ok {
			continue
		}
		groupData := tableDataItem.Data()[s.groupFieldOffset : s.groupFieldOffset+uint32(s.groupFieldSize)]
		groupAddress := groupData[s.dataFieldOffset : s.dataFieldOffset+uint32(s.dataFieldSize)]
		groupDataItem, ok := d.dataItems[typeAndAddr{
			irType: uint32(s.groupSliceTypeID),
			addr:   binary.NativeEndian.Uint64(groupAddress),
		}]
		if !ok {
			log.Tracef("group data item not found for addr %x", binary.NativeEndian.Uint64(groupAddress))
			continue
		}
		numberOfGroups := groupDataItem.Header().Length / s.elementTypeSize
		for i := range numberOfGroups {
			singleGroupData := groupDataItem.Data()[s.elementTypeSize*i : s.elementTypeSize*(i+1)]
			elementsEncoded, err := s.encodeSwissMapGroup(d, enc, singleGroupData)
			if err != nil {
				return totalElementsEncoded, err
			}
			totalElementsEncoded += elementsEncoded
		}
	}
	return totalElementsEncoded, nil
}

func (s *goSwissMapHeaderType) encodeSwissMapGroup(
	d *Decoder,
	enc *jsontext.Encoder,
	groupData []byte,
) (valuesEncoded int, err error) {
	slotsData := groupData[s.slotsOffset : s.slotsOffset+s.slotsSize]
	controlWord := binary.LittleEndian.Uint64(groupData[s.ctrlOffset : s.ctrlOffset+uint32(s.ctrlSize)])
	for i := range 8 {
		if controlWord&(1<<(7+(8*i))) != 0 {
			// slot is empty or deleted
			continue
		}
		entryData := slotsData[uint32(i)*s.slotsArrayEntrySize : uint32(i+1)*s.slotsArrayEntrySize]
		keyData := entryData[s.keyFieldOffset : s.keyFieldOffset+s.keyTypeSize]
		valueData := entryData[s.valueFieldOffset : s.valueFieldOffset+s.valueTypeSize]

		if err := writeTokens(enc, jsontext.BeginArray); err != nil {
			return valuesEncoded, err
		}
		if err := d.encodeValue(enc, s.keyTypeID, keyData, s.keyTypeName); err != nil {
			return valuesEncoded, err
		}
		if err := d.encodeValue(enc, s.valueTypeID, valueData, s.valueTypeName); err != nil {
			return valuesEncoded, err
		}
		if err := writeTokens(enc, jsontext.EndArray); err != nil {
			return valuesEncoded, err
		}
		valuesEncoded++
	}
	return valuesEncoded, nil
}
