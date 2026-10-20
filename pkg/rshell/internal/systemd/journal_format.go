// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	journalHeaderMinSize     = 208
	journalHeaderCurrentSize = 272
	journalObjectHeaderSize  = 16

	journalStateOffline  = 0
	journalStateOnline   = 1
	journalStateArchived = 2

	journalObjectData           = 1
	journalObjectField          = 2
	journalObjectEntry          = 3
	journalObjectDataHashTable  = 4
	journalObjectFieldHashTable = 5
	journalObjectEntryArray     = 6
	journalObjectTag            = 7

	journalObjectCompressedXZ    = 1 << 0
	journalObjectCompressedLZ4   = 1 << 1
	journalObjectCompressedZSTD  = 1 << 2
	journalObjectCompressionMask = journalObjectCompressedXZ |
		journalObjectCompressedLZ4 |
		journalObjectCompressedZSTD

	journalHeaderIncompatibleCompressedXZ   = 1 << 0
	journalHeaderIncompatibleCompressedLZ4  = 1 << 1
	journalHeaderIncompatibleKeyedHash      = 1 << 2
	journalHeaderIncompatibleCompressedZSTD = 1 << 3
	journalHeaderIncompatibleCompact        = 1 << 4
	journalHeaderKnownIncompatible          = journalHeaderIncompatibleCompressedXZ |
		journalHeaderIncompatibleCompressedLZ4 |
		journalHeaderIncompatibleKeyedHash |
		journalHeaderIncompatibleCompressedZSTD |
		journalHeaderIncompatibleCompact
)

var (
	errJournalCorrupt     = errors.New("corrupt systemd journal")
	errJournalUnsupported = errors.New("unsupported systemd journal format")
	errJournalLimit       = errors.New("systemd journal resource limit exceeded")
	journalSignature      = [8]byte{'L', 'P', 'K', 'S', 'H', 'H', 'R', 'H'}
)

type journalID [16]byte

func (id journalID) String() string {
	return hex.EncodeToString(id[:])
}

func (id journalID) zero() bool {
	return id == journalID{}
}

type journalHeader struct {
	compatibleFlags   uint32
	incompatibleFlags uint32
	state             uint8
	fileID            journalID
	machineID         journalID
	tailEntryBootID   journalID
	seqnumID          journalID
	headerSize        uint64
	arenaSize         uint64
	arenaEnd          uint64

	dataHashTableOffset  uint64
	dataHashTableSize    uint64
	fieldHashTableOffset uint64
	fieldHashTableSize   uint64
	tailObjectOffset     uint64
	nObjects             uint64
	nEntries             uint64
	tailEntrySeqnum      uint64
	headEntrySeqnum      uint64
	entryArrayOffset     uint64
	headEntryRealtime    uint64
	tailEntryRealtime    uint64
	tailEntryMonotonic   uint64

	tailEntryArrayOffset   uint32
	tailEntryArrayNEntries uint32
	hasTailEntryArray      bool
	tailEntryOffset        uint64
	hasTailEntryOffset     bool
}

func (h journalHeader) compact() bool {
	return h.incompatibleFlags&journalHeaderIncompatibleCompact != 0
}

func (h journalHeader) keyedHash() bool {
	return h.incompatibleFlags&journalHeaderIncompatibleKeyedHash != 0
}

type journalObject struct {
	offset     uint64
	objectType uint8
	flags      uint8
	size       uint64
}

type journalFileView struct {
	name   string
	reader io.ReaderAt
	size   uint64
	header journalHeader
}

func newJournalFileView(name string, reader io.ReaderAt, size uint64) (*journalFileView, error) {
	if reader == nil {
		return nil, fmt.Errorf("journal reader is nil")
	}
	if size > math.MaxInt64 {
		return nil, journalUnsupported(name, "file is larger than the supported reader offset range")
	}

	header, err := readJournalHeader(name, reader, size)
	if err != nil {
		return nil, err
	}
	return &journalFileView{name: name, reader: reader, size: size, header: header}, nil
}

func readJournalHeader(name string, reader io.ReaderAt, fileSize uint64) (journalHeader, error) {
	if fileSize < journalHeaderMinSize {
		return journalHeader{}, journalCorrupt(name, 0, "file is smaller than the minimum %d-byte header", journalHeaderMinSize)
	}

	var raw [journalHeaderCurrentSize]byte
	if err := readJournalAt(name, reader, fileSize, 0, raw[:journalHeaderMinSize]); err != nil {
		return journalHeader{}, err
	}
	if string(raw[:len(journalSignature)]) != string(journalSignature[:]) {
		return journalHeader{}, journalCorrupt(name, 0, "invalid journal signature")
	}

	headerSize := binary.LittleEndian.Uint64(raw[88:96])
	if headerSize < journalHeaderMinSize {
		return journalHeader{}, journalCorrupt(name, 88, "header size %d is smaller than %d", headerSize, journalHeaderMinSize)
	}
	if headerSize%8 != 0 {
		return journalHeader{}, journalCorrupt(name, 88, "header size %d is not 8-byte aligned", headerSize)
	}
	if headerSize > fileSize {
		return journalHeader{}, journalCorrupt(name, 88, "header size %d exceeds the %d-byte file", headerSize, fileSize)
	}

	readSize := headerSize
	if readSize > journalHeaderCurrentSize {
		readSize = journalHeaderCurrentSize
	}
	if readSize > journalHeaderMinSize {
		if err := readJournalAt(name, reader, fileSize, journalHeaderMinSize, raw[journalHeaderMinSize:readSize]); err != nil {
			return journalHeader{}, err
		}
	}

	incompatibleFlags := binary.LittleEndian.Uint32(raw[12:16])
	if unknown := incompatibleFlags &^ uint32(journalHeaderKnownIncompatible); unknown != 0 {
		return journalHeader{}, journalUnsupported(name, "unknown incompatible feature flags 0x%08x", unknown)
	}
	state := raw[16]
	if state > journalStateArchived {
		return journalHeader{}, journalCorrupt(name, 16, "invalid journal state %d", state)
	}

	arenaSize := binary.LittleEndian.Uint64(raw[96:104])
	if arenaSize > math.MaxUint64-headerSize {
		return journalHeader{}, journalCorrupt(name, 96, "header and arena sizes overflow")
	}
	arenaEnd := headerSize + arenaSize
	if arenaEnd > fileSize {
		return journalHeader{}, journalCorrupt(name, 96, "journal arena ends at %d beyond the %d-byte file", arenaEnd, fileSize)
	}

	header := journalHeader{
		compatibleFlags:      binary.LittleEndian.Uint32(raw[8:12]),
		incompatibleFlags:    incompatibleFlags,
		state:                state,
		headerSize:           headerSize,
		arenaSize:            arenaSize,
		arenaEnd:             arenaEnd,
		dataHashTableOffset:  binary.LittleEndian.Uint64(raw[104:112]),
		dataHashTableSize:    binary.LittleEndian.Uint64(raw[112:120]),
		fieldHashTableOffset: binary.LittleEndian.Uint64(raw[120:128]),
		fieldHashTableSize:   binary.LittleEndian.Uint64(raw[128:136]),
		tailObjectOffset:     binary.LittleEndian.Uint64(raw[136:144]),
		nObjects:             binary.LittleEndian.Uint64(raw[144:152]),
		nEntries:             binary.LittleEndian.Uint64(raw[152:160]),
		tailEntrySeqnum:      binary.LittleEndian.Uint64(raw[160:168]),
		headEntrySeqnum:      binary.LittleEndian.Uint64(raw[168:176]),
		entryArrayOffset:     binary.LittleEndian.Uint64(raw[176:184]),
		headEntryRealtime:    binary.LittleEndian.Uint64(raw[184:192]),
		tailEntryRealtime:    binary.LittleEndian.Uint64(raw[192:200]),
		tailEntryMonotonic:   binary.LittleEndian.Uint64(raw[200:208]),
	}
	copy(header.fileID[:], raw[24:40])
	copy(header.machineID[:], raw[40:56])
	copy(header.tailEntryBootID[:], raw[56:72])
	copy(header.seqnumID[:], raw[72:88])

	if headerSize >= 264 {
		header.tailEntryArrayOffset = binary.LittleEndian.Uint32(raw[256:260])
		header.tailEntryArrayNEntries = binary.LittleEndian.Uint32(raw[260:264])
		header.hasTailEntryArray = true
	}
	if headerSize >= 272 {
		header.tailEntryOffset = binary.LittleEndian.Uint64(raw[264:272])
		header.hasTailEntryOffset = true
	}
	if header.hasTailEntryArray {
		if (header.tailEntryArrayOffset == 0) != (header.tailEntryArrayNEntries == 0) {
			return journalHeader{}, journalCorrupt(name, uint64(header.tailEntryArrayOffset), "tail entry array offset and count are inconsistent")
		}
		if header.tailEntryArrayOffset != 0 {
			if err := validateJournalObjectOffset(name, header, uint64(header.tailEntryArrayOffset), "tail entry array"); err != nil {
				return journalHeader{}, err
			}
			if header.entryArrayOffset == 0 || header.entryArrayOffset > uint64(header.tailEntryArrayOffset) {
				return journalHeader{}, journalCorrupt(name, uint64(header.tailEntryArrayOffset), "tail entry array precedes the global entry array chain")
			}
		}
	}

	for _, reference := range []struct {
		offset uint64
		label  string
	}{
		{offset: header.tailObjectOffset, label: "tail object"},
		{offset: header.entryArrayOffset, label: "entry array"},
		{offset: header.tailEntryOffset, label: "tail entry"},
	} {
		if reference.offset == 0 {
			continue
		}
		if err := validateJournalObjectOffset(name, header, reference.offset, reference.label); err != nil {
			return journalHeader{}, err
		}
	}
	if err := validateJournalHashTableRange(name, header, header.dataHashTableOffset, header.dataHashTableSize, "data"); err != nil {
		return journalHeader{}, err
	}
	if err := validateJournalHashTableRange(name, header, header.fieldHashTableOffset, header.fieldHashTableSize, "field"); err != nil {
		return journalHeader{}, err
	}

	return header, nil
}

func validateJournalObjectOffset(name string, header journalHeader, offset uint64, label string) error {
	if offset%8 != 0 {
		return journalCorrupt(name, offset, "%s offset is not 8-byte aligned", label)
	}
	if offset < header.headerSize {
		return journalCorrupt(name, offset, "%s offset points into the journal header", label)
	}
	if offset > header.arenaEnd || header.arenaEnd-offset < journalObjectHeaderSize {
		return journalCorrupt(name, offset, "%s offset does not contain a complete object header", label)
	}
	if header.tailObjectOffset != 0 && offset > header.tailObjectOffset {
		return journalCorrupt(name, offset, "%s offset is after the tail object", label)
	}
	return nil
}

func validateJournalHashTableRange(name string, header journalHeader, offset, size uint64, label string) error {
	if (offset == 0) != (size == 0) {
		return journalCorrupt(name, offset, "%s hash table offset and size are inconsistent", label)
	}
	if offset == 0 {
		return nil
	}
	if offset < journalObjectHeaderSize {
		return journalCorrupt(name, offset, "%s hash table payload offset is invalid", label)
	}
	objectOffset := offset - journalObjectHeaderSize
	if err := validateJournalObjectOffset(name, header, objectOffset, label+" hash table"); err != nil {
		return err
	}
	if size%16 != 0 {
		return journalCorrupt(name, offset, "%s hash table size %d is not a multiple of 16", label, size)
	}
	if size > header.arenaEnd-offset {
		return journalCorrupt(name, offset, "%s hash table extends beyond the journal arena", label)
	}
	return nil
}

func (f *journalFileView) objectAt(offset uint64, expectedType uint8) (journalObject, error) {
	if err := validateJournalObjectOffset(f.name, f.header, offset, "object"); err != nil {
		return journalObject{}, err
	}

	var raw [journalObjectHeaderSize]byte
	if err := readJournalAt(f.name, f.reader, f.size, offset, raw[:]); err != nil {
		return journalObject{}, err
	}
	object := journalObject{
		offset:     offset,
		objectType: raw[0],
		flags:      raw[1],
		size:       binary.LittleEndian.Uint64(raw[8:16]),
	}
	if expectedType != 0 && object.objectType != expectedType {
		return journalObject{}, journalCorrupt(f.name, offset, "expected object type %d, found %d", expectedType, object.objectType)
	}
	if unknown := object.flags &^ uint8(journalObjectCompressionMask); unknown != 0 {
		return journalObject{}, journalUnsupported(f.name, "object at offset %d uses unknown flags 0x%02x", offset, unknown)
	}
	if object.flags != 0 && object.objectType != journalObjectData {
		return journalObject{}, journalCorrupt(f.name, offset, "compression flags are set on non-DATA object type %d", object.objectType)
	}
	if object.flags != 0 && object.flags&(object.flags-1) != 0 {
		return journalObject{}, journalCorrupt(f.name, offset, "DATA object has multiple compression flags")
	}
	for _, compression := range []struct {
		objectFlag uint8
		headerFlag uint32
		name       string
	}{
		{objectFlag: journalObjectCompressedXZ, headerFlag: journalHeaderIncompatibleCompressedXZ, name: "XZ"},
		{objectFlag: journalObjectCompressedLZ4, headerFlag: journalHeaderIncompatibleCompressedLZ4, name: "LZ4"},
		{objectFlag: journalObjectCompressedZSTD, headerFlag: journalHeaderIncompatibleCompressedZSTD, name: "ZSTD"},
	} {
		if object.flags&compression.objectFlag != 0 && f.header.incompatibleFlags&compression.headerFlag == 0 {
			return journalObject{}, journalCorrupt(f.name, offset, "%s-compressed DATA object is not declared in the journal header", compression.name)
		}
	}
	if object.size < journalObjectHeaderSize {
		return journalObject{}, journalCorrupt(f.name, offset, "object size %d is smaller than its header", object.size)
	}
	if object.size > f.header.arenaEnd-offset {
		return journalObject{}, journalCorrupt(f.name, offset, "object size %d extends beyond the journal arena", object.size)
	}
	return object, nil
}

func readJournalAt(name string, reader io.ReaderAt, fileSize, offset uint64, destination []byte) error {
	if offset > fileSize || uint64(len(destination)) > fileSize-offset {
		return journalCorrupt(name, offset, "read of %d bytes exceeds the %d-byte file", len(destination), fileSize)
	}
	if offset > math.MaxInt64 {
		return journalUnsupported(name, "object offset %d exceeds the supported reader range", offset)
	}
	n, err := reader.ReadAt(destination, int64(offset))
	if n != len(destination) {
		return journalCorrupt(name, offset, "short read: got %d of %d bytes", n, len(destination))
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read systemd journal %q at offset %d: %w", name, offset, err)
	}
	return nil
}

func journalCorrupt(name string, offset uint64, format string, arguments ...any) error {
	return fmt.Errorf("%w %q at offset %d: %s", errJournalCorrupt, name, offset, fmt.Sprintf(format, arguments...))
}

func journalUnsupported(name, format string, arguments ...any) error {
	return fmt.Errorf("%w %q: %s", errJournalUnsupported, name, fmt.Sprintf(format, arguments...))
}
