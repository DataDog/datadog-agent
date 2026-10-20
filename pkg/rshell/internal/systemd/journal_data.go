// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

const (
	journalDataRegularHeaderSize = 64
	journalDataCompactHeaderSize = 72

	maxJournalDecodeWindow   = 8 * 1024 * 1024
	maxJournalEncodedRead    = 8 * 1024 * 1024
	maxJournalLZ4DataSize    = 8 * 1024 * 1024
	maxJournalPayloadRead    = maxJournalFieldSize + 512
	journalLZ4ReadBufferSize = 4 * 1024
)

type journalDataObject struct {
	journalObject
	hash                       uint64
	nextHashOffset             uint64
	nextFieldOffset            uint64
	entryOffset                uint64
	entryArrayOffset           uint64
	nEntries                   uint64
	tailEntryArrayOffset       uint32
	tailEntryArrayNEntries     uint32
	hasTailEntryArrayReference bool
	payloadOffset              uint64
	payloadSize                uint64
}

func (f *journalFileView) dataObjectAt(offset uint64) (journalDataObject, error) {
	object, err := f.objectAt(offset, journalObjectData)
	if err != nil {
		return journalDataObject{}, err
	}

	headerSize := uint64(journalDataRegularHeaderSize)
	if f.header.compact() {
		headerSize = journalDataCompactHeaderSize
	}
	if object.size < headerSize {
		return journalDataObject{}, journalCorrupt(f.name, offset, "DATA object size %d is smaller than its %d-byte header", object.size, headerSize)
	}

	var raw [journalDataCompactHeaderSize]byte
	if err := readJournalAt(f.name, f.reader, f.size, offset, raw[:headerSize]); err != nil {
		return journalDataObject{}, err
	}
	data := journalDataObject{
		journalObject:    object,
		hash:             binary.LittleEndian.Uint64(raw[16:24]),
		nextHashOffset:   binary.LittleEndian.Uint64(raw[24:32]),
		nextFieldOffset:  binary.LittleEndian.Uint64(raw[32:40]),
		entryOffset:      binary.LittleEndian.Uint64(raw[40:48]),
		entryArrayOffset: binary.LittleEndian.Uint64(raw[48:56]),
		nEntries:         binary.LittleEndian.Uint64(raw[56:64]),
		payloadOffset:    offset + headerSize,
		payloadSize:      object.size - headerSize,
	}
	if f.header.compact() {
		data.tailEntryArrayOffset = binary.LittleEndian.Uint32(raw[64:68])
		data.tailEntryArrayNEntries = binary.LittleEndian.Uint32(raw[68:72])
		data.hasTailEntryArrayReference = true
	}

	for _, reference := range []struct {
		offset uint64
		label  string
	}{
		{offset: data.nextHashOffset, label: "next DATA hash"},
		{offset: data.nextFieldOffset, label: "next DATA field"},
		{offset: data.entryOffset, label: "first DATA entry"},
		{offset: data.entryArrayOffset, label: "DATA entry array"},
	} {
		if reference.offset == 0 {
			continue
		}
		if err := validateJournalObjectOffset(f.name, f.header, reference.offset, reference.label); err != nil {
			return journalDataObject{}, err
		}
	}
	if data.hasTailEntryArrayReference {
		if (data.tailEntryArrayOffset == 0) != (data.tailEntryArrayNEntries == 0) {
			return journalDataObject{}, journalCorrupt(f.name, uint64(data.tailEntryArrayOffset), "DATA tail entry array offset and count are inconsistent")
		}
		if data.tailEntryArrayOffset != 0 {
			tailOffset := uint64(data.tailEntryArrayOffset)
			if err := validateJournalObjectOffset(f.name, f.header, tailOffset, "DATA tail entry array"); err != nil {
				return journalDataObject{}, err
			}
			if data.entryArrayOffset == 0 || data.entryArrayOffset > tailOffset {
				return journalDataObject{}, journalCorrupt(f.name, tailOffset, "DATA tail entry array precedes its entry array chain")
			}
		}
	}

	return data, nil
}

func (f *journalFileView) readDataPayload(data journalDataObject, limit int) ([]byte, bool, error) {
	if limit <= 0 {
		return nil, false, fmt.Errorf("journal DATA payload limit must be positive")
	}
	if limit == math.MaxInt {
		return nil, false, fmt.Errorf("journal DATA payload limit is too large")
	}
	if limit > maxJournalPayloadRead {
		return nil, false, fmt.Errorf("journal DATA payload limit %d exceeds maximum %d", limit, maxJournalPayloadRead)
	}

	switch data.flags {
	case 0:
		return f.readUncompressedDataPayload(data, limit)
	case journalObjectCompressedXZ:
		decoder, err := (xz.ReaderConfig{
			DictCap:      maxJournalDecodeWindow,
			SingleStream: true,
		}).NewReader(f.compressedPayloadReader(data))
		if err != nil {
			if errors.Is(err, errJournalLimit) {
				return nil, false, err
			}
			return nil, false, journalCorrupt(f.name, data.payloadOffset, "decode XZ DATA payload: %v", err)
		}
		return readDecodedJournalPayload(f.name, data.payloadOffset, "XZ", decoder, limit)
	case journalObjectCompressedLZ4:
		return f.readLZ4DataPayload(data, limit)
	case journalObjectCompressedZSTD:
		decoder, err := zstd.NewReader(
			f.compressedPayloadReader(data),
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderLowmem(true),
			zstd.WithDecoderMaxMemory(maxJournalDecodeWindow),
			zstd.WithDecoderMaxWindow(maxJournalDecodeWindow),
		)
		if err != nil {
			if errors.Is(err, errJournalLimit) {
				return nil, false, err
			}
			return nil, false, journalCorrupt(f.name, data.payloadOffset, "decode ZSTD DATA payload: %v", err)
		}
		defer decoder.Close()
		return readDecodedJournalPayload(f.name, data.payloadOffset, "ZSTD", decoder, limit)
	default:
		return nil, false, journalUnsupported(f.name, "DATA object at offset %d uses compression flags 0x%02x", data.offset, data.flags)
	}
}

func (f *journalFileView) compressedPayloadReader(data journalDataObject) io.Reader {
	section := io.NewSectionReader(f.reader, int64(data.payloadOffset), int64(data.payloadSize))
	if data.payloadSize <= maxJournalEncodedRead {
		return section
	}
	return &journalCompressedReader{
		reader:    section,
		remaining: maxJournalEncodedRead,
		name:      f.name,
		offset:    data.payloadOffset,
	}
}

type journalCompressedReader struct {
	reader    io.Reader
	remaining int
	name      string
	offset    uint64
}

func (r *journalCompressedReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		return 0, journalLimit(r.name, r.offset, "compressed DATA input exceeds %d bytes", maxJournalEncodedRead)
	}
	if len(destination) > r.remaining {
		destination = destination[:r.remaining]
	}
	n, err := r.reader.Read(destination)
	r.remaining -= n
	return n, err
}

func (f *journalFileView) readUncompressedDataPayload(data journalDataObject, limit int) ([]byte, bool, error) {
	readSize := data.payloadSize
	if readSize > uint64(limit)+1 {
		readSize = uint64(limit) + 1
	}
	payload := make([]byte, int(readSize))
	if err := readJournalAt(f.name, f.reader, f.size, data.payloadOffset, payload); err != nil {
		return nil, false, err
	}
	if len(payload) > limit {
		return payload[:limit], true, nil
	}
	return payload, false, nil
}

func (f *journalFileView) readLZ4DataPayload(data journalDataObject, limit int) ([]byte, bool, error) {
	if data.payloadSize <= 8 {
		return nil, false, journalCorrupt(f.name, data.payloadOffset, "LZ4 DATA payload is too small")
	}

	var sizeBytes [8]byte
	if err := readJournalAt(f.name, f.reader, f.size, data.payloadOffset, sizeBytes[:]); err != nil {
		return nil, false, err
	}
	decodedSize := binary.LittleEndian.Uint64(sizeBytes[:])
	if decodedSize == 0 {
		return nil, false, journalCorrupt(f.name, data.payloadOffset, "LZ4 DATA payload has an empty decoded size")
	}
	if decodedSize > maxJournalLZ4DataSize {
		return nil, false, journalLimit(f.name, data.payloadOffset, "LZ4 DATA payload expands to %d bytes; maximum is %d", decodedSize, maxJournalLZ4DataSize)
	}
	compressedSize := data.payloadSize - 8
	if compressedSize > maxJournalEncodedRead {
		return nil, false, journalLimit(f.name, data.payloadOffset, "compressed LZ4 DATA input exceeds %d bytes", maxJournalEncodedRead)
	}

	section := io.NewSectionReader(f.reader, int64(data.payloadOffset+8), int64(compressedSize))
	decoded, truncated, err := decodeJournalLZ4Prefix(section, compressedSize, decodedSize, limit)
	if err != nil {
		return nil, false, journalCorrupt(f.name, data.payloadOffset, "decode LZ4 DATA payload: %v", err)
	}
	return decoded, truncated, nil
}

type journalLZ4Source struct {
	reader    *bufio.Reader
	remaining uint64
}

// Journal LZ4 payloads are raw blocks, while the public block decoder requires
// a full-size destination. Decode sequences directly to materialize limit+1.
func decodeJournalLZ4Prefix(reader io.Reader, compressedSize, decodedSize uint64, limit int) ([]byte, bool, error) {
	outputSize := decodedSize
	truncated := decodedSize > uint64(limit)
	if truncated {
		outputSize = uint64(limit) + 1
	}
	if outputSize > uint64(math.MaxInt) {
		return nil, false, fmt.Errorf("decoded prefix exceeds the platform allocation range")
	}

	output := make([]byte, int(outputSize))
	source := journalLZ4Source{
		reader:    bufio.NewReaderSize(reader, journalLZ4ReadBufferSize),
		remaining: compressedSize,
	}
	written := 0
	for source.remaining > 0 {
		token, err := source.readByte()
		if err != nil {
			return nil, false, err
		}

		remainingDecoded := decodedSize - uint64(written)
		literalLength, err := source.readLength(uint64(token>>4), token>>4 == 15, remainingDecoded)
		if err != nil {
			return nil, false, fmt.Errorf("literal length: %w", err)
		}
		literalRead := min(literalLength, uint64(len(output)-written))
		if err := source.readFull(output[written : written+int(literalRead)]); err != nil {
			return nil, false, fmt.Errorf("literal data: %w", err)
		}
		written += int(literalRead)
		if literalRead < literalLength {
			return output[:limit], true, nil
		}
		if truncated && written == len(output) {
			return output[:limit], true, nil
		}
		if source.remaining == 0 {
			if uint64(written) != decodedSize {
				return nil, false, fmt.Errorf("decoded %d of %d bytes", written, decodedSize)
			}
			return output, false, nil
		}

		var offsetBytes [2]byte
		if err := source.readFull(offsetBytes[:]); err != nil {
			return nil, false, fmt.Errorf("match offset: %w", err)
		}
		offset := uint64(binary.LittleEndian.Uint16(offsetBytes[:]))
		if offset == 0 || offset > uint64(written) {
			return nil, false, fmt.Errorf("invalid match offset %d after %d decoded bytes", offset, written)
		}

		remainingDecoded = decodedSize - uint64(written)
		if remainingDecoded < 4 {
			return nil, false, fmt.Errorf("match exceeds the declared %d-byte decoded size", decodedSize)
		}
		matchLength, complete, err := source.readMatchLength(
			uint64(token&0x0f),
			token&0x0f == 15,
			remainingDecoded-4,
			uint64(len(output)-written),
			truncated,
		)
		if err != nil {
			return nil, false, fmt.Errorf("match length: %w", err)
		}
		matchRead := min(matchLength, uint64(len(output)-written))
		for index := uint64(0); index < matchRead; index++ {
			output[written+int(index)] = output[written+int(index)-int(offset)]
		}
		written += int(matchRead)
		if !complete || matchRead < matchLength || truncated && written == len(output) {
			return output[:limit], true, nil
		}
		if uint64(written) == decodedSize {
			if source.remaining != 0 {
				return nil, false, fmt.Errorf("compressed input continues after %d decoded bytes", decodedSize)
			}
			return output, false, nil
		}
	}

	return nil, false, fmt.Errorf("decoded %d of %d bytes", written, decodedSize)
}

func (s *journalLZ4Source) readByte() (byte, error) {
	if s.remaining == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	value, err := s.reader.ReadByte()
	if err != nil {
		return 0, err
	}
	s.remaining--
	return value, nil
}

func (s *journalLZ4Source) readFull(destination []byte) error {
	if uint64(len(destination)) > s.remaining {
		return io.ErrUnexpectedEOF
	}
	n, err := io.ReadFull(s.reader, destination)
	s.remaining -= uint64(n)
	return err
}

func (s *journalLZ4Source) readLength(base uint64, extended bool, maximum uint64) (uint64, error) {
	if base > maximum {
		return 0, fmt.Errorf("length exceeds the remaining %d decoded bytes", maximum)
	}
	if !extended {
		return base, nil
	}
	for {
		next, err := s.readByte()
		if err != nil {
			return 0, err
		}
		if uint64(next) > maximum-base {
			return 0, fmt.Errorf("length exceeds the remaining %d decoded bytes", maximum)
		}
		base += uint64(next)
		if next != 0xff {
			return base, nil
		}
	}
}

func (s *journalLZ4Source) readMatchLength(base uint64, extended bool, maximum, needed uint64, prefixOnly bool) (uint64, bool, error) {
	if base > maximum {
		return 0, false, fmt.Errorf("length exceeds the remaining %d decoded bytes", maximum+4)
	}
	if !extended {
		return base + 4, true, nil
	}
	for {
		next, err := s.readByte()
		if err != nil {
			return 0, false, err
		}
		if uint64(next) > maximum-base {
			return 0, false, fmt.Errorf("length exceeds the remaining %d decoded bytes", maximum+4)
		}
		base += uint64(next)
		if next != 0xff {
			return base + 4, true, nil
		}
		if prefixOnly && base+4 >= needed {
			return base + 4, false, nil
		}
	}
}

func readDecodedJournalPayload(name string, offset uint64, algorithm string, reader io.Reader, limit int) ([]byte, bool, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		if errors.Is(err, errJournalLimit) {
			return nil, false, err
		}
		return nil, false, journalCorrupt(name, offset, "decode %s DATA payload: %v", algorithm, err)
	}
	if len(payload) > limit {
		return payload[:limit], true, nil
	}
	return payload, false, nil
}

func journalLimit(name string, offset uint64, format string, arguments ...any) error {
	return fmt.Errorf("%w %q at offset %d: %s", errJournalLimit, name, offset, fmt.Sprintf(format, arguments...))
}
