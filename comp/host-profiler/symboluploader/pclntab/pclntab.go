// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package pclntab provides utilities for parsing and extracting Go pclntab data.
package pclntab

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	elf "github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

type HeaderVersion int

const (
	verUnknown HeaderVersion = iota
	ver12
	ver116
	ver118
	ver120
)

const (
	ptrSize           = 8
	maxBytesGoPclntab = 128 * 1024 * 1024
	maxGoFuncID       = 22

	funcdataInlTree = 3

	// pclntabHeader magic identifying Go version
	magicGo1_2  = 0xfffffffb
	magicGo1_16 = 0xfffffffa
	magicGo1_18 = 0xfffffff0
	magicGo1_20 = 0xfffffff1
)

var (
	disableRecover = false
)

type GoPCLnTabInfo struct {
	Address    uint64        // goPCLnTab address
	Data       []byte        // goPCLnTab data
	Version    HeaderVersion // gopclntab header version
	Offsets    TableOffsets
	TextStart  TextStartInfo // text start information
	GoFuncAddr uint64        // goFunc address
	GoFuncData []byte        // goFunc data

	numFuncs            int
	funcSize            int
	fieldSize           int
	funcNpcdataOffset   int
	funcNfuncdataOffset int
	functab             []byte
	funcdata            []byte
}

type TableOffsets struct {
	FuncNameTabOffset uint64
	CuTabOffset       uint64
	FileTabOffset     uint64
	PcTabOffset       uint64
	FuncTabOffset     uint64
}

type pclntabHeader struct {
	// magic is one of the magicGo1_xx constants identifying the version
	magic uint32
	// pad is unused and is needed for alignment
	pad uint16
	// quantum is the CPU instruction size alignment (e.g. 1 for x86, 4 for arm)
	quantum uint8
	// ptrSize is the CPU pointer size in bytes
	ptrSize uint8
	// numFuncs is the number of function definitions to follow
	numFuncs uint64
}

type pclntabHeader116 struct {
	pclntabHeader

	nfiles         uint
	funcnameOffset uintptr
	cuOffset       uintptr
	filetabOffset  uintptr
	pctabOffset    uintptr
	pclnOffset     uintptr
}

// pclntabHeader118 is the Golang pclntab header structure starting Go 1.18
// structural definition of this is found in go/src/runtime/symtab.go as pcHeader
type pclntabHeader118 struct {
	pclntabHeader

	nfiles         uint
	textStart      uintptr
	funcnameOffset uintptr
	cuOffset       uintptr
	filetabOffset  uintptr
	pctabOffset    uintptr
	pclnOffset     uintptr
}

type rawInlinedCall112 struct {
	Parent   int16 // index of parent in the inltree, or < 0
	FuncID   uint8 // type of the called function
	padding  byte
	File     int32 // perCU file index for inlined call. See cmd/link:pcln.go
	Line     int32 // line number of the call site
	Func     int32 // offset into pclntab for name of called function
	ParentPC int32 // position of an instruction whose source position is the call site (offset from entry)
}

// rawInlinedCall120 is the encoding of entries in the FUNCDATA_InlTree table
// from Go 1.20. It is equivalent to runtime.inlinedCall.
type rawInlinedCall120 struct {
	FuncID    uint8 // type of the called function
	padding   [3]byte
	NameOff   int32 // offset into pclntab for name of called function
	ParentPC  int32 // position of an instruction whose source position is the call site (offset from entry)
	StartLine int32 // line number of start of function (func keyword/TEXT directive)
}

type TextStartOrigin int

const (
	TextStartOriginPCLnTab TextStartOrigin = iota
	TextStartOriginModuleData
	TextStartOriginRuntimeTextSymbol
)

type TextStartInfo struct {
	Origin             TextStartOrigin
	Address            uint64
	TextSectionAddress uint64
}

func (h HeaderVersion) String() string {
	switch h {
	case ver12:
		return "1.2"
	case ver116:
		return "1.16"
	case ver118:
		return "1.18"
	case ver120:
		return "1.20"
	default:
		return "unknown"
	}
}

// getInt32 gets a 32-bit integer from the data slice at offset with bounds checking
func getInt32(data []byte, offset int) int {
	if offset < 0 || offset+4 > len(data) {
		return -1
	}
	return int(*(*int32)(unsafe.Pointer(&data[offset])))
}

func getUInt32(data []byte, offset int) int {
	if offset < 0 || offset+4 > len(data) {
		return -1
	}
	return int(*(*uint32)(unsafe.Pointer(&data[offset])))
}

func getUint8(data []byte, offset int) int {
	if offset < 0 || offset+1 > len(data) {
		return -1
	}
	return int(*(*uint8)(unsafe.Pointer(&data[offset])))
}

// pclntabHeaderSize returns the minimal pclntab header size.
func pclntabHeaderSize() int {
	return int(unsafe.Sizeof(pclntabHeader{}))
}

func sectionContaining(elfFile *pfelf.File, addr uint64) *pfelf.Section {
	for _, s := range elfFile.Sections {
		if s.Type != elf.SHT_NOBITS && addr >= s.Addr && addr < s.Addr+s.Size {
			return &s
		}
	}
	return nil
}

// goFuncOffset returns the offset of the goFunc field in moduledata.
func goFuncOffset(v HeaderVersion) (uint32, error) {
	if v < ver118 {
		return 0, fmt.Errorf("unsupported pclntab version: %v", v.String())
	}
	if v < ver120 {
		return 38 * ptrSize, nil
	}
	return 40 * ptrSize, nil
}

func FindModuleData(ef *pfelf.File, goPCLnTabInfo *GoPCLnTabInfo, runtimeFirstModuleDataSymbolValue libpf.SymbolValue) (data []byte, address uint64, returnedErr error) {
	// First: try to use the 'go.module' section introduced in Go 1.26.
	moduleSection := ef.Section(".go.module")
	if moduleSection != nil {
		data, err := moduleSection.Data(maxBytesGoPclntab)
		if err != nil {
			return nil, 0, fmt.Errorf("could not read .go.module section: %w", err)
		}
		return data, moduleSection.Addr, nil
	}

	// Second: try to locate module data by looking for runtime.firstmoduledata symbol.
	if runtimeFirstModuleDataSymbolValue != 0 {
		addr := uint64(runtimeFirstModuleDataSymbolValue)
		section := sectionContaining(ef, addr)
		if section == nil {
			return nil, 0, errors.New("could not find section containing runtime.firstmoduledata")
		}
		data, err := section.Data(maxBytesGoPclntab)
		if err != nil {
			return nil, 0, fmt.Errorf("could not read section containing runtime.firstmoduledata: %w", err)
		}
		return data[addr-section.Addr:], addr, nil
	}

	// If runtime.firstmoduledata is missing, heuristically search for gopclntab address in .noptrdata section.
	// https://www.mandiant.com/resources/blog/golang-internals-symbol-recovery
	noPtrSection := ef.Section(".noptrdata")
	noPtrSectionData, err := noPtrSection.Data(maxBytesGoPclntab)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read .noptrdata section: %w", err)
	}

	var buf [2 * ptrSize]byte
	binary.NativeEndian.PutUint64(buf[:], goPCLnTabInfo.Address)
	binary.NativeEndian.PutUint64(buf[ptrSize:], goPCLnTabInfo.Address+goPCLnTabInfo.Offsets.FuncNameTabOffset)
	for i := 0; i < len(noPtrSectionData)-19*ptrSize; i += ptrSize {
		n := bytes.Index(noPtrSectionData[i:], buf[:])
		if n < 0 {
			break
		}
		i += n

		off := i + 4*ptrSize
		cuTabAddr := binary.NativeEndian.Uint64(noPtrSectionData[off:])
		off += 3 * ptrSize
		fileTabAddr := binary.NativeEndian.Uint64(noPtrSectionData[off:])
		off += 3 * ptrSize
		pcTabAddr := binary.NativeEndian.Uint64(noPtrSectionData[off:])
		off += 6 * ptrSize
		funcTabAddr := binary.NativeEndian.Uint64(noPtrSectionData[off:])

		// Check if the offsets are valid.
		if cuTabAddr != goPCLnTabInfo.Address+goPCLnTabInfo.Offsets.CuTabOffset ||
			fileTabAddr != goPCLnTabInfo.Address+goPCLnTabInfo.Offsets.FileTabOffset ||
			pcTabAddr != goPCLnTabInfo.Address+goPCLnTabInfo.Offsets.PcTabOffset ||
			funcTabAddr != goPCLnTabInfo.Address+goPCLnTabInfo.Offsets.FuncTabOffset {
			continue
		}
		return noPtrSectionData[n:], noPtrSection.Addr + uint64(n), nil
	}

	return nil, 0, errors.New("could not find moduledata")
}

func findGoFuncEnd112(data []byte) int {
	elemSize := int(unsafe.Sizeof(rawInlinedCall112{}))
	nbElem := len(data) / elemSize
	inlineCalls := unsafe.Slice((*rawInlinedCall112)(unsafe.Pointer(&data[0])), nbElem)
	for i, ic := range inlineCalls {
		// all inlined functions FuncID seem to be 0, but to be safe we just check that it is not greater than maxGoFuncID
		if ic.padding != 0 || ic.FuncID > maxGoFuncID || ic.Line < 0 || ic.ParentPC < 0 || ic.Func < 0 {
			return i * elemSize
		}
	}
	return nbElem * elemSize
}

func findGoFuncEnd120(data []byte) int {
	elemSize := int(unsafe.Sizeof(rawInlinedCall120{}))
	nbElem := len(data) / elemSize
	inlineCalls := unsafe.Slice((*rawInlinedCall120)(unsafe.Pointer(&data[0])), nbElem)
	for i, ic := range inlineCalls {
		// all inlined functions FuncID seem to be 0, but to be safe we just check that it is not greater than maxGoFuncID
		if ic.padding[0] != 0 || ic.padding[1] != 0 || ic.padding[2] != 0 || ic.FuncID > maxGoFuncID || ic.StartLine < 0 || ic.ParentPC < 0 || ic.NameOff < 0 {
			return i * elemSize
		}
	}
	return nbElem * elemSize
}

// Determine heuristically the end of go func data.
func findGoFuncEnd(data []byte, version HeaderVersion) int {
	if version < ver120 {
		return findGoFuncEnd112(data)
	}
	return findGoFuncEnd120(data)
}

func findGoFuncVal(ef *pfelf.File, goPCLnTabInfo *GoPCLnTabInfo, runtimeFirstModuleDataSymbolValue libpf.SymbolValue) (uint64, error) {
	moduleData, _, err := FindModuleData(ef, goPCLnTabInfo, runtimeFirstModuleDataSymbolValue)
	if err != nil {
		return 0, fmt.Errorf("could not find module data: %w", err)
	}
	goFuncOff, err := goFuncOffset(goPCLnTabInfo.Version)
	if err != nil {
		return 0, fmt.Errorf("could not get go func offset: %w", err)
	}
	if goFuncOff+ptrSize >= uint32(len(moduleData)) {
		return 0, fmt.Errorf("invalid go func offset: %v", goFuncOff)
	}
	goFuncVal := binary.NativeEndian.Uint64(moduleData[goFuncOff:])

	return goFuncVal, nil
}

func FindGoFunc(ef *pfelf.File, goPCLnTabInfo *GoPCLnTabInfo, runtimeFirstModuleDataSymbolValue, goFuncSymbolValue libpf.SymbolValue) (data []byte, goFuncVal uint64, err error) {
	if goFuncSymbolValue == 0 {
		goFuncVal, err = findGoFuncVal(ef, goPCLnTabInfo, runtimeFirstModuleDataSymbolValue)
		if err != nil {
			return nil, 0, fmt.Errorf("could not find go func value: %w", err)
		}
	} else {
		// Symbol go:func.* or go.func.* is present, use it to get the goFunc value.
		goFuncVal = uint64(goFuncSymbolValue)
	}
	sec := sectionContaining(ef, goFuncVal)
	if sec == nil {
		return nil, 0, errors.New("could not find section containing gofunc")
	}
	secData, err := sec.Data(maxBytesGoPclntab)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read section containing gofunc: %w", err)
	}
	return secData[goFuncVal-sec.Addr:], goFuncVal, nil
}

func pclntabHeaderSignature(arch elf.Machine) []byte {
	var quantum byte

	switch arch {
	case elf.EM_X86_64:
		quantum = 0x1
	case elf.EM_AARCH64:
		quantum = 0x4
	}

	//  - the first byte is ignored and not included in this signature
	//    as it is different per Go version (see magicGo1_XX)
	//  - next three bytes are 0xff (shared on magicGo1_XX)
	//  - pad is zero (two bytes)
	//  - quantum depends on the architecture
	//  - ptrSize is 8 for 64 bit systems (arm64 and amd64)

	return []byte{0xff, 0xff, 0xff, 0x00, 0x00, quantum, 0x08}
}

func SearchGoPclntab(ef *pfelf.File) (data []byte, address uint64, err error) {
	signature := pclntabHeaderSignature(ef.Machine)

	for i := range ef.Progs {
		p := ef.Progs[i]
		// Search for the .rodata (read-only) and .data.rel.ro (read-write which gets
		// turned into read-only after relocations handling via GNU_RELRO header).
		if p.Type != elf.PT_LOAD || p.Flags&elf.PF_X == elf.PF_X || p.Flags&elf.PF_R != elf.PF_R {
			continue
		}

		// Skip segments that are too small anyway.
		if p.Filesz < uint64(pclntabHeaderSize()) {
			continue
		}

		data, err = p.Data(maxBytesGoPclntab)
		if err != nil {
			return nil, 0, err
		}

		for i := 1; i < len(data)-pclntabHeaderSize(); i += 8 {
			// Search for something looking like a valid pclntabHeader header
			// Ignore the first byte on bytes.Index (differs on magicGo1_XXX)
			n := bytes.Index(data[i:], signature)
			if n < 0 {
				break
			}
			i += n - 1

			// Check the 'magic' against supported list, and if valid, use this
			// location as the .gopclntab base. Otherwise, continue just search
			// for next candidate location.
			magic := binary.NativeEndian.Uint32(data[i:])
			switch magic {
			case magicGo1_2, magicGo1_16, magicGo1_18, magicGo1_20:
				return data[i:], uint64(i) + p.Vaddr, nil
			}
		}
	}

	return nil, 0, errors.New("could not find .gopclntab with signature search")
}

func (g *GoPCLnTabInfo) findMaxInlineTreeOffset() int {
	maxInlineTreeOffset := -1
	for i := range g.numFuncs {
		funcIdx := (2*i + 1) * g.fieldSize
		funcOff := getUInt32(g.functab, funcIdx)
		if funcOff == -1 {
			continue
		}
		nfuncdata := getUint8(g.funcdata, funcOff+g.funcNfuncdataOffset)
		npcdata := getUInt32(g.funcdata, funcOff+g.funcNpcdataOffset)
		if nfuncdata != -1 && npcdata != -1 && nfuncdata > funcdataInlTree {
			off := funcOff + g.funcSize + (npcdata+funcdataInlTree)*4
			inlineTreeOffset := getUInt32(g.funcdata, off)
			if inlineTreeOffset != int(^uint32(0)) && inlineTreeOffset > maxInlineTreeOffset {
				maxInlineTreeOffset = inlineTreeOffset
			}
		}
	}

	return maxInlineTreeOffset
}

func alignUp(v, align int) int {
	return (v + align - 1) &^ (align - 1)
}

func (g *GoPCLnTabInfo) findFuncDataSize() int {
	maxFuncOffset := -1
	maxFuncOffsetIdx := -1
	for i := range g.numFuncs {
		funcIdx := (2*i + 1) * g.fieldSize
		funcOff := getUInt32(g.functab, funcIdx)
		if funcOff > maxFuncOffset {
			maxFuncOffset = funcOff
			maxFuncOffsetIdx = i
		}
	}

	if maxFuncOffsetIdx == -1 {
		return -1
	}

	nfuncdata := getUint8(g.funcdata, maxFuncOffset+g.funcNfuncdataOffset)
	npcdata := getUInt32(g.funcdata, maxFuncOffset+g.funcNpcdataOffset)
	return maxFuncOffset + g.funcSize + npcdata*4 + nfuncdata*g.fieldSize
}

func (g *GoPCLnTabInfo) computePCLnTabSize() int {
	if g.Version < ver116 {
		return -1
	}

	funcDataSize := g.findFuncDataSize()
	if funcDataSize == -1 {
		return -1
	}
	return alignUp(int(g.Offsets.FuncTabOffset)+funcDataSize, ptrSize)
}

func (g *GoPCLnTabInfo) trimGoFunc(goFuncData []byte, goFuncAddr uint64, runtimeGcbitsSymbolValue libpf.SymbolValue) ([]byte, error) {
	if runtimeGcbitsSymbolValue != 0 {
		// symbol runtime.gcbits.* follows goFunc, use it to determine the end of goFunc.
		if uint64(runtimeGcbitsSymbolValue) > goFuncAddr {
			dist := uint64(runtimeGcbitsSymbolValue) - goFuncAddr
			if dist < uint64(len(goFuncData)) {
				return goFuncData[:dist], nil
			}
		}
	}

	// Iterate over the functions to find the maximum offset of the inline tree.
	maxInlineTreeOffset := g.findMaxInlineTreeOffset()

	if maxInlineTreeOffset == -1 || maxInlineTreeOffset >= len(goFuncData) {
		return nil, fmt.Errorf("invalid inline tree offset: %v", maxInlineTreeOffset)
	}

	// maxInlineTreeOffset is the base address of the inlined calls for a function
	// find the end of the goFunc by finding heuristically the end of the last inlined call.
	goFuncEndOffset := findGoFuncEnd(goFuncData[maxInlineTreeOffset:], g.Version)
	goFuncSize := maxInlineTreeOffset + goFuncEndOffset

	// Starting from Go 1.19, Go linker appears to align some symbols to ptrSize bytes.
	// https://github.com/golang/go/commit/c2c76c6f198480f3c9aece4aa5d9b8de044d8457
	// Cannot check explicitly for Go 1.19 as the version is not available in the pclntab header.
	if g.Version >= ver118 {
		goFuncSize = alignUp(goFuncSize, ptrSize)
		// Just to be safe (and for Go 1.18)
		goFuncSize = min(goFuncSize, len(goFuncData))
	}

	return goFuncData[:goFuncSize], nil
}

func parseGoPCLnTab(data []byte) (*GoPCLnTabInfo, error) {
	var version HeaderVersion
	var offsets TableOffsets
	var funcSize, funcNpcdataOffset int
	var textStart uint64
	hdrSize := uintptr(pclntabHeaderSize())

	dataLen := uintptr(len(data))
	if dataLen < hdrSize {
		return nil, fmt.Errorf(".gopclntab is too short (%v)", len(data))
	}
	var functab, funcdata []byte

	hdr := (*pclntabHeader)(unsafe.Pointer(&data[0]))
	fieldSize := int(hdr.ptrSize)
	switch hdr.magic {
	case magicGo1_2:
		version = ver12
		funcSize = ptrSize + 8*4
		funcNpcdataOffset = ptrSize + 6*4
		mapSize := uintptr(2 * ptrSize)
		functabEnd := int(hdrSize + uintptr(hdr.numFuncs)*mapSize + uintptr(hdr.ptrSize))
		filetabOffset := getInt32(data, functabEnd)
		numSourceFiles := getInt32(data, filetabOffset)
		if filetabOffset == 0 || numSourceFiles == 0 {
			return nil, fmt.Errorf(".gopclntab corrupt (filetab 0x%x, nfiles %d)",
				filetabOffset, numSourceFiles)
		}
		functab = data[hdrSize:filetabOffset]
		funcdata = data
		offsets = TableOffsets{
			FuncTabOffset: uint64(hdrSize),
			CuTabOffset:   uint64(filetabOffset),
		}
	case magicGo1_16:
		version = ver116
		hdrSize = unsafe.Sizeof(pclntabHeader116{})
		funcSize = ptrSize + 9*4
		funcNpcdataOffset = ptrSize + 6*4
		if dataLen < hdrSize {
			return nil, fmt.Errorf(".gopclntab is too short (%v)", len(data))
		}
		hdr116 := (*pclntabHeader116)(unsafe.Pointer(&data[0]))
		if dataLen < hdr116.funcnameOffset || dataLen < hdr116.cuOffset ||
			dataLen < hdr116.filetabOffset || dataLen < hdr116.pctabOffset ||
			dataLen < hdr116.pclnOffset {
			return nil, fmt.Errorf(".gopclntab is corrupt (%x, %x, %x, %x, %x)",
				hdr116.funcnameOffset, hdr116.cuOffset,
				hdr116.filetabOffset, hdr116.pctabOffset,
				hdr116.pclnOffset)
		}
		functab = data[hdr116.pclnOffset:]
		funcdata = functab
		offsets = TableOffsets{
			FuncNameTabOffset: uint64(hdr116.funcnameOffset),
			CuTabOffset:       uint64(hdr116.cuOffset),
			FileTabOffset:     uint64(hdr116.filetabOffset),
			PcTabOffset:       uint64(hdr116.pctabOffset),
			FuncTabOffset:     uint64(hdr116.pclnOffset),
		}
	case magicGo1_18, magicGo1_20:
		if hdr.magic == magicGo1_20 {
			version = ver120
			funcSize = 11 * 4
		} else {
			version = ver118
			funcSize = 10 * 4
		}
		funcNpcdataOffset = 7 * 4
		hdrSize = unsafe.Sizeof(pclntabHeader118{})
		if dataLen < hdrSize {
			return nil, fmt.Errorf(".gopclntab is too short (%v)", dataLen)
		}
		hdr118 := (*pclntabHeader118)(unsafe.Pointer(&data[0]))
		if dataLen < hdr118.funcnameOffset || dataLen < hdr118.cuOffset ||
			dataLen < hdr118.filetabOffset || dataLen < hdr118.pctabOffset ||
			dataLen < hdr118.pclnOffset {
			return nil, fmt.Errorf(".gopclntab is corrupt (%x, %x, %x, %x, %x)",
				hdr118.funcnameOffset, hdr118.cuOffset,
				hdr118.filetabOffset, hdr118.pctabOffset,
				hdr118.pclnOffset)
		}
		functab = data[hdr118.pclnOffset:]
		funcdata = functab
		fieldSize = 4
		offsets = TableOffsets{
			FuncNameTabOffset: uint64(hdr118.funcnameOffset),
			CuTabOffset:       uint64(hdr118.cuOffset),
			FileTabOffset:     uint64(hdr118.filetabOffset),
			PcTabOffset:       uint64(hdr118.pctabOffset),
			FuncTabOffset:     uint64(hdr118.pclnOffset),
		}
		textStart = uint64(hdr118.textStart)
	default:
		return nil, fmt.Errorf(".gopclntab format (0x%x) not supported", hdr.magic)
	}
	if hdr.pad != 0 || hdr.ptrSize != ptrSize {
		return nil, fmt.Errorf(".gopclntab header: %x, %x", hdr.pad, hdr.ptrSize)
	}

	// nfuncdata is the last field in _func struct
	funcNfuncdataOffset := funcSize - 1

	return &GoPCLnTabInfo{
		Data:    data,
		Version: version,
		Offsets: offsets,
		TextStart: TextStartInfo{
			Origin:  TextStartOriginPCLnTab,
			Address: textStart,
		},
		numFuncs:            int(hdr.numFuncs),
		funcSize:            funcSize,
		fieldSize:           fieldSize,
		funcNpcdataOffset:   funcNpcdataOffset,
		funcNfuncdataOffset: funcNfuncdataOffset,
		functab:             functab,
		funcdata:            funcdata,
	}, nil
}

func DisableRecoverFromPanic() {
	disableRecover = true
}

func FindGoPCLnTab(ef *pfelf.File) (goPCLnTabInfo *GoPCLnTabInfo, err error) {
	return findGoPCLnTab(ef, false)
}

func FindGoPCLnTabWithChecks(ef *pfelf.File) (goPCLnTabInfo *GoPCLnTabInfo, err error) {
	return findGoPCLnTab(ef, true)
}

func findGoPCLnTab(ef *pfelf.File, additionalChecks bool) (goPCLnTabInfo *GoPCLnTabInfo, err error) {
	if !disableRecover {
		// gopclntab parsing code might panic if the data is corrupt.
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic while searching pclntab: %v", r)
			}
		}()
	}

	var data []byte
	var goPCLnTabAddr uint64

	goPCLnTabEndKnown := true
	if s := ef.Section(".gopclntab"); s != nil {
		if data, err = s.Data(maxBytesGoPclntab); err != nil {
			return nil, fmt.Errorf("failed to load .gopclntab: %w", err)
		}
		goPCLnTabAddr = s.Addr
	} else if s := ef.Section(".data.rel.ro.gopclntab"); s != nil {
		if data, err = s.Data(maxBytesGoPclntab); err != nil {
			return nil, fmt.Errorf("failed to load .data.rel.ro.gopclntab: %w", err)
		}
		goPCLnTabAddr = s.Addr
	} else if s := ef.Section(".go.buildinfo"); s != nil {
		var start, end libpf.SymbolValue
		_ = ef.VisitSymbols(func(sym libpf.Symbol) bool {
			if start == 0 && sym.Name == "runtime.pclntab" {
				start = sym.Address
			} else if end == 0 && sym.Name == "runtime.epclntab" {
				end = sym.Address
			}
			return start == 0 || end == 0
		})
		if start == 0 || end == 0 {
			// It seems the Go binary was stripped, use the heuristic approach to get find gopclntab.
			// Note that `SearchGoPclntab` returns a slice starting from gopcltab header to the end of segment
			// containing gopclntab. Therefore this slice might contain additional data after gopclntab.
			// There does not seem to be an easy way to get the end of gopclntab segment without parsing the
			// gopclntab itself.
			if data, goPCLnTabAddr, err = SearchGoPclntab(ef); err != nil {
				return nil, fmt.Errorf("failed to search .gopclntab: %w", err)
			}
			// Truncate the data to the end of the section containing gopclntab.
			if sec := sectionContaining(ef, goPCLnTabAddr); sec != nil {
				data = data[:sec.Addr+sec.Size-goPCLnTabAddr]
			}
			goPCLnTabEndKnown = false
		} else {
			if start >= end {
				return nil, fmt.Errorf("invalid .gopclntab symbols: %v-%v", start, end)
			}
			data, err = ef.VirtualMemory(int64(start), int(end-start), maxBytesGoPclntab)
			if err != nil {
				return nil, fmt.Errorf("failed to load .gopclntab via symbols: %w", err)
			}
			goPCLnTabAddr = uint64(start)
		}
	}

	if data == nil {
		return nil, errors.New("file does not contain any of the gopclntab expected sections")
	}

	goPCLnTabInfo, err = parseGoPCLnTab(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse .gopclntab: %w", err)
	}

	if goPCLnTabInfo.TextStart.Address == 0 && goPCLnTabInfo.Version >= ver118 {
		// Starting from Go 1.26, textStart address in pclntab is always set to 0.
		// Therefore we need to get it from either `runtime.text` symbol or moduledata.
		// Note that it does not always match the address of `.text` section
		// (for example with cgo binaries or when built with -linkmode=external).
		textStartInfo, err := findTextStart(ef)
		if err != nil {
			return nil, fmt.Errorf("failed to find text start: %w", err)
		}
		textSection := ef.Section(".text")
		if textSection != nil {
			textStartInfo.TextSectionAddress = textSection.Addr
		}
		goPCLnTabInfo.TextStart = textStartInfo
	}
	goPCLnTabInfo.Address = goPCLnTabAddr

	// Only search for goFunc if the version is 1.18 or later and heuristic search is enabled.
	if goPCLnTabInfo.Version >= ver118 {
		var runtimeFirstModuleDataSymbolValue, goFuncSymbolValue, runtimeGcbitsSymbolValue libpf.SymbolValue
		// Retrieve the address of some symbols that can be used to find the goFunc start/end.
		// Do it once here to avoid calling VisitSymbols multiple times for each symbol.
		_ = ef.VisitSymbols(func(sym libpf.Symbol) bool {
			switch {
			case runtimeFirstModuleDataSymbolValue == 0 && sym.Name == "runtime.firstmoduledata":
				runtimeFirstModuleDataSymbolValue = sym.Address
			// Depending on the Go version, the symbol name may be "go:func.*" or "go.func.*".
			case goFuncSymbolValue == 0 && (sym.Name == "go:func.*" || sym.Name == "go.func.*"):
				goFuncSymbolValue = sym.Address
			case runtimeGcbitsSymbolValue == 0 && sym.Name == "runtime.gcbits.*":
				runtimeGcbitsSymbolValue = sym.Address
			}
			return runtimeFirstModuleDataSymbolValue == 0 || goFuncSymbolValue == 0 || runtimeGcbitsSymbolValue == 0
		})

		goFuncData, goFuncAddr, err := FindGoFunc(ef, goPCLnTabInfo, runtimeFirstModuleDataSymbolValue, goFuncSymbolValue)
		if err == nil {
			// Starting from Go 1.26, goFunc is stored in gopclntab, so if we know the end of gopclntab, we don't need to trim goFunc.
			if goPCLnTabEndKnown && goFuncAddr > goPCLnTabInfo.Address && goFuncAddr < goPCLnTabInfo.Address+uint64(len(goPCLnTabInfo.Data)) {
				goPCLnTabInfo.GoFuncAddr = goFuncAddr
				// Do not set goPCLnTabInfo.GoFuncData since goFunc is already in gopclntab.
				return goPCLnTabInfo, nil
			}
			goFuncData, err = goPCLnTabInfo.trimGoFunc(goFuncData, goFuncAddr, runtimeGcbitsSymbolValue)
			if err == nil {
				goPCLnTabInfo.GoFuncAddr = goFuncAddr
				goPCLnTabInfo.GoFuncData = goFuncData
			} else if additionalChecks {
				// if we failed to trim goFunc, return an error only if additionalChecks is enabled
				// otherwise discard goFunc and continue.
				// goFunc in this case is discarded because goFunc is in .rodata section that may contain
				// sensitive information past the end of the goFunc.
				return nil, fmt.Errorf("failed to trim goFunc: %w", err)
			}
		}
	}

	if (!goPCLnTabEndKnown || additionalChecks) && goPCLnTabInfo.Version >= ver116 {
		// Do not try to find the size of the .gopclntab for older versions because pclntab layout is different.
		// Failure to find the size of the .gopclntab is not critical because when gopclntab does not have
		// its own section, it is stored in .data.rel.ro  most likely does not contain sensitive information.
		// This happens with buildmode=pie and cgo or external linkmode because in that case gopclntab is in
		// .data.rel.ro.gopclntab which is then merged with .data.rel.ro section by system linker
		// (that does not happen if there is no cgo or external linkmode because in that case Go uses its own
		// internal linker which does not merge the .data.rel.ro* sections).
		//
		// Starting from Go 1.26, .gopclntab has no more relocation and therefore stays in its own .gopclntab section even with buildmode=pie.
		goPCLnTabSize := goPCLnTabInfo.computePCLnTabSize()
		if additionalChecks && goPCLnTabEndKnown {
			// check that the computed size matches the known size
			if len(goPCLnTabInfo.Data) != goPCLnTabSize {
				return nil, fmt.Errorf("invalid computed .gopclntab size: %v (computed) vs %v (expected)", goPCLnTabSize, len(goPCLnTabInfo.Data))
			}
		}

		if goPCLnTabSize != -1 && goPCLnTabSize < len(goPCLnTabInfo.Data) {
			goPCLnTabInfo.Data = goPCLnTabInfo.Data[:goPCLnTabSize]
		}
	}

	return goPCLnTabInfo, nil
}

func findTextStartFromModuleData(ef *pfelf.File) (uint64, error) {
	// Starting from Go 1.26, moduledata has its own `.go.module` section.
	moduleDataSection := ef.Section(".go.module")
	if moduleDataSection == nil || moduleDataSection.Type == elf.SHT_NOBITS {
		// Stop here and do not try to locate moduledata with other means because this function is expected to be called only for Go 1.26+ binaries
		// and for those cases moduledata is always in .go.module section.
		return 0, errors.New("could not find .go.module section")
	}
	const textStartOff = 22 * ptrSize
	var textStartBytes [ptrSize]byte
	_, err := moduleDataSection.ReadAt(textStartBytes[:], textStartOff)
	if err != nil {
		return 0, fmt.Errorf("could not read .go.module section at offset %v: %w", textStartOff, err)
	}

	return binary.NativeEndian.Uint64(textStartBytes[:]), nil
}

func findTextStart(ef *pfelf.File) (TextStartInfo, error) {
	// Use moduledata to find textstart, since it's more efficient than iterating over symbols.
	textStart, err := findTextStartFromModuleData(ef)
	if err == nil {
		return TextStartInfo{
			Origin:  TextStartOriginModuleData,
			Address: textStart,
		}, nil
	}

	// Use `runtime.text` symbol to find textstart, this is only for tests where moduledata is not available.
	_ = ef.VisitSymbols(func(sym libpf.Symbol) bool {
		if sym.Name == "runtime.text" {
			textStart = uint64(sym.Address)
			return false
		}
		return true
	})

	if textStart != 0 {
		return TextStartInfo{
			Origin:  TextStartOriginRuntimeTextSymbol,
			Address: textStart,
		}, nil
	}

	return TextStartInfo{}, errors.New("could not find text start")
}
