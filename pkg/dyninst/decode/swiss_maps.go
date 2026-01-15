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

const (
	ptrSizeBytes = 8 // 64-bit pointer size
)

// swissMapGroupCallback processes a single swiss map group.
// Returns the number of items processed and any error.
type swissMapGroupCallback func(groupData []byte) (itemsProcessed int, err error)

// parseTablePointers extracts and deduplicates table pointers from slice data.
func parseTablePointers(data []byte) ([]uint64, error) {
	if len(data)%ptrSizeBytes != 0 {
		return nil, fmt.Errorf(
			"table pointer slice length %d not multiple of %d",
			len(data), ptrSizeBytes,
		)
	}
	numPtrs := len(data) / ptrSizeBytes
	addrs := make([]uint64, 0, numPtrs)
	for i := 0; i < numPtrs; i++ {
		startIdx := i * ptrSizeBytes
		endIdx := startIdx + ptrSizeBytes
		addrs = append(addrs, binary.NativeEndian.Uint64(data[startIdx:endIdx]))
	}
	// Deduplicate addrs by sorting and then removing duplicates.
	// Go swiss maps may have multiple table pointers for the same group.
	slices.Sort(addrs)
	return slices.Compact(addrs), nil
}

// extractGroupsArray extracts the groups array data from a table.
func extractGroupsArray(
	c *encodingContext,
	tableData []byte,
	s *goSwissMapHeaderType,
) ([]byte, error) {
	groupsData := tableData[s.groupFieldOffset : s.groupFieldOffset+uint32(s.groupFieldSize)]
	groupsPtrData := groupsData[s.dataFieldOffset : s.dataFieldOffset+uint32(s.dataFieldSize)]
	groupsPtr := binary.NativeEndian.Uint64(groupsPtrData)
	if groupsPtr == 0 {
		return nil, nil // Empty groups array
	}
	groupsArrayDataItem, ok := c.getPtr(groupsPtr, s.groupSliceTypeID)
	if !ok {
		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("group data item not found for addr %x", groupsPtr)
		}
		return nil, fmt.Errorf("group data item not found for addr %x", groupsPtr)
	}
	groupsArrayData, ok := groupsArrayDataItem.Data()
	if !ok {
		return nil, fmt.Errorf("failed to read group array data at %x", groupsPtr)
	}
	return groupsArrayData, nil
}

// iterateSwissMapTables traverses the table pointer slice and calls the callback
// for each group in each table.
func iterateSwissMapTables(
	s *goSwissMapHeaderType,
	c *encodingContext,
	tablePtrSliceData []byte,
	maxItems int,
	callback swissMapGroupCallback,
) (totalItemsProcessed int, err error) {
	addrs, err := parseTablePointers(tablePtrSliceData)
	if err != nil {
		return 0, err
	}

	for _, addr := range addrs {
		if shouldStop(maxItems, totalItemsProcessed) {
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

		groupsArrayData, err := extractGroupsArray(c, tableData, s)
		if err != nil {
			// Log but continue - other tables might be valid
			if log.ShouldLog(log.TraceLvl) {
				log.Tracef("failed to extract groups array: %v", err)
			}
			continue
		}
		if groupsArrayData == nil {
			continue
		}

		numberOfGroups := uint32(len(groupsArrayData)) / s.elementTypeSize
		for i := range numberOfGroups {
			if shouldStop(maxItems, totalItemsProcessed) {
				break
			}
			singleGroupData := groupsArrayData[s.elementTypeSize*i : s.elementTypeSize*(i+1)]
			itemsProcessed, err := callback(singleGroupData)
			if err != nil {
				return totalItemsProcessed, err
			}
			totalItemsProcessed += itemsProcessed
		}
	}
	return totalItemsProcessed, nil
}

// encodeSwissMapTables traverses the table pointer slice and encodes the data items for each table.
func (s *goSwissMapHeaderType) encodeSwissMapTables(
	c *encodingContext,
	enc *jsontext.Encoder,
	tablePtrSliceDataItem []byte,
) (totalElementsEncoded int, err error) {
	callback := func(groupData []byte) (int, error) {
		return s.encodeSwissMapGroup(c, enc, groupData)
	}
	return iterateSwissMapTables(s, c, tablePtrSliceDataItem, unlimitedItems, callback)
}

// swissMapEntryCallback is an alias for mapEntryCallback.
type swissMapEntryCallback = mapEntryCallback

// iterateSwissMapGroup iterates through a swiss map group and calls the callback
// for each valid entry.
func iterateSwissMapGroup(
	s *goSwissMapHeaderType,
	groupData []byte,
	maxItems int,
	callback swissMapEntryCallback,
) (itemsProcessed int, err error) {
	slotsData := groupData[s.slotsOffset : s.slotsOffset+s.slotsSize]
	controlWord := binary.LittleEndian.Uint64(
		groupData[s.ctrlOffset : s.ctrlOffset+uint32(s.ctrlSize)],
	)
	for i := range 8 {
		if shouldStop(maxItems, itemsProcessed) {
			break
		}
		if controlWord&(1<<(7+(8*i))) != 0 {
			// Slot is empty or deleted.
			continue
		}
		entryData := slotsData[uint32(i)*s.slotsArrayEntrySize : uint32(i+1)*s.slotsArrayEntrySize]
		keyData := entryData[s.keyFieldOffset : s.keyFieldOffset+s.keyTypeSize]
		valueData := entryData[s.valueFieldOffset : s.valueFieldOffset+s.valueTypeSize]

		shouldContinue, err := callback(keyData, valueData, i)
		if err != nil {
			return itemsProcessed, err
		}
		if !shouldContinue {
			return itemsProcessed, nil
		}
		itemsProcessed++
	}
	return itemsProcessed, nil
}

func (s *goSwissMapHeaderType) encodeSwissMapGroup(
	c *encodingContext,
	enc *jsontext.Encoder,
	groupData []byte,
) (valuesEncoded int, err error) {
	callback := makeEncodeMapEntryCallback(
		c, enc,
		s.keyTypeID, s.keyTypeName,
		s.valueTypeID, s.valueTypeName,
	)
	return iterateSwissMapGroup(s, groupData, unlimitedItems, callback)
}

// formatSwissMapTables traverses the table pointer slice and formats the data
// items for each table.
func (s *goSwissMapHeaderType) formatSwissMapTables(
	c *encodingContext,
	buf *bytes.Buffer,
	tablePtrSliceData []byte,
	limits *formatLimits,
) (totalElementsFormatted int, err error) {
	maxItems := limits.maxCollectionItems
	callback := func(groupData []byte) (int, error) {
		// Update limits to reflect remaining items we can format.
		remainingItems := maxItems
		if maxItems != unlimitedItems {
			remainingItems = max(maxItems-totalElementsFormatted, 0)
			if remainingItems == 0 {
				return 0, nil
			}
		}
		needComma := totalElementsFormatted > 0
		originalMaxItems := limits.maxCollectionItems
		limits.maxCollectionItems = remainingItems
		elementsFormatted, err := s.formatSwissMapGroup(
			c, buf, groupData, needComma, limits,
		)
		limits.maxCollectionItems = originalMaxItems
		if err != nil {
			return elementsFormatted, err
		}
		totalElementsFormatted += elementsFormatted
		return elementsFormatted, nil
	}
	return iterateSwissMapTables(s, c, tablePtrSliceData, maxItems, callback)
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
	maxItems := limits.maxCollectionItems
	keyType, ok := c.getType(s.keyTypeID)
	if !ok {
		return 0, fmt.Errorf("key type not found: %d", s.keyTypeID)
	}
	valueType, ok := c.getType(s.valueTypeID)
	if !ok {
		return 0, fmt.Errorf("value type not found: %d", s.valueTypeID)
	}
	callback := makeFormatMapEntryCallback(
		c, buf, limits, needComma,
		keyType.irType(),
		valueType.irType(),
	)
	return iterateSwissMapGroup(s, groupData, maxItems, callback)
}
