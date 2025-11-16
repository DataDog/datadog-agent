// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// encodeSwissMapTables traverses the table pointer slice and encodes the data items for each table.
func (s *goSwissMapHeaderType) encodeSwissMapTables(
	c *encodingContext,
	enc *jsontext.Encoder,
	tablePtrSliceDataItem []byte,
) (totalElementsEncoded int, err error) {
	addrs := []uint64{}
	numPtrs := uint32(len(tablePtrSliceDataItem) / 8)
	for i := range numPtrs {
		startIdx := i * 8
		endIdx := startIdx + 8
		if endIdx > uint32(len(tablePtrSliceDataItem)) {
			return totalElementsEncoded, fmt.Errorf(
				"table pointer %d extends beyond data bounds: need %d bytes, have %d",
				i, endIdx, len(tablePtrSliceDataItem),
			)
		}
		addrs = append(addrs, binary.NativeEndian.Uint64(tablePtrSliceDataItem[startIdx:endIdx]))
	}
	// Deduplicate addrs by sorting and then removing duplicates.
	// Go swiss maps may have multiple table pointers for the same group.
	slices.Sort(addrs)
	addrs = slices.Compact(addrs)
	for _, addr := range addrs {
		tableDataItem, ok := c.getPtr(addr, s.tableTypeID)
		if !ok {
			continue
		}
		tableData, ok := tableDataItem.Data()
		if !ok {
			// Should we tell the user about this fault?
			continue
		}
		groupsData := tableData[s.groupFieldOffset : s.groupFieldOffset+uint32(s.groupFieldSize)]
		groupsPtrData := groupsData[s.dataFieldOffset : s.dataFieldOffset+uint32(s.dataFieldSize)]
		groupsPtr := binary.NativeEndian.Uint64(groupsPtrData)
		groupsArrayDataItem, ok := c.getPtr(groupsPtr, s.groupSliceTypeID)
		if !ok {
			if log.ShouldLog(log.TraceLvl) {
				groupsPtr := groupsPtr
				log.Tracef("group data item not found for addr %x", groupsPtr)
			}
			continue
		}
		groupsArrayData, ok := groupsArrayDataItem.Data()
		if !ok {
			// Should we tell the user about this fault?
			continue
		}
		numberOfGroups := uint32(len(groupsArrayData)) / s.elementTypeSize
		for i := range numberOfGroups {
			singleGroupData := groupsArrayData[s.elementTypeSize*i : s.elementTypeSize*(i+1)]
			elementsEncoded, err := s.encodeSwissMapGroup(c, enc, singleGroupData)
			if err != nil {
				return totalElementsEncoded, err
			}
			totalElementsEncoded += elementsEncoded
		}
	}
	return totalElementsEncoded, nil
}

func (s *goSwissMapHeaderType) encodeSwissMapGroup(
	c *encodingContext,
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
		if err := encodeValue(
			c, enc, s.keyTypeID, keyData, s.keyTypeName,
		); err != nil {
			return valuesEncoded, err
		}
		if err := encodeValue(
			c, enc, s.valueTypeID, valueData, s.valueTypeName,
		); err != nil {
			return valuesEncoded, err
		}
		if err := writeTokens(enc, jsontext.EndArray); err != nil {
			return valuesEncoded, err
		}
		valuesEncoded++
	}
	return valuesEncoded, nil
}

// formatSwissMapTables traverses the table pointer slice and formats the data
// items for each table.
func (s *goSwissMapHeaderType) formatSwissMapTables(
	c *encodingContext,
	buf *bytes.Buffer,
	tablePtrSliceData []byte,
	limits *formatLimits,
) (totalElementsFormatted int, err error) {
	addrs := []uint64{}
	numPtrs := uint32(len(tablePtrSliceData) / 8)
	for i := range numPtrs {
		startIdx := i * 8
		endIdx := startIdx + 8
		if endIdx > uint32(len(tablePtrSliceData)) {
			return totalElementsFormatted, fmt.Errorf(
				"table pointer %d extends beyond data bounds: need %d bytes, have %d",
				i, endIdx, len(tablePtrSliceData),
			)
		}
		addrs = append(addrs, binary.NativeEndian.Uint64(tablePtrSliceData[startIdx:endIdx]))
	}
	// Deduplicate addrs by sorting and then removing duplicates.
	// Go swiss maps may have multiple table pointers for the same group.
	slices.Sort(addrs)
	addrs = slices.Compact(addrs)
	maxItems := limits.maxCollectionItems
	for _, addr := range addrs {
		if totalElementsFormatted >= maxItems {
			break
		}
		if addr == 0 {
			continue
		}
		tableDataItem, ok := c.getPtr(addr, s.tableTypeID)
		if !ok {
			continue
		}
		tableData, ok := tableDataItem.Data()
		if !ok {
			continue
		}
		groupsData := tableData[s.groupFieldOffset : s.groupFieldOffset+uint32(s.groupFieldSize)]
		groupsPtrData := groupsData[s.dataFieldOffset : s.dataFieldOffset+uint32(s.dataFieldSize)]
		groupsPtr := binary.NativeEndian.Uint64(groupsPtrData)
		if groupsPtr == 0 {
			continue
		}
		groupsArrayDataItem, ok := c.getPtr(groupsPtr, s.groupSliceTypeID)
		if !ok {
			continue
		}
		groupsArrayData, ok := groupsArrayDataItem.Data()
		if !ok {
			continue
		}
		numberOfGroups := uint32(len(groupsArrayData)) / s.elementTypeSize
		for i := range numberOfGroups {
			if totalElementsFormatted >= maxItems {
				break
			}
			// Update limits to reflect remaining items we can format.
			remainingItems := maxItems - totalElementsFormatted
			originalMaxItems := limits.maxCollectionItems
			limits.maxCollectionItems = remainingItems
			singleGroupData := groupsArrayData[s.elementTypeSize*i : s.elementTypeSize*(i+1)]
			elementsFormatted, err := s.formatSwissMapGroup(
				c, buf, singleGroupData, totalElementsFormatted > 0, limits,
			)
			limits.maxCollectionItems = originalMaxItems
			if err != nil {
				return totalElementsFormatted, err
			}
			totalElementsFormatted += elementsFormatted
			if totalElementsFormatted >= maxItems {
				break
			}
		}
	}
	return totalElementsFormatted, nil
}

// formatSwissMapGroup formats entries from a single swiss map group.
// It respects the remaining limit in maxCollectionItems and returns the number
// of items formatted.
func (s *goSwissMapHeaderType) formatSwissMapGroup(
	c *encodingContext,
	buf *bytes.Buffer,
	groupData []byte,
	needComma bool,
	limits *formatLimits,
) (formattedItems int, err error) {
	slotsData := groupData[s.slotsOffset : s.slotsOffset+s.slotsSize]
	controlWord := binary.LittleEndian.Uint64(
		groupData[s.ctrlOffset : s.ctrlOffset+uint32(s.ctrlSize)],
	)
	maxItems := limits.maxCollectionItems
	for i := range 8 {
		if formattedItems >= maxItems {
			break
		}
		if controlWord&(1<<(7+(8*i))) != 0 {
			// Slot is empty or deleted.
			continue
		}
		if needComma || formattedItems > 0 {
			if !writeBoundedString(buf, limits, formatCommaSpace) {
				return formattedItems, nil
			}
		}
		formattedItems++
		entryData := slotsData[uint32(i)*s.slotsArrayEntrySize : uint32(i+1)*s.slotsArrayEntrySize]
		keyData := entryData[s.keyFieldOffset : s.keyFieldOffset+s.keyTypeSize]
		valueData := entryData[s.valueFieldOffset : s.valueFieldOffset+s.valueTypeSize]

		// Format key.
		keyType, ok := c.getType(s.keyTypeID)
		if !ok {
			return formattedItems, fmt.Errorf("key type not found: %d", s.keyTypeID)
		}
		keyBeforeLen := buf.Len()
		if err := keyType.formatValueFields(c, buf, keyData, limits); err != nil {
			return formattedItems, err
		}
		keyWritten := buf.Len() - keyBeforeLen
		limits.consume(keyWritten)

		if !writeBoundedString(buf, limits, formatColonSpace) {
			return formattedItems, nil
		}

		// Format value.
		valueType, ok := c.getType(s.valueTypeID)
		if !ok {
			return formattedItems, fmt.Errorf("value type not found: %d", s.valueTypeID)
		}
		valueBeforeLen := buf.Len()
		if err := valueType.formatValueFields(c, buf, valueData, limits); err != nil {
			return formattedItems, err
		}
		valueWritten := buf.Len() - valueBeforeLen
		limits.consume(valueWritten)
	}
	return formattedItems, nil
}
