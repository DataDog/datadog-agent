// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// This file uses the DWARF and cilium/ebpf libraries to build a source map for an eBPF object file.
// This map links each instruction in the program to the source line in the original C code. This task
// is more complex than it'd looks like due to the complexity of the DWARF format and the way eBPF programs
// are compiled and stored in the ELF file.
//
// In the DWARF format, the source line information is stored in the .debug_lines section. The contents
// of that section (or the result of interpreting that section, because the binary format is just a list of
// instructions that generate the data, not the data itself; luckily for us that's done by the DWARF library)
// is a list of line entries, each of them with a file, line number, and address. The address represents the offset
// of the instruction in the program.
//
// Then, we have the .debug_info section which contains all the information about symbols, types, and others. We
// are interested in the Subprogram objects (functions), which have a name and a lowpc attribute. The lowpc attribute
// is the address of the first instruction of the function in the .debug_lines section.
//
// Now, the main problem is that those addresses are not unique, but are just offsets relative to the start of the sequence, which is a
// concept I couldn't find a definition for. But from reading the standard, it seems that a sequence is an ELF section with
// executable code. So, in order to have a translation from the .debug_lines addresses to eBPF program + instruction index, we
// need to do the following>
// 1. Build a map of symbols (functions) to the sequence they belong to (implemented in buildSymbolToSequenceMap based on ELF data)
// 2. Read the .debug_info section to get the start address of each program, and use the previous map to append the sequence index. With that we
//    build a map (sequence, address) -> program name.
// 3. Read the .debug_lines section, tracking the sequence index. Each sequence is a monotonically
//    increasing sequence of addresses, so we track restarts of the sequence to increase the sequence index.
//    For each line, we check if that tuple (sequence, address) points to a program start. If so, then every subsequent line
//    belongs to that program, until we find a new program start. The corresponding assembly instruction index is the offset of the line
//    relative to the start of the program. With this information we build a map of (program name, instruction index) -> source line.
// 4. Now we just iterate through all the instructions in each eBPF program using cilium/ebpf and assign the corresponding source line
//    to each instruction. We use this extra library as, in some cases, it provides the source line information for the instruction from
//    BTF data, which can be helpful to validate the correctness of the DWARF data.

package verifier

import (
	"debug/dwarf"
	"debug/elf"
	"errors"
	"fmt"
	"io"

	"github.com/cilium/ebpf"
)

// getLineReader gets the line reader for a DWARF data object, searching in the compilation unit entry
func getLineReader(dwarfData *dwarf.Data) (*dwarf.LineReader, error) {
	entryReader := dwarfData.Reader()
	if entryReader == nil {
		return nil, errors.New("cannot get dwarf reader")
	}

	for {
		entry, err := entryReader.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to iterate DWARF entries: %w", err)
		}
		if entry == nil {
			break
		}

		if entry.Tag == dwarf.TagCompileUnit {
			lineReader, err := dwarfData.LineReader(entry)
			if err != nil {
				return nil, fmt.Errorf("cannot instantiate line reader: %w", err)
			}
			return lineReader, nil
		}
	}
	return nil, fmt.Errorf("no line reader found in DWARF data")
}

// progStartPoint defines a possible start point for a program: section index + address
type progStartPoint struct {
	sequenceIndex int
	addr          int64
}

// buildProgStartMap builds a map of DWARF line information points that are the start of a program.
// This is used to know which program a line belongs to, as the DWARF line info data doesn't have that information.
func buildProgStartMap(dwarfData *dwarf.Data, symToSeq map[string]int) (map[progStartPoint]string, error) {
	progStartLines := make(map[progStartPoint]string)
	entryReader := dwarfData.Reader()
	if entryReader == nil {
		return nil, fmt.Errorf("cannot get dwarf reader")
	}

	for {
		entry, err := entryReader.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to iterate DWARF entries: %w", err)
		}
		if entry == nil {
			break
		}

		if entry.Tag != dwarf.TagSubprogram || len(entry.Field) == 0 {
			continue
		}

		// Find the program name and program start address in its sequence
		progStartAddr := int64(-1)
		progName := ""
		for _, field := range entry.Field {
			if field.Attr == dwarf.AttrName {
				progName = field.Val.(string)
			} else if field.Attr == dwarf.AttrLowpc {
				progStartAddr = int64(field.Val.(uint64))
			}
		}
		if progName == "" || progStartAddr == -1 {
			continue // Ignore if we don't have all the fields
		}

		seqIndex, ok := symToSeq[progName]
		if !ok {
			return nil, fmt.Errorf("cannot find sequence for symbol %s", progName)
		}
		startPoint := progStartPoint{seqIndex, progStartAddr}
		progStartLines[startPoint] = progName
	}

	return progStartLines, nil
}

// buildSymbolToSequenceMap builds a map that links each symbol to the sequence index it belongs to.
// The address in the DWARF debug_line section is relative to the start of each sequence, but the symbol information
// doesn't explicitly say which sequence it belongs to. This function builds that map.
func buildSymbolToSequenceMap(elfFile *elf.File) (map[string]int, error) {
	symbols, err := elfFile.Symbols()
	if err != nil {
		return nil, fmt.Errorf("failed to read symbols from ELF file: %w", err)
	}

	// Each sequence is a section, unless that section has no content
	sectIndexToSeqIndex := make(map[int]int)
	idx := 0
	for i, sect := range elfFile.Sections {
		if sect.Flags&elf.SHF_EXECINSTR != 0 && sect.Size > 0 {
			sectIndexToSeqIndex[i] = idx
			idx++
		}
	}

	symToSeq := make(map[string]int)
	for _, sym := range symbols {
		sectIndex := int(sym.Section)
		if sectIndex >= 0 && sectIndex < len(elfFile.Sections) {
			symToSeq[sym.Name] = sectIndexToSeqIndex[int(sectIndex)]
		}
	}

	return symToSeq, nil
}

// getSourceMap builds the source map for an eBPF program. It returns two maps, one that
// for each program function maps the instruction offset to the source line information, and
// another that for each section maps the functions that belong to it.
func getSourceMap(file string, spec *ebpf.CollectionSpec) (map[string]map[int]*SourceLine, map[string][]string, error) {
	// Open the ELF file
	elfFile, err := elf.Open(file)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open ELF file %s: %w", file, err)
	}
	defer elfFile.Close()

	// Read the debug information for line data. The Go DWARF reader fails when reading eBPF
	// files because of missing support for relocations. However, we don't need them here as we're
	// not necessary for line info, so we can skip them. The DWARF library will skip that processing
	// if we set manually the type of the file to ET_EXEC.
	elfFile.Type = elf.ET_EXEC
	dwarfData, err := elfFile.DWARF()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read DWARF data for %s: %w", file, err)
	}
	entryReader := dwarfData.Reader()
	if entryReader == nil {
		return nil, nil, fmt.Errorf("cannot get dwarf reader for %s: %w", file, err)
	}

	// Get the reader for the .debug_lines section
	lineReader, err := getLineReader(dwarfData)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get line reader for %s: %w", file, err)
	}

	// Build the map that links each symbol to the sequence index it belongs to
	symToSeq, err := buildSymbolToSequenceMap(elfFile)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot build symbol to section index map for %s: %w", file, err)
	}

	// Build the map that links the sequence index + address to the start of each program, as the DWARF
	// line info data doesn't tell you which program a line belongs to.
	progStartMap, err := buildProgStartMap(dwarfData, symToSeq)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot build program start lines for %s: %w", file, err)
	}

	// Now build the map that, for each program, links the instruction offset to line information
	// Read all the lines in the .debug_lines section
	offsets := make(map[string]map[uint64]string)
	currProgram := ""
	startingOffset := uint64(0)
	sequenceIndex := 0
	prevAddress := 0
	for {
		var line dwarf.LineEntry
		err := lineReader.Next(&line)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("DWARF lineReader file %s: %w", file, err)
		}

		// Ignore lines with no data
		if line.File == nil || line.Line == 0 {
			continue
		}

		// Increase section indexes whenever we reset the program. We have to look at the value of the previous
		// address, because we might have multiple lines that have the same zero address.
		if line.Address == 0 && prevAddress != 0 {
			sequenceIndex++
		}
		prevAddress = int(line.Address)

		startPoint := progStartPoint{sequenceIndex, int64(line.Address)}
		lineinfo := fmt.Sprintf("%s:%d", line.File.Name, line.Line)

		// Reset the current program only if it's the first time we see it. Multiple
		// assembly instructions might point to the first source line of a program
		if newProg, ok := progStartMap[startPoint]; ok && newProg != currProgram {
			// We need to keep track of the starting offset for each program to calculate
			// the offset relative to the start. The eBPF loaders count program instructions
			// from the start of the program, while in the ELF binary they're relative to the
			// section start, and we might have multiple functions per section.
			startingOffset = line.Address
			currProgram = progStartMap[startPoint]
		}

		if currProgram == "" {
			// We might have information of programs that are not in the spec, ignore those
			continue
		}
		if _, ok := offsets[currProgram]; !ok {
			offsets[currProgram] = make(map[uint64]string)
		}
		offset := line.Address - startingOffset
		offsets[currProgram][offset] = lineinfo
	}

	// Now that we have line information for each instruction, we can build the source map
	sourceMap := make(map[string]map[int]*SourceLine)
	funcsPerSection := make(map[string][]string)
	currLineInfo := ""
	currLine := ""
	for _, progSpec := range spec.Programs {
		sourceMap[progSpec.Name] = make(map[int]*SourceLine)
		funcsPerSection[progSpec.SectionName] = append(funcsPerSection[progSpec.SectionName], progSpec.Name)

		iter := progSpec.Instructions.Iterate()
		for iter.Next() {
			ins := iter.Ins
			insOffset := iter.Offset.Bytes()
			insIdx := int(insOffset / 8) // Use the instruction offset in bytes as the index, because that's what the verifier uses
			if _, ok := offsets[progSpec.Name][insOffset]; ok {
				// A single C line can generate multiple instructions, only update the value
				// if we have a new source line
				currLineInfo = offsets[progSpec.Name][insOffset]
				currLine = ""
			} else if insIdx == 0 {
				return nil, nil, fmt.Errorf("missing line information at initial instruction for program %s", progSpec.Name)
			}
			// Keep the last source line for the instruction if we don't have a new one
			if ins.Source() != nil && ins.Source().String() != "" {
				currLine = ins.Source().String()
			}

			sline := SourceLine{LineInfo: currLineInfo, Line: currLine}
			sourceMap[progSpec.Name][insIdx] = &sline
		}
	}

	return sourceMap, funcsPerSection, nil
}
