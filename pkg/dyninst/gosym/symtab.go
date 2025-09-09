// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package gosym provides SymbolTable and PcLnTable utilities. It mimics
// the debug/gosym package, but it handles inlined functions, and is more performant.
// Better performance is achieved by operating on references to the original byte
// data, instead of making copies for each parsed structure.
package gosym

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"iter"
	"math"
	"slices"
)

// GoFunction represents function information stored in the pclntab.
type GoFunction struct {
	// The function's entry point
	Entry uint64
	// The function's end address
	End uint64
	// The function's deferreturn address, if any
	DeferReturn uint32
	// The function name, lazily populated.
	name string
	// The function name offset.
	nameOff nameOff
	// The index of the function in the symbol table
	idx uint32
	// The internal function info
	funcInfo funcInfo
}

// Name returns the function name.
func (f *GoFunction) Name() string {
	if f.name != "" || f.nameOff == math.MaxUint32 {
		return f.name
	}
	f.name = f.funcInfo.lt.funcName(uint32(f.nameOff))
	return f.name
}

// HasName returns true if the function name is equal to the given string.
func (f *GoFunction) HasName(s string) bool {
	if f.nameOff == math.MaxUint32 {
		return false
	}
	if f.name != "" {
		return f.name == s
	}
	return bytes.Equal(f.funcInfo.lt.funcNameData(uint32(f.nameOff)), []byte(s))
}

// GoLocation represents a resolved source code location.
type GoLocation struct {
	// The function name.
	Function string
	// The source file
	File string
	// The source line number
	Line uint32
}

// GoSymbolTable represents a Go symbol table consisting of function symbols.
type GoSymbolTable struct {
	pclntab lineTable
	gofunc  []byte
}

// ParseGoSymbolTable parses the Go symbol table from an object file.
func ParseGoSymbolTable(
	pclntabData []byte, goFuncData []byte,
	textStart, textEnd, minPC, maxPC uint64,
) (*GoSymbolTable, error) {
	textRange := [2]uint64{textStart, textEnd}
	pcRange := [2]uint64{minPC, maxPC}

	lineTable, err := parselineTable(pclntabData, textRange, pcRange)
	if err != nil {
		return nil, fmt.Errorf("failed to parse line table: %w", err)
	}
	st := &GoSymbolTable{
		pclntab: *lineTable,
		gofunc:  goFuncData,
	}

	// Default the wrapperFuncID to the maximum value, which is not a valid
	// function ID. Search the symbol table for the magic function that is known
	// to be marked as a wrapper function. If we find it, set the wrapper
	// function ID to the actual value. If we don't find it, then we'll end up
	// not actually considering any function to be a wrapper function.
	const invalidWrapperFuncID = math.MaxUint8
	const magicFuncName = "runtime.deferreturn"
	st.pclntab.wrapperFuncID = invalidWrapperFuncID
	for f := range st.Functions() {
		if f.HasName(magicFuncName) {
			if fid, ok := f.funcInfo.funcID(); ok {
				st.pclntab.wrapperFuncID = fid
			}
			break
		}
	}

	return st, nil
}

// CollectFunctions collects all the functions in the symbol table.
func (gst *GoSymbolTable) CollectFunctions() []GoFunction {
	res := make([]GoFunction, 0, gst.pclntab.nfunctab)
	return slices.AppendSeq(res, gst.Functions())
}

// Functions returns an iterator over the functions in the symbol table.
func (gst *GoSymbolTable) Functions() iter.Seq[GoFunction] {
	return func(yield func(v GoFunction) bool) {
		for i := range gst.pclntab.nfunctab {
			if fn, ok := gst.pclntab.getGoFunctionByIndex(i); ok {
				if !yield(fn) {
					return
				}
			}
		}
	}
}

// PCToFunction returns the function that contains the given PC. Inlined
// functions are not returned, only the functions that remain in the object file.
func (gst *GoSymbolTable) PCToFunction(pc uint64) *GoFunction {
	fn, ok := gst.pclntab.getGoFunctionByPC(pc)
	if !ok {
		return nil
	}
	return &fn
}

// LocatePC returns the location that contains the given PC.
func (gst *GoSymbolTable) LocatePC(pc uint64) []GoLocation {
	return gst.locatePC(pc)
}

// LineRange represents a range of PC that map to the same line number.
type LineRange struct {
	PCLo uint64
	PCHi uint64
	Line uint32
}

// FunctionLines represents the lines of a function. Line ranges are non-overlapping
// and sorted by the PCLo. Line number values may repeat across different pc ranges.
// PC ranges only include instructions that directly belong to the function, inlined
// pc ranges are not included.
type FunctionLines struct {
	File  string
	Lines []LineRange
}

// FunctionLines resolves a physical (non-inlined) function at a given pc,
// and returns a mapping from all function names (self and inlined) to their lines.
func (gst *GoSymbolTable) FunctionLines(pc uint64) (map[string]FunctionLines, error) {
	return gst.functionLines(pc)
}

// Constants for pclntab parsing
const (
	pcdataInlTreeIndex = 2
	funcdataInlTree    = 3

	go12Magic  = 0xfffffffb
	go116Magic = 0xfffffffa
	go118Magic = 0xfffffff0
	go120Magic = 0xfffffff1

	// Inline call structure constants
	inlinedCallSize       = 16
	funcIDOffset          = 0
	functionNameOffOffset = 4
	parentPCOffset        = 8
)

// pclnTabVersion represents the supported pclntab versions.
type pclnTabVersion int

// pclnTabVersion constants
const (
	ver12 pclnTabVersion = iota
	ver116
	ver118
	ver120
)

// lineTable represents a parsed representation of the Go pclntab.
type lineTable struct {
	// The entire pclntab data
	data []byte
	// The pclntab version
	version pclnTabVersion
	// The "pc quantum" (from byte 6)
	quantum uint32
	// The pointer size (from byte 7)
	ptrSize int
	// The number of function entries
	nfunctab uint32
	// The number of file entries
	nfiletab uint32

	// Following fields are offset ranges that map to corresponding tables.

	// The offset of the file table
	filetab [2]int
	// The offset of the function table
	functab [2]int
	// The blob of function metadata
	funcdata [2]int
	// The function name table
	funcnametab [2]int
	// The compile unit table
	cutab *[2]int
	// The pc table
	pcTab [2]int
	// For ver118/120, the text start address (used to relocate PCs); otherwise zeros
	textRange [2]uint64
	// The range of PCs that can be symbolicated
	pcRange [2]uint64

	// The FuncID of wrapper functions
	wrapperFuncID uint8
}

func parselineTable(data []byte, textRange, pcRange [2]uint64) (*lineTable, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("pclntab too short")
	}

	magic := binary.LittleEndian.Uint32(data[0:4])

	var version pclnTabVersion
	switch magic {
	case go12Magic:
		version = ver12
	case go116Magic:
		version = ver116
	case go118Magic:
		version = ver118
	case go120Magic:
		version = ver120
	default:
		return nil, fmt.Errorf("unsupported pclntab magic: %x", magic)
	}

	// Check pad bytes
	if data[4] != 0 || data[5] != 0 {
		return nil, fmt.Errorf("unexpected pclntab header bytes")
	}

	quantum := uint32(data[6])
	ptrSize := int(data[7])

	if ptrSize != 4 && ptrSize != 8 {
		return nil, fmt.Errorf("invalid pointer size in pclntab: %d", ptrSize)
	}

	readOffset := func(word uint32) (uint64, error) {
		start := 8 + int(word)*ptrSize
		if len(data) < start+ptrSize {
			return 0, fmt.Errorf("pclntab too short for offset word %d", word)
		}
		if ptrSize == 8 {
			return binary.LittleEndian.Uint64(data[start : start+8]), nil
		}
		return uint64(binary.LittleEndian.Uint32(data[start : start+4])), nil
	}

	switch version {
	// --- Go 1.18 and Go 1.20 ---
	case ver118, ver120:
		nfunctab64, err := readOffset(0)
		if err != nil {
			return nil, err
		}
		nfunctab := uint32(nfunctab64)

		nfiletab64, err := readOffset(1)
		if err != nil {
			return nil, err
		}
		nfiletab := uint32(nfiletab64)

		off3, err := readOffset(3)
		if err != nil {
			return nil, err
		}
		if int(off3) >= len(data) {
			return nil, fmt.Errorf("invalid funcnametab offset")
		}
		funcnametab := [2]int{int(off3), len(data)}

		off4, err := readOffset(4)
		if err != nil {
			return nil, err
		}
		if int(off4) >= len(data) {
			return nil, fmt.Errorf("invalid cutab offset")
		}
		cutab := [2]int{int(off4), len(data)}

		off5, err := readOffset(5)
		if err != nil {
			return nil, err
		}
		if int(off5) >= len(data) {
			return nil, fmt.Errorf("invalid filetab offset")
		}
		filetab := [2]int{int(off5), len(data)}

		off6, err := readOffset(6)
		if err != nil {
			return nil, err
		}
		if int(off6) >= len(data) {
			return nil, fmt.Errorf("invalid pc_tab offset")
		}
		pcTab := [2]int{int(off6), len(data)}

		off7, err := readOffset(7)
		if err != nil {
			return nil, err
		}
		if int(off7) >= len(data) {
			return nil, fmt.Errorf("invalid funcdata offset")
		}
		base := int(off7)
		fieldSize := 4 // For ver118 and later, functab fields are 4 bytes
		required := (int(nfunctab)*2 + 1) * fieldSize
		if len(data) < base+required {
			return nil, fmt.Errorf("pclntab too short for functab data")
		}
		functab := [2]int{base, base + required}
		funcdata := [2]int{base, len(data)}

		return &lineTable{
			data:        data,
			version:     version,
			quantum:     quantum,
			ptrSize:     ptrSize,
			nfunctab:    nfunctab,
			nfiletab:    nfiletab,
			functab:     functab,
			funcdata:    funcdata,
			funcnametab: funcnametab,
			filetab:     filetab,
			pcTab:       pcTab,
			cutab:       &cutab,
			textRange:   textRange,
			pcRange:     pcRange,
		}, nil

	// --- Go 1.16 ---
	case ver116:
		nfunctab64, err := readOffset(0)
		if err != nil {
			return nil, err
		}
		nfunctab := uint32(nfunctab64)

		nfiletab64, err := readOffset(1)
		if err != nil {
			return nil, err
		}
		nfiletab := uint32(nfiletab64)

		off2, err := readOffset(2)
		if err != nil {
			return nil, err
		}
		if int(off2) >= len(data) {
			return nil, fmt.Errorf("invalid funcnametab offset")
		}
		funcnametab := [2]int{int(off2), len(data)}

		off3, err := readOffset(3)
		if err != nil {
			return nil, err
		}
		if int(off3) >= len(data) {
			return nil, fmt.Errorf("invalid cutab offset")
		}
		cutab := [2]int{int(off3), len(data)}

		off4, err := readOffset(4)
		if err != nil {
			return nil, err
		}
		if int(off4) >= len(data) {
			return nil, fmt.Errorf("invalid filetab offset")
		}
		filetab := [2]int{int(off4), len(data)}

		off5, err := readOffset(5)
		if err != nil {
			return nil, err
		}
		if int(off5) >= len(data) {
			return nil, fmt.Errorf("invalid pc_tab offset")
		}
		pcTab := [2]int{int(off5), len(data)}

		off6, err := readOffset(6)
		if err != nil {
			return nil, err
		}
		if int(off6) >= len(data) {
			return nil, fmt.Errorf("invalid funcdata offset")
		}

		base := int(off6)
		fieldSize := functabFieldSize(ptrSize, version)
		functabSize := (int(nfunctab)*2 + 1) * fieldSize
		if len(data) < base+functabSize {
			return nil, fmt.Errorf("pclntab too short for functab data")
		}
		functab := [2]int{base, base + functabSize}
		funcdata := [2]int{base, len(data)}

		return &lineTable{
			data:        data,
			version:     version,
			quantum:     quantum,
			ptrSize:     ptrSize,
			nfunctab:    nfunctab,
			nfiletab:    nfiletab,
			functab:     functab,
			funcdata:    funcdata,
			funcnametab: funcnametab,
			filetab:     filetab,
			pcTab:       pcTab,
			cutab:       &cutab,
			textRange:   textRange,
			pcRange:     pcRange,
		}, nil

	// --- Go 1.2 ---
	case ver12:
		var nfunctab uint32
		if ptrSize == 8 {
			if len(data) < 8+8 {
				return nil, fmt.Errorf("pclntab too short for nfunctab")
			}
			nfunctab = uint32(binary.LittleEndian.Uint64(data[8 : 8+8]))
		} else {
			if len(data) < 8+4 {
				return nil, fmt.Errorf("pclntab too short for nfunctab")
			}
			nfunctab = binary.LittleEndian.Uint32(data[8 : 8+4])
		}

		functabOffset := 8 + ptrSize
		functabSize := (int(nfunctab)*2 + 1) * ptrSize
		if len(data) < functabOffset+functabSize {
			return nil, fmt.Errorf("pclntab too short for functab")
		}
		functab := [2]int{functabOffset, functabOffset + functabSize}

		if len(data) < functab[1]+4 {
			return nil, fmt.Errorf("pclntab too short for filetab offset")
		}
		filetabOffset := binary.LittleEndian.Uint32(data[functab[1] : functab[1]+4])

		if int(filetabOffset)+4 > len(data) {
			return nil, fmt.Errorf("filetab offset out of bounds")
		}
		nfiletab := binary.LittleEndian.Uint32(data[filetabOffset : filetabOffset+4])

		funcdata := [2]int{0, len(data)}
		funcnametab := [2]int{0, len(data)}
		filetab := [2]int{int(filetabOffset), int(filetabOffset) + int(nfiletab)*4}
		pcTab := [2]int{0, len(data)}

		return &lineTable{
			data:        data,
			version:     version,
			quantum:     quantum,
			ptrSize:     ptrSize,
			nfunctab:    nfunctab,
			nfiletab:    nfiletab,
			functab:     functab,
			funcdata:    funcdata,
			funcnametab: funcnametab,
			filetab:     filetab,
			pcTab:       pcTab,
			cutab:       nil,
			textRange:   textRange,
			pcRange:     pcRange,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported pclntab version")
	}
}

func functabFieldSize(ptrSize int, version pclnTabVersion) int {
	switch version {
	case ver118, ver120:
		return 4
	default:
		return ptrSize
	}
}

func (lt *lineTable) getGoFunctionByIndex(idx uint32) (GoFunction, bool) {
	if idx >= lt.nfunctab {
		return GoFunction{}, false
	}

	funcTab := lt.funcTab()
	pc, err := funcTab.pc(idx)
	if err != nil {
		return GoFunction{}, false
	}

	var end uint64
	if idx+1 < lt.nfunctab {
		end, err = funcTab.pc(idx + 1)
		if err != nil {
			return GoFunction{}, false
		}
	} else {
		end = lt.textRange[1]
	}

	funcInfo, err := lt.funcInfo(idx)
	if err != nil {
		return GoFunction{}, false
	}

	nameOff, ok := funcInfo.nameOff()
	if !ok {
		nameOff = math.MaxUint32
	}
	deferReturn := uint32(0)
	if dr, found := funcInfo.deferReturn(); found {
		deferReturn = dr
	}

	return GoFunction{
		Entry:       pc,
		End:         end,
		DeferReturn: deferReturn,
		nameOff:     nameOff,
		idx:         idx,
		funcInfo:    funcInfo,
	}, true
}

func (lt *lineTable) getGoFunctionByPC(pc uint64) (GoFunction, bool) {
	idx, found := lt.findFunc(pc)
	if !found {
		return GoFunction{}, false
	}
	return lt.getGoFunctionByIndex(idx)
}

func (lt *lineTable) funcTab() *funcTab {
	return (*funcTab)(lt)
}

func (lt *lineTable) funcInfo(i uint32) (funcInfo, error) {
	funcTab := lt.funcTab()
	funcOff, err := funcTab.funcOff(i)
	if err != nil {
		return funcInfo{}, err
	}

	actualOffset := lt.funcdata[0] + int(funcOff)
	if actualOffset >= len(lt.data) {
		return funcInfo{}, fmt.Errorf("function offset out of bounds")
	}

	return funcInfo{
		lt:   lt,
		data: lt.data[actualOffset:],
	}, nil
}

func (lt *lineTable) funcName(off uint32) string {
	return string(lt.funcNameData(off))
}

func (lt *lineTable) funcNameData(off uint32) []byte {
	offset := lt.funcnametab[0] + int(off)
	if offset >= lt.funcnametab[1] {
		return nil
	}

	return stringDataFromOffset(lt.data, offset)
}

func (gst *GoSymbolTable) locatePC(pc uint64) []GoLocation {
	lt := gst.pclntab
	function, ok := lt.getGoFunctionByPC(pc)
	if !ok {
		return nil
	}

	adjustedPC := pc
	if pc+1 == function.End {
		adjustedPC = pc - 1
	}

	makeLocation := func(idx uint32, pc uint64, functionName string) GoLocation {
		file := lt.pcToFile(idx, pc)
		line, found := lt.pcToLine(idx, pc)

		if !found {
			line = 0
		}

		return GoLocation{
			Function: functionName,
			File:     file,
			Line:     line,
		}
	}

	inlinedCalls := unwindInlinedCalls(function.funcInfo, adjustedPC, gst.gofunc)
	if inlinedCalls == nil {
		return []GoLocation{makeLocation(function.idx, adjustedPC, function.Name())}
	}

	var locations []GoLocation
	for _, call := range inlinedCalls {
		funcID := call.FuncID
		if funcID == nil {
			if fid, found := function.funcInfo.funcID(); found {
				funcID = &fid
			}
		}

		// Skip wrapper functions
		if funcID != nil && *funcID == lt.wrapperFuncID {
			continue
		}

		var functionName string
		if call.Function != nil {
			functionName = *call.Function
		} else {
			functionName = function.Name()
		}

		locations = append(locations, makeLocation(function.idx, call.PC, functionName))
	}

	return locations
}

func readInlTree(inlineTree []byte, fIdx uint32) (funcID uint8, nameOff uint32, parentPC uint32, ok bool) {
	offset := inlinedCallSize * int(fIdx)
	if offset+parentPCOffset+4 > len(inlineTree) {
		return 0, 0, 0, false
	}
	funcID = inlineTree[offset+funcIDOffset]
	nameOff = binary.LittleEndian.Uint32(inlineTree[offset+functionNameOffOffset:])
	parentPC = binary.LittleEndian.Uint32(inlineTree[offset+parentPCOffset:])
	ok = true
	return
}

type fileRange struct {
	pcLo uint64
	pcHi uint64
	file string
}

// Parse file lookup table to produce mapping from PC ranges to file names.
// Returned ranges are non-overlapping and sorted by pcLo.
func (lt *lineTable) fileRanges(f *GoFunction) ([]fileRange, error) {
	offset, found := f.funcInfo.pcfile()
	if !found {
		return nil, fmt.Errorf("no file data for function %s", f.Name())
	}
	offset += uint32(lt.pcTab[0])
	if int(offset) >= len(lt.data) {
		return nil, fmt.Errorf("file data offset out of range for function %s", f.Name())
	}

	names := map[uint32]string{}
	pc := f.Entry
	var ranges []fileRange
	for file := range iterPCValues(lt.data[offset:], pc, lt.quantum) {
		fno := uint32(file.Val)
		var name string
		var ok bool
		if name, ok = names[fno]; !ok {
			name = lt.fileName(&f.funcInfo, fno)
			names[fno] = name
		}
		ranges = append(ranges, fileRange{
			pcLo: pc,
			pcHi: file.PC,
			file: name,
		})
		pc = file.PC
	}
	return ranges, nil
}

type functionRange struct {
	pcLo uint64
	pcHi uint64
	name string
	file string
}

// Parse inlined tree lookup table for function f, to produce mapping from its PC ranges
// to a function inlined to that range, or f itself otherwise.
// Returned ranges are non-overlapping and sorted by pcLo.
func (gst *GoSymbolTable) inlinedFunctionsMapping(f *GoFunction) []functionRange {
	lt := gst.pclntab
	inlineTree := f.funcInfo.funcData(funcdataInlTree, gst.gofunc)
	if inlineTree == nil {
		return nil
	}

	offset, found := f.funcInfo.pcDataStart(pcdataInlTreeIndex)
	if !found {
		return nil
	}
	offset += uint32(lt.pcTab[0])
	if int(offset) >= len(lt.data) {
		return nil
	}

	files, err := lt.fileRanges(f)
	if err != nil {
		return nil
	}
	fileIdx := 0

	functionNames := map[uint32]string{}
	pc := f.Entry
	var ranges []functionRange
	for inlinedFuncIdx := range iterPCValues(lt.data[offset:], pc, lt.quantum) {
		// [pc, index.PC) maps to inline function data at index.Val.
		var functionName string
		if inlinedFuncIdx.Val < 0 {
			// This range doesn't correspond to any inlined function,
			// it belongs to f itself.
			functionName = f.Name()
		} else {
			funcID, nameOff, _, found := readInlTree(inlineTree, uint32(inlinedFuncIdx.Val))
			if !found || funcID == lt.wrapperFuncID {
				pc = inlinedFuncIdx.PC
				continue
			}
			var ok bool
			if functionName, ok = functionNames[nameOff]; !ok {
				functionName = lt.funcName(nameOff)
				functionNames[nameOff] = functionName
			}
		}
		// Find the file that covers the beginning of the current PC range.
		// It must be the same for the whole function range.
		for fileIdx < len(files) && pc >= files[fileIdx].pcHi {
			fileIdx++
		}
		file := unknownFile
		if fileIdx < len(files) && files[fileIdx].pcLo <= pc {
			file = files[fileIdx].file
		}
		ranges = append(ranges, functionRange{
			pcLo: pc,
			pcHi: inlinedFuncIdx.PC,
			name: functionName,
			file: file,
		})
		pc = inlinedFuncIdx.PC
	}
	return ranges
}

func (gst *GoSymbolTable) functionLines(pc uint64) (map[string]FunctionLines, error) {
	lt := gst.pclntab
	f, ok := lt.getGoFunctionByPC(pc)
	if !ok {
		return nil, fmt.Errorf("no function for PC 0x%x", pc)
	}

	// Collect pc range -> function name/file mapping
	funcs := gst.inlinedFunctionsMapping(&f)
	if funcs == nil {
		funcs = []functionRange{
			{
				pcLo: 0,
				pcHi: math.MaxUint64,
				name: f.Name(),
				file: lt.pcToFile(f.idx, f.Entry),
			},
		}
	}

	offset, found := f.funcInfo.pcln()
	if !found {
		return nil, fmt.Errorf("no line data for function %s", f.Name())
	}
	offset += uint32(lt.pcTab[0])
	if int(offset) >= len(lt.data) {
		return nil, fmt.Errorf("line data offset out of range for function %s", f.Name())
	}

	res := make(map[string]FunctionLines, len(funcs))
	upsert := func(name string, file string, lr LineRange) {
		fl := res[name]
		fl.File = file
		fl.Lines = append(fl.Lines, lr)
		res[name] = fl
	}

	// Iterate over lines, while tracking which functions they belong to.
	pc = f.Entry
	funcIdx := 0
	for line := range iterPCValues(lt.data[offset:], pc, lt.quantum) {
		// [pc, index.PC) maps line.Val.
		// Skip function ranges that end before current line range.
		for funcIdx < len(funcs) && pc >= funcs[funcIdx].pcHi {
			funcIdx++
		}
		// Iterate over function ranges that are fully covered by the current line range.
		for funcIdx < len(funcs) && line.PC >= funcs[funcIdx].pcHi {
			upsert(funcs[funcIdx].name, funcs[funcIdx].file, LineRange{
				PCLo: max(pc, funcs[funcIdx].pcLo),
				PCHi: funcs[funcIdx].pcHi,
				Line: uint32(line.Val),
			})
			funcIdx++
		}
		// Check if the next function range starts within the current line range.
		if funcIdx < len(funcs) && pc >= funcs[funcIdx].pcLo {
			upsert(funcs[funcIdx].name, funcs[funcIdx].file, LineRange{
				PCLo: max(pc, funcs[funcIdx].pcLo),
				PCHi: line.PC,
				Line: uint32(line.Val),
			})
		}
		pc = line.PC
	}

	return res, nil
}

func (lt *lineTable) findFunc(pc uint64) (uint32, bool) {
	if pc < lt.pcRange[0] || pc >= lt.pcRange[1] {
		return 0, false
	}

	funcTab := lt.funcTab()
	count := funcTab.count()

	// Search for the first function that has a PC greater than the target PC.
	left := uint32(0)
	right := count
	for left < right {
		mid := (left + right) / 2
		midPC, err := funcTab.pc(mid)
		if err != nil {
			return 0, false
		}

		if pc < midPC {
			right = mid
		} else {
			left = mid + 1
		}
	}

	if left == 0 {
		return 0, false
	}

	return left - 1, true
}

func (lt *lineTable) pcToFile(funcIdx uint32, pc uint64) string {
	funcInfo, err := lt.funcInfo(funcIdx)
	if err != nil {
		return unknownFile
	}

	pcFile, found := funcInfo.pcfile()
	if !found {
		return unknownFile
	}

	startPC, found := funcInfo.entryPC()
	if !found {
		return unknownFile
	}

	fno, found := lt.pcValue(pcFile, startPC, pc)
	if !found {
		return unknownFile
	}

	return lt.fileName(&funcInfo, fno)
}

const unknownFile = "<unknown>"

func (lt *lineTable) fileName(funcInfo *funcInfo, fno uint32) string {
	switch lt.version {
	case ver12:
		offset := lt.funcdata[0] + lt.filetab[0] + int(fno)*4
		if offset+4 > len(lt.data) {
			return unknownFile
		}
		return stringFromOffset(lt.data, offset)

	case ver116, ver118, ver120:
		cuOffset, found := funcInfo.cuOffset()
		if !found || cuOffset == 0xFFFFFFFF {
			return unknownFile
		}

		if lt.cutab == nil {
			return unknownFile
		}

		cutabOffset := (*lt.cutab)[0] + int(cuOffset+fno)*4
		if cutabOffset+4 > len(lt.data) {
			return unknownFile
		}

		fnoff := binary.LittleEndian.Uint32(lt.data[cutabOffset:])
		if fnoff == 0xFFFFFFFF {
			return unknownFile
		}

		fileOffset := lt.filetab[0] + int(fnoff)
		if fileOffset >= len(lt.data) {
			return unknownFile
		}

		return stringFromOffset(lt.data, fileOffset)

	default:
		return unknownFile
	}
}

func (lt *lineTable) pcToLine(funcIdx uint32, pc uint64) (uint32, bool) {
	funcInfo, err := lt.funcInfo(funcIdx)
	if err != nil {
		return 0, false
	}

	pclnOff, found := funcInfo.pcln()
	if !found {
		return 0, false
	}

	entryPC, found := funcInfo.entryPC()
	if !found {
		return 0, false
	}

	return lt.pcValue(pclnOff, entryPC, pc)
}

// pcValue reports the value associated with the target pc.
func (lt *lineTable) pcValue(off uint32, entry uint64, targetPC uint64) (uint32, bool) {
	offset := lt.pcTab[0] + int(off)
	if offset >= len(lt.data) {
		return 0, false
	}

	pc := entry
	for pcVal := range iterPCValues(lt.data[offset:], pc, lt.quantum) {
		if targetPC < pcVal.PC {
			if pcVal.Val < 0 {
				return 0, false
			}
			return uint32(pcVal.Val), true
		}
	}
	return 0, false
}

// pcValue represents a program counter and its associated value from the PC line table.
// It is used when iterating through PC data to decode line numbers and other metadata.
type pcValue struct {
	// PC is the program counter address.
	PC uint64
	// Val is the associated value (line number, file index, etc.).
	Val int32
}

func iterPCValues(pcValueSeq []byte, pc uint64, quantum uint32) iter.Seq[pcValue] {
	val := int32(-1)
	return func(yield func(pcValue) bool) {
		first := true
		for {
			uvdelta, n := decodeVarint(pcValueSeq)
			if !first && uvdelta == 0 {
				return
			}
			first = false
			var vdelta int32
			if (uvdelta & 1) != 0 {
				vdelta = int32(^(uvdelta >> 1))
			} else {
				vdelta = int32(uvdelta >> 1)
			}
			pcValueSeq = pcValueSeq[n:]
			pcdelta, n := decodeVarint(pcValueSeq)
			pcValueSeq = pcValueSeq[n:]
			pc += uint64(pcdelta * quantum)
			val += vdelta
			if !yield(pcValue{PC: pc, Val: val}) {
				return
			}
		}
	}
}

func decodeVarint(buf []byte) (uint32, int) {
	var result uint32
	var shift uint
	var bytesRead int

	for i, b := range buf {
		if i >= 5 { // Maximum 5 bytes for uint32
			return 0, 0
		}

		result |= uint32(b&0x7F) << shift
		bytesRead++

		if b&0x80 == 0 {
			return result, bytesRead
		}

		shift += 7
	}

	return 0, 0
}

type inlinedCall struct {
	PC       uint64
	FuncID   *uint8
	Index    *uint32
	Function *string
}

func unwindInlinedCalls(f funcInfo, pc uint64, goFuncData []byte) []inlinedCall {
	inlineTree := f.funcData(funcdataInlTree, goFuncData)
	if inlineTree == nil {
		return nil
	}

	var inlinedCalls []inlinedCall
	entryPC, found := f.entryPC()
	if !found {
		return nil
	}

	for {
		var nextPC uint64
		if len(inlinedCalls) > 0 {
			lastCall := &inlinedCalls[len(inlinedCalls)-1]
			if lastCall.Index == nil {
				break
			}
			funcID, nameOff, parentPC, found := readInlTree(inlineTree, *lastCall.Index)
			if !found {
				break
			}
			lastCall.FuncID = &funcID
			name := f.lt.funcName(nameOff)
			lastCall.Function = &name
			nextPC = entryPC + uint64(parentPC)
		} else {
			nextPC = pc
		}

		// Get inline tree index for this PC
		index, found := f.pcValue(pcdataInlTreeIndex, nextPC)
		var indexPtr *uint32
		if found {
			indexPtr = &index
		}
		if !found && len(inlinedCalls) == 0 {
			return nil
		}

		inlinedCalls = append(inlinedCalls, inlinedCall{
			PC:       nextPC,
			Index:    indexPtr,
			Function: nil,
			FuncID:   nil,
		})
	}

	if len(inlinedCalls) == 0 {
		return nil
	}

	return inlinedCalls
}

// funcTab represents the function table portion of lineTable.
type funcTab lineTable

func (ft *funcTab) count() uint32 {
	return ft.nfunctab
}

func (ft *funcTab) pc(i uint32) (uint64, error) {
	if i >= ft.nfunctab {
		return 0, fmt.Errorf("function index out of range")
	}

	fieldSize := functabFieldSize(ft.ptrSize, ft.version)
	offset := ft.functab[0] + int(2*i)*fieldSize

	if offset+fieldSize > len(ft.data) {
		return 0, fmt.Errorf("function table entry out of bounds")
	}

	var pc uint64
	if fieldSize == 8 {
		pc = binary.LittleEndian.Uint64(ft.data[offset:])
	} else if fieldSize == 4 {
		pc = uint64(binary.LittleEndian.Uint32(ft.data[offset:]))
	} else {
		return 0, fmt.Errorf("unexpected field size: %d", fieldSize)
	}

	// For ver118/120, add text_start for relocation
	switch ft.version {
	case ver118, ver120:
		pc += ft.textRange[0]
	}

	return pc, nil
}

func (ft *funcTab) funcOff(i uint32) (uint64, error) {
	if i >= ft.nfunctab {
		return 0, fmt.Errorf("function index out of range")
	}

	fieldSize := functabFieldSize(ft.ptrSize, ft.version)
	offset := ft.functab[0] + int(2*i+1)*fieldSize

	if offset+fieldSize > len(ft.data) {
		return 0, fmt.Errorf("function offset out of bounds")
	}

	var funcOff uint64
	if fieldSize == 8 {
		funcOff = binary.LittleEndian.Uint64(ft.data[offset:])
	} else if fieldSize == 4 {
		funcOff = uint64(binary.LittleEndian.Uint32(ft.data[offset:]))
	} else {
		return 0, fmt.Errorf("unexpected field size: %d", fieldSize)
	}

	return funcOff, nil
}

// funcInfo represents single function data in the pclntab.
type funcInfo struct {
	lt *lineTable
	// data points into lt.data at the start of this function's data.
	data []byte
}

func (fi *funcInfo) field(n uint32) (uint32, bool) {
	if n == 0 || n > 9 {
		return 0, false
	}

	offset, found := fi.getFuncInfoOffset(n)
	if !found {
		return 0, false
	}

	if offset+4 > len(fi.data) {
		return 0, false
	}

	val := binary.LittleEndian.Uint32(fi.data[offset:])
	return val, true
}

func (fi *funcInfo) getFuncInfoOffset(n uint32) (int, bool) {
	var sz0 int
	if fi.lt.version == ver118 || fi.lt.version == ver120 {
		sz0 = 4
	} else {
		sz0 = fi.lt.ptrSize
	}

	offset := sz0 + int(n-1)*4
	if offset+4 > len(fi.data) {
		return 0, false
	}

	return offset, true
}

type nameOff uint32

func (fi *funcInfo) nameOff() (nameOff, bool) {
	val, found := fi.field(1)
	return nameOff(val), found
}

func (fi *funcInfo) deferReturn() (uint32, bool) {
	return fi.field(3)
}

func (fi *funcInfo) pcln() (uint32, bool) {
	return fi.field(6)
}

func (fi *funcInfo) npcdata() (uint32, bool) {
	return fi.field(7)
}

func (fi *funcInfo) nfuncdata() (uint8, bool) {
	offset, found := fi.getFuncInfoOffset(11)
	if !found {
		return 0, false
	}

	adjustedOffset := offset - 1
	if adjustedOffset < 0 || adjustedOffset >= len(fi.data) {
		return 0, false
	}

	val := fi.data[adjustedOffset]
	return val, true
}

func (fi *funcInfo) funcID() (uint8, bool) {
	offset, found := fi.getFuncInfoOffset(10)
	if !found {
		return 0, false
	}

	if offset >= len(fi.data) {
		return 0, false
	}

	val := fi.data[offset]
	return val, true
}

func (fi *funcInfo) pcfile() (uint32, bool) {
	return fi.field(5)
}

func (fi *funcInfo) entryPC() (uint64, bool) {
	switch fi.lt.version {
	case ver118, ver120:
		if len(fi.data) < 4 {
			return 0, false
		}
		u := binary.LittleEndian.Uint32(fi.data[0:4])
		result := uint64(u) + fi.lt.textRange[0]
		return result, true
	default:
		if len(fi.data) < 8 {
			return 0, false
		}
		u := binary.LittleEndian.Uint64(fi.data[0:8])
		return u, true
	}
}

func (fi funcInfo) cuOffset() (uint32, bool) {
	return fi.field(8)
}

func (fi funcInfo) funcData(i uint8, goFuncData []byte) []byte {
	nfuncdata, found := fi.nfuncdata()
	if !found || i >= nfuncdata {
		return nil
	}

	offsetBase, found := fi.getFuncInfoOffset(11)
	if !found {
		return nil
	}

	npcdata, found := fi.npcdata()
	if !found {
		return nil
	}

	funcDataOffset := offsetBase + 4*int(npcdata)
	offset := funcDataOffset + 4*int(i)

	if offset+4 > len(fi.data) {
		return nil
	}

	off := binary.LittleEndian.Uint32(fi.data[offset:])
	if off == 0xFFFFFFFF {
		return nil
	}

	if int(off) >= len(goFuncData) {
		return nil
	}

	return goFuncData[off:]
}

func (fi *funcInfo) pcDataStart(table uint8) (uint32, bool) {
	base, found := fi.getFuncInfoOffset(11)
	if !found {
		return 0, false
	}

	npcdata, found := fi.npcdata()
	if !found {
		return 0, false
	}

	if uint32(table) > npcdata {
		return 0, false
	}

	offset := base + 4*int(table)
	if offset+4 > len(fi.data) {
		return 0, false
	}

	offsetVal := binary.LittleEndian.Uint32(fi.data[offset:])
	if offsetVal == 0 {
		return 0, false
	}

	return offsetVal, true
}

func (fi *funcInfo) pcValue(table uint8, targetPC uint64) (uint32, bool) {
	offset, found := fi.pcDataStart(table)
	if !found {
		return 0, false
	}

	entry, found := fi.entryPC()
	if !found {
		return 0, false
	}

	return fi.lt.pcValue(offset, entry, targetPC)
}

func stringFromOffset(data []byte, offset int) string {
	return string(stringDataFromOffset(data, offset))
}

func stringDataFromOffset(data []byte, offset int) []byte {
	if offset >= len(data) {
		return nil
	}
	data = data[offset:]
	// Find the null terminator.
	if end := bytes.IndexByte(data, 0); end != -1 {
		data = data[:end]
	}
	return data
}
