// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"cmp"
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

	typeMap := procInfo.TypeMap

	functionIndex, _ := slices.BinarySearchFunc(typeMap.FunctionsByPC, &ditypes.FuncByPCEntry{LowPC: pc}, func(a, b *ditypes.FuncByPCEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})

	if functionIndex >= len(typeMap.FunctionsByPC) {
		return nil, fmt.Errorf("invalid function index")
	}
	funcEntry := typeMap.FunctionsByPC[functionIndex]

	compileUnitIndex, _ := slices.BinarySearchFunc(typeMap.DeclaredFiles, &ditypes.DwarfFilesEntry{LowPC: pc}, func(a, b *ditypes.DwarfFilesEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})

	files := typeMap.DeclaredFiles[compileUnitIndex].Files
	if len(files) < int(funcEntry.FileNumber) {
		return nil, fmt.Errorf("invalid file number in dwarf function entry associated with compile unit")
	}

	if int(funcEntry.FileNumber) >= len(files) || files[funcEntry.FileNumber] == nil {
		return nil, fmt.Errorf("could not find file")
	}
	file := files[funcEntry.FileNumber].Name

	return &funcInfo{
		file: file,
		line: funcEntry.Line,
		fn:   funcEntry.Fn,
	}, nil
}
