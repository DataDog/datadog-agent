// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

type version int

const (
	version125 version = iota
	version126
)

// Module data offsets
const (
	moduledataMinPCOffset  = 160
	moduledataTextOffset   = moduledataMinPCOffset + 16
	moduledataBssOffset    = moduledataTextOffset + 48
	moduledataGcdataOffset = moduledataBssOffset + 56
	moduledataTypesOffset  = moduledataGcdataOffset + 16
	moduledataGofuncOffset = moduledataTypesOffset + 24
)

func moduledataTextsectMapOffset(v version) int {
	if v == version126 {
		return moduledataGofuncOffset + 16
	}
	return moduledataGofuncOffset + 8
}

// ModuleData represents module data parsed from a Go object file
type ModuleData struct {
	// Address of the first moduledata
	FirstModuleData uint64
	// The minimum PC value that can be symbolicated
	MinPC uint64
	// The maximum PC value that can be symbolicated
	MaxPC uint64
	// Address of the text section
	Text uint64
	// The address of the end of the text section
	EText uint64
	// The address of the bss blob
	BSS uint64
	// The address of the end of the bss blob
	EBSS uint64
	// The address of the types blob
	Types uint64
	// The address of the end of the types blob
	ETypes uint64
	// The address of the go:func.* blob
	GoFunc uint64
	// The address of the runtime.gcdata symbol
	GcData uint64
	// The textsect map
	TextSectMap []TextSect
	// Information about the text section
	TextSectionInfo SectionInfo
}

// SectionInfo represents information about a section of an object file
type SectionInfo struct {
	// The file offset of the section
	FileOffset uint64
	// The address of the section
	Address uint64
}

// TextSect represents information about a text section
// https://github.com/golang/go/blob/4cc7705e56be24d5719b59cb369ce4d40643983c/src/runtime/symtab.go#L567-L571
type TextSect struct {
	// The virtual address of the section
	VAddr uint64
	// The end address of the section
	End uint64
	// The base address of the section
	BaseAddr uint64
}

// ParseModuleData parses module data from a Go object file
func ParseModuleData(obj File) (*ModuleData, error) {
	return parseModuleData(obj)
}

// GoDebugSections represents the go debug sections.
type GoDebugSections struct {
	PcLnTab SectionData
	GoFunc  SectionData
}

// Close closes the go debug sections
func (m *GoDebugSections) Close() error {
	return errors.Join(m.PcLnTab.Close(), m.GoFunc.Close())
}

// GoDebugSections returns the go debug sections
func (m *ModuleData) GoDebugSections(mef File) (_ *GoDebugSections, retErr error) {
	pclntabSection := mef.Section(".gopclntab")
	if pclntabSection == nil {
		return nil, errors.New("no pclntab section")
	}

	pclntab, err := mef.SectionDataRange(pclntabSection, 0, pclntabSection.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to load pclntab: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = pclntab.Close()
		}
	}()

	var gofunc SectionData
	if m.GoFunc != 0 && pclntabSection.Addr < m.GoFunc && pclntabSection.Addr+pclntabSection.Size > m.GoFunc {
		// Since Go 1.26 the gofunc is stored in the pclntab section.
		gofunc, err = mef.SectionDataRange(pclntabSection, m.GoFunc-pclntabSection.Addr, pclntabSection.Addr+pclntabSection.Size-m.GoFunc)
		if err != nil {
			return nil, fmt.Errorf("failed to load gofunc: %w", err)
		}
	} else if m.GoFunc != 0 && m.GcData != 0 {
		rodataSection := mef.Section(".rodata")
		if rodataSection.Addr > m.GoFunc || m.GcData > rodataSection.Addr+rodataSection.Size {
			return nil, errors.New("gofunc outside rodata section")
		}
		offset := m.GoFunc - rodataSection.Addr
		size := m.GcData - m.GoFunc
		gofunc, err = mef.SectionDataRange(rodataSection, offset, size)
		if err != nil {
			return nil, fmt.Errorf("failed to load gofunc: %w", err)
		}
	}
	defer func() {
		if err != nil {
			_ = gofunc.Close()
		}
	}()

	return &GoDebugSections{
		PcLnTab: pclntab,
		GoFunc:  gofunc,
	}, nil
}

func parseModuleData(obj SectionLoader) (*ModuleData, error) {
	pclntabSection := obj.Section(".gopclntab")
	if pclntabSection == nil {
		return nil, errors.New("no pclntab section")
	}

	noptrdataSection := obj.Section(".noptrdata")
	if noptrdataSection == nil {
		return nil, errors.New("no noptrdata section")
	}

	rodataSection := obj.Section(".rodata")
	if rodataSection == nil {
		return nil, errors.New("no rodata section")
	}

	textSection := obj.Section(".text")
	if textSection == nil {
		return nil, errors.New("no text section")
	}

	noptrdataSectionData, err := obj.SectionDataRange(noptrdataSection, 0, noptrdataSection.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to load noptrdata: %w", err)
	}
	defer noptrdataSectionData.Close()

	rodataSectionData, err := obj.SectionDataRange(rodataSection, 0, rodataSection.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to load rodata: %w", err)
	}
	defer rodataSectionData.Close()
	rodataData := rodataSectionData.Data()

	textRange := [2]uint64{textSection.Addr, textSection.Addr + textSection.Size}
	rodataRange := [2]uint64{rodataSection.Addr, rodataSection.Addr + rodataSection.Size}

	goModuleSection := obj.Section(".go.module")
	if goModuleSection != nil {
		// Since Go 1.26 the moduledata is stored in its own section.
		goModuleSectionData, err := obj.SectionDataRange(goModuleSection, 0, goModuleSection.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to load go.module: %w", err)
		}
		defer goModuleSectionData.Close()
		return tryParseModuleDataAt(
			version126,
			goModuleSectionData.Data(), rodataData, 0, textRange, rodataRange, noptrdataSection, rodataSection, textSection)
	}

	// Search for the moduledata structure
	addrBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(addrBytes, pclntabSection.Addr)

	noptrdataData := noptrdataSectionData.Data()
	offsets := findAll(noptrdataData, addrBytes)
	for _, offset := range offsets {
		// Try to parse moduledata at this offset
		moduleData, err := tryParseModuleDataAt(
			version125,
			noptrdataData, rodataData, offset, textRange, rodataRange,
			noptrdataSection, rodataSection, textSection,
		)
		if err == nil {
			return moduleData, nil
		}
	}

	return nil, errors.New("no valid moduledata found")
}

func tryParseModuleDataAt(
	version version,
	noptrdataData, rodataData []byte,
	offset int,
	textRange, rodataRange [2]uint64,
	noptrdataSection, rodataSection, textSection *safeelf.SectionHeader,
) (*ModuleData, error) {

	// Parse types range
	typesStart := offset + moduledataTypesOffset
	if typesStart+16 > len(noptrdataData) {
		return nil, errors.New("types data out of bounds")
	}
	types := binary.LittleEndian.Uint64(noptrdataData[typesStart:])
	etypes := binary.LittleEndian.Uint64(noptrdataData[typesStart+8:])

	// Parse text range
	textStart := offset + moduledataTextOffset
	if textStart+16 > len(noptrdataData) {
		return nil, errors.New("text data out of bounds")
	}
	text := binary.LittleEndian.Uint64(noptrdataData[textStart:])
	etext := binary.LittleEndian.Uint64(noptrdataData[textStart+8:])

	// Validate ranges
	if types > etypes || types < rodataRange[0] || etypes > rodataRange[1] {
		return nil, errors.New("invalid types range")
	}
	if text > etext || text < textRange[0] || etext > textRange[1] {
		return nil, errors.New("invalid text range")
	}

	// Parse textsect map
	textsectMapOffset := offset + moduledataTextsectMapOffset(version)
	if textsectMapOffset+16 > len(noptrdataData) {
		return nil, errors.New("A textsect map data out of bounds")
	}
	textsectMapPtr := binary.LittleEndian.Uint64(noptrdataData[textsectMapOffset:])
	textsectMapLen := binary.LittleEndian.Uint64(noptrdataData[textsectMapOffset+8:])

	if textsectMapPtr < rodataSection.Addr {
		return nil, errors.New("textsect map pointer out of range")
	}

	textsectSize := uint64(unsafe.Sizeof(TextSect{}))
	textsectMapDataOffset := int(textsectMapPtr - rodataSection.Addr)
	textsectMapDataLen := int(textsectMapLen * textsectSize)

	if textsectMapDataOffset < 0 || textsectMapDataOffset+textsectMapDataLen > len(rodataData) {
		return nil, errors.New("B textsect map data out of bounds")
	}

	textsectMapData := rodataData[textsectMapDataOffset : textsectMapDataOffset+textsectMapDataLen]
	var textsectMap []TextSect
	for i := 0; i < len(textsectMapData); i += int(textsectSize) {
		vaddr := binary.LittleEndian.Uint64(textsectMapData[i:])
		end := binary.LittleEndian.Uint64(textsectMapData[i+8:])
		baseaddr := binary.LittleEndian.Uint64(textsectMapData[i+16:])
		textsectMap = append(textsectMap, TextSect{
			VAddr:    vaddr,
			End:      end,
			BaseAddr: baseaddr,
		})
	}

	// Parse BSS range
	bssOffset := offset + moduledataBssOffset
	if bssOffset+16 > len(noptrdataData) {
		return nil, errors.New("bss data out of bounds")
	}
	bss := binary.LittleEndian.Uint64(noptrdataData[bssOffset:])
	ebss := binary.LittleEndian.Uint64(noptrdataData[bssOffset+8:])

	// Parse gofunc offset
	gofuncOffset := offset + moduledataGofuncOffset
	if gofuncOffset+8 > len(noptrdataData) {
		return nil, errors.New("gofunc data out of bounds")
	}
	gofunc := binary.LittleEndian.Uint64(noptrdataData[gofuncOffset:])

	// Parse gcdata offset
	gcdataOffset := offset + moduledataGcdataOffset
	if gcdataOffset+8 > len(noptrdataData) {
		return nil, errors.New("gcdata data out of bounds")
	}
	gcdata := binary.LittleEndian.Uint64(noptrdataData[gcdataOffset:])

	// Parse min/max PC
	minPCOffset := offset + moduledataMinPCOffset
	if minPCOffset+16 > len(noptrdataData) {
		return nil, errors.New("minPC data out of bounds")
	}
	minPC := binary.LittleEndian.Uint64(noptrdataData[minPCOffset:])
	maxPC := binary.LittleEndian.Uint64(noptrdataData[minPCOffset+8:])

	return &ModuleData{
		FirstModuleData: uint64(noptrdataSection.Addr) + uint64(offset),
		MinPC:           minPC,
		MaxPC:           maxPC,
		Text:            text,
		EText:           etext,
		BSS:             bss,
		EBSS:            ebss,
		Types:           types,
		ETypes:          etypes,
		GoFunc:          gofunc,
		GcData:          gcdata,
		TextSectMap:     textsectMap,
		TextSectionInfo: SectionInfo{
			FileOffset: textSection.Offset,
			Address:    textSection.Addr,
		},
	}, nil
}

// findAll finds all occurrences of needle in haystack
func findAll(haystack, needle []byte) []int {
	var offsets []int
	start := 0
	for {
		idx := bytes.Index(haystack[start:], needle)
		if idx == -1 {
			break
		}
		offsets = append(offsets, start+idx)
		start += idx + 1
	}
	return offsets
}
