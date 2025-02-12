// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"cmp"
	"debug/dwarf"
	"errors"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

// parseStackTrace parses a raw byte array into 10 uint64 program counters
// which then get resolved into strings representing lines of a stack trace
func parseStackTrace(procInfo *ditypes.ProcessInfo, rawProgramCounters []uint64) ([]ditypes.StackFrame, error) {
	stackTrace := make([]ditypes.StackFrame, 0)
	if procInfo == nil {
		return stackTrace, errors.New("nil process info")
	}

	for i := range rawProgramCounters {
		if rawProgramCounters[i] == 0 {
			break
		}

		funcInfo, err := pcToLine(procInfo, rawProgramCounters[i])
		if err != nil && len(stackTrace) == 0 {
			return stackTrace, fmt.Errorf("no stack trace: %w", err)
		} else if err != nil {
			return stackTrace, nil
		}
		stackFrame := ditypes.StackFrame{Function: funcInfo.fn, FileName: funcInfo.file, Line: int(funcInfo.line)}
		stackTrace = append(stackTrace, stackFrame)

		if funcInfo.fn == "main.main" {
			break
		}
	}
	return stackTrace, nil
}

type funcInfo struct {
	file string
	line int64
	fn   string
}

func pcToLine(procInfo *ditypes.ProcessInfo, pc uint64) (*funcInfo, error) {

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

	if int(fileNumber) >= len(files) || files[fileNumber] == nil {
		return nil, fmt.Errorf("could not find file")
	}
	file = files[fileNumber].Name

	return &funcInfo{
		file: file,
		line: line,
		fn:   fn,
	}, nil
}
