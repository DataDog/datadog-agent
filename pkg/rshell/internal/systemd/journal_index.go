// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	journalHashItemSize      = 16
	maxJournalHashChainDepth = 128
)

func (f *journalFileView) dataHash(payload []byte) uint64 {
	if f.header.keyedHash() {
		return journalSipHash24(payload, f.header.fileID)
	}
	return journalJenkinsHash64(payload)
}

func (f *journalFileView) findDataObject(payload []byte) (journalDataObject, bool, error) {
	if len(payload) == 0 {
		return journalDataObject{}, false, fmt.Errorf("journal DATA lookup payload is empty")
	}
	if len(payload) > maxJournalPayloadRead {
		return journalDataObject{}, false, fmt.Errorf("journal DATA lookup payload exceeds %d bytes", maxJournalPayloadRead)
	}
	if f.header.dataHashTableSize == 0 {
		return journalDataObject{}, false, nil
	}

	tableObjectOffset := f.header.dataHashTableOffset - journalObjectHeaderSize
	table, err := f.objectAt(tableObjectOffset, journalObjectDataHashTable)
	if err != nil {
		return journalDataObject{}, false, err
	}
	if table.size-journalObjectHeaderSize != f.header.dataHashTableSize {
		return journalDataObject{}, false, journalCorrupt(
			f.name,
			tableObjectOffset,
			"DATA hash table object payload is %d bytes; header declares %d",
			table.size-journalObjectHeaderSize,
			f.header.dataHashTableSize,
		)
	}
	bucketCount := f.header.dataHashTableSize / journalHashItemSize
	if bucketCount == 0 {
		return journalDataObject{}, false, journalCorrupt(f.name, f.header.dataHashTableOffset, "DATA hash table has no buckets")
	}

	hash := f.dataHash(payload)
	bucket := hash % bucketCount
	itemOffset := f.header.dataHashTableOffset + bucket*journalHashItemSize
	var item [journalHashItemSize]byte
	if err := readJournalAt(f.name, f.reader, f.size, itemOffset, item[:]); err != nil {
		return journalDataObject{}, false, err
	}
	headOffset := binary.LittleEndian.Uint64(item[0:8])
	tailOffset := binary.LittleEndian.Uint64(item[8:16])
	if (headOffset == 0) != (tailOffset == 0) {
		return journalDataObject{}, false, journalCorrupt(f.name, itemOffset, "DATA hash bucket head and tail are inconsistent")
	}
	if headOffset == 0 {
		return journalDataObject{}, false, nil
	}
	if err := validateJournalObjectOffset(f.name, f.header, headOffset, "DATA hash bucket head"); err != nil {
		return journalDataObject{}, false, err
	}
	if err := validateJournalObjectOffset(f.name, f.header, tailOffset, "DATA hash bucket tail"); err != nil {
		return journalDataObject{}, false, err
	}

	seen := make(map[uint64]struct{}, maxJournalHashChainDepth)
	for offset, depth := headOffset, 0; offset != 0; depth++ {
		if depth >= maxJournalHashChainDepth {
			return journalDataObject{}, false, journalLimit(f.name, offset, "DATA hash chain exceeds %d objects", maxJournalHashChainDepth)
		}
		if _, exists := seen[offset]; exists {
			return journalDataObject{}, false, journalCorrupt(f.name, offset, "DATA hash chain contains a cycle")
		}
		seen[offset] = struct{}{}

		data, err := f.dataObjectAt(offset)
		if err != nil {
			return journalDataObject{}, false, err
		}
		if data.hash%bucketCount != bucket {
			return journalDataObject{}, false, journalCorrupt(f.name, offset, "DATA object hash belongs to bucket %d instead of %d", data.hash%bucketCount, bucket)
		}
		if data.hash == hash {
			equal, err := f.dataPayloadEqual(data, payload)
			if err != nil {
				return journalDataObject{}, false, err
			}
			if equal {
				return data, true, nil
			}
		}

		if offset == tailOffset {
			if data.nextHashOffset != 0 {
				return journalDataObject{}, false, journalCorrupt(f.name, offset, "DATA hash bucket continues after its declared tail")
			}
			return journalDataObject{}, false, nil
		}
		if data.nextHashOffset == 0 {
			return journalDataObject{}, false, journalCorrupt(f.name, offset, "DATA hash bucket ends before its declared tail")
		}
		offset = data.nextHashOffset
	}

	return journalDataObject{}, false, journalCorrupt(f.name, itemOffset, "DATA hash bucket did not reach its declared tail")
}

func (f *journalFileView) dataPayloadEqual(data journalDataObject, expected []byte) (bool, error) {
	switch data.flags {
	case 0:
		if data.payloadSize != uint64(len(expected)) {
			return false, nil
		}
	case journalObjectCompressedLZ4:
		if data.payloadSize <= 8 {
			return false, journalCorrupt(f.name, data.payloadOffset, "LZ4 DATA payload is too small")
		}
		var sizeBytes [8]byte
		if err := readJournalAt(f.name, f.reader, f.size, data.payloadOffset, sizeBytes[:]); err != nil {
			return false, err
		}
		if binary.LittleEndian.Uint64(sizeBytes[:]) != uint64(len(expected)) {
			return false, nil
		}
	}

	payload, truncated, err := f.readDataPayload(data, len(expected))
	if err != nil {
		return false, err
	}
	return !truncated && bytes.Equal(payload, expected), nil
}
