// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

// This file uses the cilium/ebpf library to build a source map for an eBPF object file, linking each
// instruction of each program to the source line in the original C code. It relies on the BTF line
// information of the .BTF.ext section, which the kernel requires for the first instruction of each
// function and which, unlike DWARF, survives stripping debug information.

package verifier

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
)

// getSourceMap builds the source map for an eBPF program. It returns two maps, one that
// for each program function maps the instruction offset to the source line information, and
// another that for each section maps the functions that belong to it.
func getSourceMap(spec *ebpf.CollectionSpec) (map[string]map[int]*SourceLine, map[string][]string, error) {
	sourceMap := make(map[string]map[int]*SourceLine)
	funcsPerSection := make(map[string][]string)
	currLineInfo := ""
	currLine := ""
	for _, progSpec := range spec.Programs {
		sourceMap[progSpec.Name] = make(map[int]*SourceLine)
		funcsPerSection[progSpec.SectionName] = append(funcsPerSection[progSpec.SectionName], progSpec.Name)

		iter := progSpec.Instructions.Iterate()
		for iter.Next() {
			insIdx := int(iter.Offset) // Use the instruction offset as the index, because that's what the verifier uses. This accounts for double-wide instructions
			// A single C line can generate multiple instructions, only update the value
			// if we have a new source line
			if line, ok := iter.Ins.Source().(*btf.Line); ok {
				absFilePath, err := ensureSourceFilePathIsAbsolute(line.FileName())
				if err != nil {
					return nil, nil, fmt.Errorf("cannot get absolute path for %s: %w", line.FileName(), err)
				}
				currLineInfo = fmt.Sprintf("%s:%d", absFilePath, line.LineNumber())
				currLine = line.Line()
			} else if insIdx == 0 {
				return nil, nil, fmt.Errorf("missing line information at initial instruction for program %s", progSpec.Name)
			}

			sline := SourceLine{LineInfo: currLineInfo, Line: currLine}
			sourceMap[progSpec.Name][insIdx] = &sline
		}
	}

	return sourceMap, funcsPerSection, nil
}

func ensureSourceFilePathIsAbsolute(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	_, b, _, _ := runtime.Caller(0)
	basepath := filepath.Dir(b)
	repoRoot, err := filepath.Abs(filepath.Join(basepath, "..", "..", ".."))
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, path), nil
}
