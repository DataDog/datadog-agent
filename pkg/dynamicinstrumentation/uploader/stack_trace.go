// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package uploader

import (
	"bytes"
	"cmp"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

// parseStackTrace parses a raw byte array into 10 uint64 program counters
// which then get resolved into strings representing lines of a stack trace
func parseStackTrace(procInfo *ditypes.ProcessInfo, pcArray []byte) ([]ditypes.StackFrame, error) {
	stackTrace := make([]ditypes.StackFrame, 0)
	if procInfo == nil {
		return stackTrace, errors.New("nil process info")
	}

	rawProgramCounters := [10]uint64{}
	err := binary.Read(
		bytes.NewBuffer(pcArray[:]),
		binary.LittleEndian,
		&rawProgramCounters,
	)
	if err != nil {
		return stackTrace, fmt.Errorf("couldn't read raw stack trace bytes: %w", err)
	}

	for i := range rawProgramCounters {
		if rawProgramCounters[i] == 0 {
			break
		}

		entries, ok := procInfo.TypeMap.InlinedFunctions[rawProgramCounters[i]]
		if ok {
			for n := range entries {
				inlinedFuncInfo, err := pcToLine(procInfo, rawProgramCounters[i])
				if err != nil {
					return stackTrace, fmt.Errorf("could not resolve pc to inlined function info: %w", err)
				}

				symName, lineNumber, err := parseInlinedEntry(procInfo.DwarfData.Reader(), entries[n])
				if err != nil {
					return stackTrace, fmt.Errorf("could not get inlined entries: %w", err)
				}
				stackFrame := ditypes.StackFrame{Function: fmt.Sprintf("%s [inlined in %s]", symName, inlinedFuncInfo.fn), FileName: inlinedFuncInfo.file, Line: int(lineNumber)}
				stackTrace = append(stackTrace, stackFrame)
			}
		}

		funcInfo, err := pcToLine(procInfo, rawProgramCounters[i])
		if err != nil {
			return stackTrace, fmt.Errorf("could not resolve pc to function info: %w", err)
		}
		stackFrame := ditypes.StackFrame{Function: funcInfo.fn, FileName: funcInfo.file, Line: int(funcInfo.line)}
		stackTrace = append(stackTrace, stackFrame)

		if funcInfo.fn == "main.main" {
			break
		}
	}
	return stackTrace, nil
}

type FuncInfo struct {
	file string
	line int64
	fn   string
}

func pcToLine(procInfo *ditypes.ProcessInfo, pc uint64) (*FuncInfo, error) {

	var (
		file string
		line int64
		fn   string
	)

	typeMap := procInfo.TypeMap

	functionIndex, _ := slices.BinarySearchFunc(typeMap.FunctionsByPC, &ditypes.LowPCEntry{LowPC: pc}, func(a, b *ditypes.LowPCEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})

	var fileNumber int64

	if functionIndex >= len(typeMap.FunctionsByPC) {
		return nil, fmt.Errorf("invalid function index")
	}
	funcEntry := typeMap.FunctionsByPC[functionIndex].Entry
	for _, field := range funcEntry.Field {
		if field.Attr == dwarf.AttrName {
			fn = field.Val.(string)
		}
		if field.Attr == dwarf.AttrDeclFile {
			fileNumber = field.Val.(int64)
		}
		if field.Attr == dwarf.AttrDeclLine {
			line = field.Val.(int64)
		}
	}

	compileUnitIndex, _ := slices.BinarySearchFunc(typeMap.DeclaredFiles, &ditypes.LowPCEntry{LowPC: pc}, func(a, b *ditypes.LowPCEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})

	compileUnitEntry := typeMap.DeclaredFiles[compileUnitIndex].Entry

	cuLineReader, err := procInfo.DwarfData.LineReader(compileUnitEntry)
	if err != nil {
		return nil, fmt.Errorf("could not get file line reader for compile unit: %w", err)
	}
	files := cuLineReader.Files()
	if len(files) < int(fileNumber) {
		return nil, fmt.Errorf("invalid file number in dwarf function entry associated with compile unit")
	}

	file = files[fileNumber].Name

	return &FuncInfo{
		file: file,
		line: line,
		fn:   fn,
	}, nil
}

const MAX_BUFFER_SIZE = 10000 // TODO: find this out from configuration, maybe a special value at the begining of all events?

func parseInlinedEntry(reader *dwarf.Reader, e *dwarf.Entry) (name string, line int64, err error) {

	var offset dwarf.Offset

	for i := range e.Field {
		if e.Field[i].Attr == dwarf.AttrAbstractOrigin {
			offset = e.Field[i].Val.(dwarf.Offset)
			reader.Seek(offset)
			entry, err := reader.Next()
			if err != nil {
				return "", -1, fmt.Errorf("could not read inlined function origin: %w", err)
			}
			for j := range entry.Field {
				if entry.Field[j].Attr == dwarf.AttrName {
					name = entry.Field[j].Val.(string)
				}
			}
		}

		if e.Field[i].Attr == dwarf.AttrCallLine {
			line = e.Field[i].Val.(int64)
		}
	}

	return name, line, nil
}
