// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	pclntabSectionName = ".gopclntab"

	go116magic = 0xfffffffa
	go118magic = 0xfffffff0
	go120magic = 0xfffffff1
)

// version of the pclntab
type version int

const (
	verUnknown version = iota
	ver11
	ver12
	ver116
	ver118
	ver120
)

var (
	// ErrMissingPCLNTABSection is returned when the pclntab section is missing.
	ErrMissingPCLNTABSection = errors.New("failed to find pclntab section")

	// ErrUnsupportedPCLNTABVersion is returned when the pclntab version is not supported.
	ErrUnsupportedPCLNTABVersion = errors.New("unsupported pclntab version")

	// ErrFailedToFindAllSymbols is returned when not all symbols were found.
	ErrFailedToFindAllSymbols = errors.New("failed to find all symbols")
)

// sectionAccess is a wrapper around elf.Section to provide ReadAt functionality.
// This is used to lazy read from the pclntab section, as the pclntab is large and we don't want to read it all at once,
// or store it in memory.
type sectionAccess struct {
	section    *elf.Section
	baseOffset int64
}

// ReadAt reads len(p) bytes from the section starting at the given offset.
func (s *sectionAccess) ReadAt(outBuffer []byte, offset int64) (int, error) {
	return s.section.ReadAt(outBuffer, s.baseOffset+offset)
}

// pclntanSymbolParser is a parser for pclntab symbols.
// Similar to LineTable struct in https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L43
type pclntanSymbolParser struct {
	// section is the pclntab section.
	section *elf.Section
	// symbolFilter is the filter for the symbols.
	symbolFilter symbolFilter

	// byteOrderParser is the binary.ByteOrder for the pclntab.
	byteOrderParser binary.ByteOrder
	// cachedVersion is the version of the pclntab.
	cachedVersion version
	// funcNameTable is the sectionAccess for the function name table.
	funcNameTable sectionAccess
	// funcData is the sectionAccess for the function data.
	funcData sectionAccess
	// funcTable is the sectionAccess for the function table.
	funcTable sectionAccess
	// funcTableSize is the size of the function table.
	funcTableSize uint32
	// ptrSize is the size of a pointer in the architecture of the binary.
	ptrSize uint32
	// ptrBufferSizeHelper is a buffer for reading pointers of the size ptrSize.
	ptrBufferSizeHelper []byte
	// funcNameHelper is a buffer for reading function names. Of the maximum size of the symbol names.
	funcNameHelper []byte
	// funcTableFieldSize is the size of a field in the function table.
	funcTableFieldSize int
	// funcTableBuffer is a buffer for reading fields in the function table.
	funcTableBuffer []byte
}

// GetPCLNTABSymbolParser returns the matching symbols from the pclntab section.
func GetPCLNTABSymbolParser(f *elf.File, symbolFilter symbolFilter) (map[string]*elf.Symbol, error) {
	section := f.Section(pclntabSectionName)
	if section == nil {
		return nil, ErrMissingPCLNTABSection
	}

	parser := &pclntanSymbolParser{section: section, symbolFilter: symbolFilter}

	if err := parser.parsePclntab(); err != nil {
		return nil, err
	}
	// Late initialization, to prevent allocation if the binary is not supported.
	_, maxSymbolsSize := symbolFilter.getMinMaxLength()
	// Adding additional byte for null terminator.
	parser.funcNameHelper = make([]byte, maxSymbolsSize+1)
	parser.funcTableFieldSize = getFuncTableFieldSize(parser.cachedVersion, int(parser.ptrSize))
	// Allocate the buffer for reading the function table.
	// TODO: Do we need 2*funcTableFieldSize?
	parser.funcTableBuffer = make([]byte, 2*parser.funcTableFieldSize)
	return parser.getSymbols()
}

// parsePclntab parses the pclntab, setting the version and verifying the header.
// Based on parsePclnTab in https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L194
func (p *pclntanSymbolParser) parsePclntab() error {
	p.cachedVersion = ver11

	pclntabHeader := make([]byte, 8)
	if n, err := p.section.ReadAt(pclntabHeader, 0); err != nil || n != len(pclntabHeader) {
		return fmt.Errorf("failed to read pclntab header: %w", err)
	}
	// Matching the condition https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L216-L220
	// Check header: 4-byte magic, two zeros, pc quantum, pointer size.
	if p.section.Size < 16 || pclntabHeader[4] != 0 || pclntabHeader[5] != 0 ||
		(pclntabHeader[6] != 1 && pclntabHeader[6] != 2 && pclntabHeader[6] != 4) || // pc quantum
		(pclntabHeader[7] != 4 && pclntabHeader[7] != 8) { // pointer size
		// TODO: add explicit error message
		return errors.New("invalid pclntab header")
	}

	leMagic := binary.LittleEndian.Uint32(pclntabHeader)
	beMagic := binary.BigEndian.Uint32(pclntabHeader)
	switch {
	case leMagic == go116magic:
		p.byteOrderParser, p.cachedVersion = binary.LittleEndian, ver116
	case beMagic == go116magic:
		p.byteOrderParser, p.cachedVersion = binary.BigEndian, ver116
	case leMagic == go118magic:
		p.byteOrderParser, p.cachedVersion = binary.LittleEndian, ver118
	case beMagic == go118magic:
		p.byteOrderParser, p.cachedVersion = binary.BigEndian, ver118
	case leMagic == go120magic:
		p.byteOrderParser, p.cachedVersion = binary.LittleEndian, ver120
	case beMagic == go120magic:
		p.byteOrderParser, p.cachedVersion = binary.BigEndian, ver120
	default:
		return ErrUnsupportedPCLNTABVersion
	}

	p.ptrSize = uint32(pclntabHeader[7])
	p.ptrBufferSizeHelper = make([]byte, p.ptrSize)

	// offset is based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L252
	offset := func(word uint32) uint64 {
		off := 8 + word*p.ptrSize
		if n, err := p.section.ReadAt(p.ptrBufferSizeHelper, int64(off)); err != nil || n != int(p.ptrSize) {
			return 0
		}
		return p.uintptr(p.ptrBufferSizeHelper)
	}

	switch p.cachedVersion {
	case ver118, ver120:
		p.funcTableSize = uint32(offset(0))
		p.funcNameTable = sectionAccess{
			section:    p.section,
			baseOffset: int64(offset(3)),
		}
		p.funcData = sectionAccess{
			section:    p.section,
			baseOffset: int64(offset(7)),
		}
		p.funcTable = sectionAccess{
			section:    p.section,
			baseOffset: int64(offset(7)),
		}
	case ver116:
		p.funcTableSize = uint32(offset(0))
		p.funcNameTable = sectionAccess{
			section:    p.section,
			baseOffset: int64(offset(2)),
		}
		p.funcData = sectionAccess{
			section:    p.section,
			baseOffset: int64(offset(6)),
		}
		p.funcTable = sectionAccess{
			section:    p.section,
			baseOffset: int64(offset(6)),
		}
	}

	return nil
}

// uintptr returns the pointer-sized value encoded at b.
// The pointer size is dictated by the table being read.
// based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L186.
func (p *pclntanSymbolParser) uintptr(b []byte) uint64 {
	if p.ptrSize == 4 {
		return uint64(p.byteOrderParser.Uint32(b))
	}
	return p.byteOrderParser.Uint64(b)
}

// getFuncTableFieldSize returns the size of a field in the function table.
// based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L388-L392
func getFuncTableFieldSize(version version, ptrSize int) int {
	if version >= ver118 {
		return 4
	}
	return ptrSize
}

// getSymbols returns the symbols from the pclntab section that match the symbol filter.
// based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L300-L329
func (p *pclntanSymbolParser) getSymbols() (map[string]*elf.Symbol, error) {
	numWanted := p.symbolFilter.getNumWanted()
	symbols := make(map[string]*elf.Symbol, numWanted)
	data := sectionAccess{section: p.section}
	for currentIdx := uint32(0); currentIdx < p.funcTableSize; currentIdx++ {
		// based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L315
		_, err := p.funcTable.ReadAt(p.funcTableBuffer, int64((2*currentIdx+1)*uint32(p.funcTableFieldSize)))
		if err != nil {
			continue
		}

		// based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L321
		data.baseOffset = int64(p.uint(p.funcTableBuffer)) + p.funcData.baseOffset
		funcName := p.funcName(data)

		if funcName == "" {
			continue
		}
		symbols[funcName] = &elf.Symbol{
			Name: funcName,
		}
		if len(symbols) == numWanted {
			break
		}
	}
	if len(symbols) < numWanted {
		return symbols, ErrFailedToFindAllSymbols
	}
	return symbols, nil
}

// funcName returns the name of the function found at off.
func (p *pclntanSymbolParser) funcName(data sectionAccess) string {
	off := funcNameOffset(p.ptrSize, p.cachedVersion, p.byteOrderParser, data, p.ptrBufferSizeHelper)
	n, err := p.funcNameTable.ReadAt(p.funcNameHelper, int64(off))
	if n == 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return ""
	}
	idxToNull := bytes.IndexByte(p.funcNameHelper, 0)
	if idxToNull == -1 || idxToNull == 0 || idxToNull >= n {
		return ""
	}

	if p.symbolFilter.want(string(p.funcNameHelper[:idxToNull])) {
		return string(p.funcNameHelper[:idxToNull])
	}
	return ""
}

// uint returns the uint stored at b.
// based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L427-L432
func (p *pclntanSymbolParser) uint(b []byte) uint64 {
	if p.funcTableFieldSize == 4 {
		return uint64(p.byteOrderParser.Uint32(b))
	}
	return p.byteOrderParser.Uint64(b)
}

// funcNameOffset returns the offset of the function name.
// based on https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L472-L485
// We can only for the usage of this function for getting the name of the function (https://github.com/golang/go/blob/6a861010be9eed02d5285509cbaf3fb26d2c5041/src/debug/gosym/pclntab.go#L463)
// So we explicitly set `n = 1` in the original implementation.
func funcNameOffset(ptrSize uint32, version version, binary binary.ByteOrder, data sectionAccess, helper []byte) uint32 {
	// In Go 1.18, the struct _func has changed. The original (prior to 1.18) was:
	// type _func struct {
	//    entry   uintptr
	//    nameoff int32
	//    ...
	// }
	// In Go 1.18, the struct is:
	// type _func struct {
	//    entryoff uint32
	//    nameoff  int32
	//    ...
	// }
	// Thus, to read the nameoff, for Go 1.18 and later, we need to skip the entryoff field (4 bytes).
	// for Go 1.17 and earlier, We need to skip the sizeof(uintptr) which is ptrSize.
	off := ptrSize
	if version >= ver118 {
		off = 4
	}
	// We read only 4 bytes, as the nameoff is an int32.
	if n, err := data.ReadAt(helper[:4], int64(off)); err != nil || n != 4 {
		return 0
	}
	return binary.Uint32(helper[:4])
}
