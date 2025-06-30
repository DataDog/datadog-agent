// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package loclist supports processing DWARF loclists.
package loclist

import (
	"bytes"
	"debug/dwarf"
	"encoding/binary"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// Reader reads DWARF loclists.
type Reader struct {
	data              []byte
	debugAddr         []byte
	ptrSize           uint8
	unitVersionGetter func(unit *dwarf.Entry) (uint8, bool)
}

// NewReader creates a new Reader.
func NewReader(
	data []byte,
	debugAddr []byte,
	ptrSize uint8,
	unitVersionGetter func(unit *dwarf.Entry) (uint8, bool),
) *Reader {
	return &Reader{
		data:              data,
		debugAddr:         debugAddr,
		ptrSize:           ptrSize,
		unitVersionGetter: unitVersionGetter,
	}
}

// Loclist represents a DWARF loclist.
type Loclist struct {
	Locations []ir.Location
	Default   []ir.Piece
}

func (r *Reader) Read(unit *dwarf.Entry, offset int64, typeByteSize uint32) (Loclist, error) {
	unitVersion, ok := r.unitVersionGetter(unit)
	if !ok {
		return Loclist{}, fmt.Errorf("no unit version found for unit at offset 0x%x", unit.Offset)
	}
	if unitVersion < 2 {
		return Loclist{}, fmt.Errorf("unsupported unit version: %d", unitVersion)
	}

	if offset > int64(len(r.data)) {
		return Loclist{}, fmt.Errorf("loclist offset %d out of bounds for section length %d", offset, len(r.data))
	}
	data := r.data[offset:]
	var loclist Loclist
	var err error
	if unitVersion < 5 {
		loclist, err = readDwarf2(data, r.ptrSize, typeByteSize)
	} else {
		if r.debugAddr == nil {
			return Loclist{}, fmt.Errorf("missing debug_addr section")
		}
		addrBase, ok := unit.Val(dwarf.AttrAddrBase).(int64)
		if !ok {
			return Loclist{}, fmt.Errorf("missing addr_base attribute")
		}
		if addrBase > int64(len(r.debugAddr)) {
			return Loclist{}, fmt.Errorf("addr base %d out of bounds for section length %d", addrBase, len(r.debugAddr))
		}
		loclist, err = readDwarf5(data, r.ptrSize, typeByteSize, r.debugAddr[addrBase:])
	}
	if err != nil {
		return Loclist{}, err
	}
	loclist.Locations, err = fixLoclists(loclist.Locations, typeByteSize)
	if err != nil {
		return Loclist{}, err
	}
	return loclist, nil
}

func readDwarf2(data []byte, ptrSize uint8, typeByteSize uint32) (Loclist, error) {
	reader := bytes.NewBuffer(data)
	pcBase := uint64(0)
	var locations []ir.Location
	for {
		lo, err := readFixedSize(reader, ptrSize)
		if err != nil {
			return Loclist{}, fmt.Errorf("failed to read low PC: %w", err)
		}
		hi, err := readFixedSize(reader, ptrSize)
		if err != nil {
			return Loclist{}, fmt.Errorf("failed to read high PC: %w", err)
		}
		if lo == 0 && hi == 0 {
			break
		}
		if lo == ^uint64(0) {
			pcBase = hi
			continue
		}
		instrLen, err := readFixedSize(reader, 2)
		if err != nil {
			return Loclist{}, fmt.Errorf("failed to read instruction length: %w", err)
		}
		instrBytes := reader.Next(int(instrLen))
		if len(instrBytes) != int(instrLen) {
			return Loclist{}, fmt.Errorf("not enough bytes for instructions: expected %d, got %d", instrLen, len(instrBytes))
		}
		pieces, err := ParseInstructions(instrBytes, ptrSize, typeByteSize)
		if err != nil {
			return Loclist{}, fmt.Errorf("failed to parse instructions: %w", err)
		}
		locations = append(locations, ir.Location{Range: ir.PCRange{lo + pcBase, hi + pcBase}, Pieces: pieces})
	}
	return Loclist{Locations: locations, Default: nil}, nil
}

// DWARF DW_LLE_* opcodes
// https://dwarfstd.org/doc/DWARF5.pdf
//
//revive:disable:var-naming
const (
	dw_lle_end_of_list      = 0x00
	dw_lle_base_addressx    = 0x01
	dw_lle_startx_endx      = 0x02
	dw_lle_startx_length    = 0x03
	dw_lle_offset_pair      = 0x04
	dw_lle_default_location = 0x05
	dw_lle_base_address     = 0x06
	dw_lle_start_end        = 0x07
	dw_lle_start_length     = 0x08
)

//revive:enable:var-naming

func readDwarf5(data []byte, ptrSize uint8, typeByteSize uint32, debugAddr []byte) (Loclist, error) {
	reader := bytes.NewBuffer(data)
	var locations []ir.Location
	var defaultPieces []ir.Piece

	readInstr := func(reader *bytes.Buffer) ([]ir.Piece, error) {
		instrLen, err := readULEB128(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read instruction length: %w", err)
		}
		instrBytes := reader.Next(int(instrLen))
		if len(instrBytes) != int(instrLen) {
			return nil, fmt.Errorf("not enough bytes for instructions: expected %d, got %d", instrLen, len(instrBytes))
		}
		return ParseInstructions(instrBytes, ptrSize, typeByteSize)
	}

	readAndAppendInstr := func(reader *bytes.Buffer, lo uint64, hi uint64) error {
		pieces, err := readInstr(reader)
		if err != nil {
			return err
		}
		locations = append(locations, ir.Location{Range: ir.PCRange{lo, hi}, Pieces: pieces})
		return nil
	}

	base := uint64(0)
loop:
	for {
		op, err := reader.ReadByte()
		if err != nil {
			return Loclist{}, fmt.Errorf("failed to read opcode: %w", err)
		}
		switch op {
		case dw_lle_end_of_list:
			break loop

		case dw_lle_base_addressx:
			addrIdx, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read base address: %w", err)
			}
			base, err = readDebugAddr(debugAddr, ptrSize, addrIdx)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read base value: %w", err)
			}

		case dw_lle_startx_endx:
			loAddrIdx, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read low address: %w", err)
			}
			lo, err := readDebugAddr(debugAddr, ptrSize, loAddrIdx)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read low value: %w", err)
			}
			hiAddrIdx, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read high address: %w", err)
			}
			hi, err := readDebugAddr(debugAddr, ptrSize, hiAddrIdx)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read high value: %w", err)
			}
			err = readAndAppendInstr(reader, lo, hi)
			if err != nil {
				return Loclist{}, err
			}

		case dw_lle_startx_length:
			loAddrIdx, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read low address: %w", err)
			}
			lo, err := readDebugAddr(debugAddr, ptrSize, loAddrIdx)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read low value: %w", err)
			}
			length, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read length: %w", err)
			}
			err = readAndAppendInstr(reader, lo, lo+length)
			if err != nil {
				return Loclist{}, err
			}

		case dw_lle_offset_pair:
			loOffset, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read low offset: %w", err)
			}
			hiOffset, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read high offset: %w", err)
			}
			err = readAndAppendInstr(reader, base+loOffset, base+hiOffset)
			if err != nil {
				return Loclist{}, err
			}

		case dw_lle_default_location:
			defaultPieces, err = readInstr(reader)
			if err != nil {
				return Loclist{}, err
			}

		case dw_lle_base_address:
			base, err = readFixedSize(reader, ptrSize)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read base address: %w", err)
			}

		case dw_lle_start_end:
			lo, err := readFixedSize(reader, ptrSize)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read low offset: %w", err)
			}
			hi, err := readFixedSize(reader, ptrSize)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read high offset: %w", err)
			}
			err = readAndAppendInstr(reader, lo, hi)
			if err != nil {
				return Loclist{}, err
			}

		case dw_lle_start_length:
			lo, err := readFixedSize(reader, ptrSize)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read low offset: %w", err)
			}
			length, err := readULEB128(reader)
			if err != nil {
				return Loclist{}, fmt.Errorf("failed to read length: %w", err)
			}
			err = readAndAppendInstr(reader, lo, lo+length)
			if err != nil {
				return Loclist{}, err
			}

		default:
			return Loclist{}, fmt.Errorf("unknown lle opcode: 0x%x", op)
		}
	}
	return Loclist{Locations: locations, Default: defaultPieces}, nil
}

func readDebugAddr(data []byte, ptrSize uint8, idx uint64) (uint64, error) {
	offset := idx * uint64(ptrSize)
	if offset+uint64(ptrSize) > uint64(len(data)) {
		return 0, fmt.Errorf("address index %d out of bounds", idx)
	}
	return binary.LittleEndian.Uint64(data[offset : offset+uint64(ptrSize)]), nil
}
