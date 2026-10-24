// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"encoding/binary"
	"fmt"
)

const (
	journalEntryHeaderSize           = 64
	journalEntryArrayHeaderSize      = 24
	journalEntryRegularItemSize      = 16
	journalEntryCompactItemSize      = 4
	journalEntryArrayRegularItemSize = 8

	maxJournalEntryFields = 1024
	maxJournalEntryArrays = 128
)

type journalEntryItem struct {
	dataOffset uint64
	hash       uint64
}

type journalEntryObject struct {
	journalObject
	seqnum    uint64
	realtime  uint64
	monotonic uint64
	bootID    journalID
	xorHash   uint64
	items     []journalEntryItem
}

func (f *journalFileView) entryObjectAt(offset uint64) (journalEntryObject, error) {
	object, err := f.objectAt(offset, journalObjectEntry)
	if err != nil {
		return journalEntryObject{}, err
	}
	if object.size < journalEntryHeaderSize {
		return journalEntryObject{}, journalCorrupt(f.name, offset, "ENTRY object size %d is smaller than its header", object.size)
	}

	itemSize := uint64(journalEntryRegularItemSize)
	if f.header.compact() {
		itemSize = journalEntryCompactItemSize
	}
	itemsSize := object.size - journalEntryHeaderSize
	if itemsSize%itemSize != 0 {
		return journalEntryObject{}, journalCorrupt(f.name, offset, "ENTRY item payload size %d is not a multiple of %d", itemsSize, itemSize)
	}
	itemCount := itemsSize / itemSize
	if itemCount == 0 {
		return journalEntryObject{}, journalCorrupt(f.name, offset, "ENTRY object has no DATA items")
	}
	if itemCount > maxJournalEntryFields {
		return journalEntryObject{}, journalLimit(f.name, offset, "ENTRY object has %d DATA items; maximum is %d", itemCount, maxJournalEntryFields)
	}

	contents := make([]byte, int(object.size))
	if err := readJournalAt(f.name, f.reader, f.size, offset, contents); err != nil {
		return journalEntryObject{}, err
	}
	entry := journalEntryObject{
		journalObject: object,
		seqnum:        binary.LittleEndian.Uint64(contents[16:24]),
		realtime:      binary.LittleEndian.Uint64(contents[24:32]),
		monotonic:     binary.LittleEndian.Uint64(contents[32:40]),
		xorHash:       binary.LittleEndian.Uint64(contents[56:64]),
		items:         make([]journalEntryItem, 0, int(itemCount)),
	}
	copy(entry.bootID[:], contents[40:56])
	for index := uint64(0); index < itemCount; index++ {
		position := uint64(journalEntryHeaderSize) + index*itemSize
		item := journalEntryItem{}
		if f.header.compact() {
			item.dataOffset = uint64(binary.LittleEndian.Uint32(contents[position : position+4]))
		} else {
			item.dataOffset = binary.LittleEndian.Uint64(contents[position : position+8])
			item.hash = binary.LittleEndian.Uint64(contents[position+8 : position+16])
		}
		if item.dataOffset == 0 {
			return journalEntryObject{}, journalCorrupt(f.name, offset, "ENTRY item %d has a zero DATA offset", index)
		}
		if err := validateJournalObjectOffset(f.name, f.header, item.dataOffset, fmt.Sprintf("ENTRY item %d DATA", index)); err != nil {
			return journalEntryObject{}, err
		}
		entry.items = append(entry.items, item)
	}
	return entry, nil
}

type journalEntryArray struct {
	journalObject
	nextOffset  uint64
	itemsOffset uint64
	capacity    uint64
}

func (f *journalFileView) entryArrayAt(offset uint64) (journalEntryArray, error) {
	object, err := f.objectAt(offset, journalObjectEntryArray)
	if err != nil {
		return journalEntryArray{}, err
	}
	if object.size < journalEntryArrayHeaderSize {
		return journalEntryArray{}, journalCorrupt(f.name, offset, "ENTRY_ARRAY object size %d is smaller than its header", object.size)
	}

	itemSize := uint64(journalEntryArrayRegularItemSize)
	if f.header.compact() {
		itemSize = journalEntryCompactItemSize
	}
	itemsSize := object.size - journalEntryArrayHeaderSize
	if itemsSize%itemSize != 0 {
		return journalEntryArray{}, journalCorrupt(f.name, offset, "ENTRY_ARRAY item payload size %d is not a multiple of %d", itemsSize, itemSize)
	}
	capacity := itemsSize / itemSize
	if capacity == 0 {
		return journalEntryArray{}, journalCorrupt(f.name, offset, "ENTRY_ARRAY object has no item capacity")
	}

	var header [journalEntryArrayHeaderSize]byte
	if err := readJournalAt(f.name, f.reader, f.size, offset, header[:]); err != nil {
		return journalEntryArray{}, err
	}
	array := journalEntryArray{
		journalObject: object,
		nextOffset:    binary.LittleEndian.Uint64(header[16:24]),
		itemsOffset:   offset + journalEntryArrayHeaderSize,
		capacity:      capacity,
	}
	if array.nextOffset != 0 {
		if err := validateJournalObjectOffset(f.name, f.header, array.nextOffset, "next ENTRY_ARRAY"); err != nil {
			return journalEntryArray{}, err
		}
	}
	return array, nil
}

func (f *journalFileView) entryArrayItem(array journalEntryArray, index uint64) (uint64, error) {
	if index >= array.capacity {
		return 0, journalCorrupt(f.name, array.offset, "ENTRY_ARRAY item index %d exceeds capacity %d", index, array.capacity)
	}
	itemSize := uint64(journalEntryArrayRegularItemSize)
	if f.header.compact() {
		itemSize = journalEntryCompactItemSize
	}
	position := array.itemsOffset + index*itemSize
	var raw [8]byte
	if err := readJournalAt(f.name, f.reader, f.size, position, raw[:itemSize]); err != nil {
		return 0, err
	}
	offset := binary.LittleEndian.Uint64(raw[:])
	if f.header.compact() {
		offset = uint64(binary.LittleEndian.Uint32(raw[:4]))
	}
	if offset == 0 {
		return 0, journalCorrupt(f.name, position, "ENTRY_ARRAY item %d has a zero ENTRY offset", index)
	}
	if err := validateJournalObjectOffset(f.name, f.header, offset, "ENTRY_ARRAY item"); err != nil {
		return 0, err
	}
	return offset, nil
}

type journalEntryArraySegment struct {
	array journalEntryArray
	count uint64
}

type journalEntryOffsetIterator struct {
	file          *journalFileView
	inlineOffset  uint64
	inlinePending bool
	segments      []journalEntryArraySegment
	segmentIndex  int
	itemIndex     uint64
	lastOffset    uint64
}

func (f *journalFileView) entryOffsetsForData(data journalDataObject) (*journalEntryOffsetIterator, error) {
	iterator := &journalEntryOffsetIterator{file: f, segmentIndex: -1}
	if data.nEntries == 0 {
		if data.entryOffset != 0 || data.entryArrayOffset != 0 || data.tailEntryArrayOffset != 0 || data.tailEntryArrayNEntries != 0 {
			return nil, journalCorrupt(f.name, data.offset, "unreferenced DATA object has entry offsets")
		}
		return iterator, nil
	}
	if data.entryOffset == 0 {
		return nil, journalCorrupt(f.name, data.offset, "referenced DATA object has no inline ENTRY offset")
	}
	if data.nEntries > f.header.nEntries {
		return nil, journalCorrupt(f.name, data.offset, "DATA object references %d entries but the journal has %d", data.nEntries, f.header.nEntries)
	}
	iterator.inlineOffset = data.entryOffset
	iterator.inlinePending = true

	remaining := data.nEntries - 1
	if remaining == 0 {
		if data.entryArrayOffset != 0 {
			return nil, journalCorrupt(f.name, data.offset, "single-entry DATA object has an ENTRY_ARRAY")
		}
		return iterator, nil
	}
	if data.entryArrayOffset == 0 {
		return nil, journalCorrupt(f.name, data.offset, "multi-entry DATA object has no ENTRY_ARRAY")
	}

	seen := make(map[uint64]struct{}, maxJournalEntryArrays)
	offset := data.entryArrayOffset
	for remaining > 0 {
		if len(iterator.segments) >= maxJournalEntryArrays {
			return nil, journalLimit(f.name, offset, "ENTRY_ARRAY chain exceeds %d objects", maxJournalEntryArrays)
		}
		if _, exists := seen[offset]; exists {
			return nil, journalCorrupt(f.name, offset, "ENTRY_ARRAY chain contains a cycle")
		}
		seen[offset] = struct{}{}

		array, err := f.entryArrayAt(offset)
		if err != nil {
			return nil, err
		}
		count := array.capacity
		if count > remaining {
			count = remaining
		}
		iterator.segments = append(iterator.segments, journalEntryArraySegment{array: array, count: count})
		remaining -= count
		if remaining == 0 {
			if array.nextOffset != 0 {
				return nil, journalCorrupt(f.name, array.offset, "ENTRY_ARRAY chain continues after all referenced entries")
			}
			break
		}
		if array.nextOffset == 0 {
			return nil, journalCorrupt(f.name, array.offset, "ENTRY_ARRAY chain ends before all referenced entries")
		}
		offset = array.nextOffset
	}

	if data.hasTailEntryArrayReference {
		last := iterator.segments[len(iterator.segments)-1]
		if uint64(data.tailEntryArrayOffset) != last.array.offset || uint64(data.tailEntryArrayNEntries) != last.count {
			return nil, journalCorrupt(f.name, data.offset, "DATA tail ENTRY_ARRAY metadata does not match its array chain")
		}
	}
	iterator.segmentIndex = len(iterator.segments) - 1
	iterator.itemIndex = iterator.segments[iterator.segmentIndex].count
	return iterator, nil
}

func (i *journalEntryOffsetIterator) previous() (uint64, bool, error) {
	for i.segmentIndex >= 0 {
		if i.itemIndex > 0 {
			i.itemIndex--
			offset, err := i.file.entryArrayItem(i.segments[i.segmentIndex].array, i.itemIndex)
			if err != nil {
				return 0, false, err
			}
			if err := i.validateDescending(offset); err != nil {
				return 0, false, err
			}
			return offset, true, nil
		}
		i.segmentIndex--
		if i.segmentIndex >= 0 {
			i.itemIndex = i.segments[i.segmentIndex].count
		}
	}
	if i.inlinePending {
		i.inlinePending = false
		if err := i.validateDescending(i.inlineOffset); err != nil {
			return 0, false, err
		}
		return i.inlineOffset, true, nil
	}
	return 0, false, nil
}

func (i *journalEntryOffsetIterator) validateDescending(offset uint64) error {
	if i.lastOffset != 0 && offset >= i.lastOffset {
		return journalCorrupt(i.file.name, offset, "ENTRY offsets are not strictly increasing in the forward index")
	}
	i.lastOffset = offset
	return nil
}
