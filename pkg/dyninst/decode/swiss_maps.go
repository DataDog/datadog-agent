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

	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
