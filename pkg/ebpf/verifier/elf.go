// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

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

// buildProgStartLinesMap builds a map of program names to the source line where they start.
// This helps to build the correct offsets for the source map.
func buildProgStartLinesMap(dwarfData *dwarf.Data, lineReader *dwarf.LineReader) (map[string]string, error) {
	progStartLines := make(map[string]string)
	entryReader := dwarfData.Reader()
	if entryReader == nil {
		return nil, fmt.Errorf("cannot get dwarf reader")
	}
	files := lineReader.Files()

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

		// Find the DeclFile and DeclLine fields
		fileIndex, line := int64(0), int64(0)
		progName := ""
		for _, field := range entry.Field {
			if field.Attr == dwarf.AttrDeclFile {
				fileIndex = field.Val.(int64)
			} else if field.Attr == dwarf.AttrDeclLine {
				line = field.Val.(int64)
			} else if field.Attr == dwarf.AttrName {
				progName = field.Val.(string)
			}
		}
		if fileIndex == 0 || line == 0 || progName == "" {
			continue // Ignore if we don't have all the fields
		} else if int(fileIndex) >= len(files) {
			return nil, fmt.Errorf("file index %d out of bounds in DWARF data with %d files, func %s", fileIndex, len(files), progName)
		}
		file := files[fileIndex]
		progStartLines[fmt.Sprintf("%s:%d", file.Name, line)] = progName
	}

	return progStartLines, nil
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

	// Now read the debug information for line data. The Go DWARF reader fails when reading eBPF
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
	lineReader, err := getLineReader(dwarfData)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get line reader for %s: %w", file, err)
	}

	// Build the map that links the source line to the start of each program, as the DWARF
	// line info data doesn't tell you which program a line belongs to.
	progStartMap, err := buildProgStartLinesMap(dwarfData, lineReader)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot build program start lines for %s: %w", file, err)
	}

	// Now build the map that, for each program, links the instruction offset to line information
	offsets := make(map[string]map[uint64]string)
	currProgram := ""
	startingOffset := uint64(0)
	for {
		var line dwarf.LineEntry
		err := lineReader.Next(&line)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("DWARF lineReader file %s: %w", file, err)
		}
		if line.File != nil && line.Line > 0 {
			lineinfo := fmt.Sprintf("%s:%d", line.File.Name, line.Line)

			// Reset the current program only if it's the first time we see it. Multiple
			// assembly instructions might point to the first source line of a program
			if newProg, ok := progStartMap[lineinfo]; ok && newProg != currProgram {
				// We need to keep track of the starting offset for each program to calculate
				// the offset relative to the start. The eBPF loaders count program instructions
				// from the start of the program, while in the ELF binary they're relative to the
				// section start, and we might have multiple functions per section.
				startingOffset = line.Address
				currProgram = progStartMap[lineinfo]
			}
			if currProgram == "" {
				return nil, nil, fmt.Errorf("no program found for line %s", lineinfo)
			}
			if _, ok := offsets[currProgram]; !ok {
				offsets[currProgram] = make(map[uint64]string)
			}
			offset := line.Address - startingOffset
			offsets[currProgram][offset] = lineinfo
		}
	}

	// Now that we have line information for each instruction, we can build the source map
	sourceMap := make(map[string]map[int]*SourceLine)
	funcsPerSection := make(map[string][]string)
	currLineInfo := ""
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
			}
			sline := SourceLine{LineInfo: currLineInfo}

			if ins.Source() != nil {
				sline.Line = ins.Source().String()
			}
			sourceMap[progSpec.Name][insIdx] = &sline
		}
	}

	return sourceMap, funcsPerSection, nil
}
