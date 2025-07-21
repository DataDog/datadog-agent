// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// irGenerator generates the ir for the program to scrape remote config.
// Importantly it only uses the symbol table from the binary to generate the ir;
// it does not decompress or parse the dwarf data.
type irGenerator struct{}

func (irGenerator) GenerateIR(
	programID ir.ProgramID,
	executable *object.ElfFile,
	probes []ir.ProbeDefinition,
) (*ir.Program, error) {
	var v1Def, v2Def ir.ProbeDefinition
	for _, probe := range probes {
		switch id := probe.GetID(); id {
		case rcProbeIDV1:
			v1Def = probe
		case rcProbeIDV2:
			v2Def = probe
		default:
			return nil, fmt.Errorf("unexpected probe ID: %s", id)
		}
	}

	if v1Def == nil || v2Def == nil {
		return nil, fmt.Errorf("missing probe definitions: %v", probes)
	}

	program := &ir.Program{
		ID: programID,
	}
	// TODO: We could use less memory if we had a better symbol table parser.
	symbols, err := executable.Underlying.Elf.Symbols()
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols: %w", err)
	}
	var v1, v2 *safeelf.Symbol
	for _, symbol := range symbols {
		switch symbol.Name {
		case v1PassProbeConfiguration:
			s := symbol
			v1 = &s
		case v2PassProbeConfiguration:
			s := symbol
			v2 = &s
		}
	}
	if v1 == nil {
		program.Issues = append(program.Issues, ir.ProbeIssue{
			ProbeDefinition: v1Def,
			Issue: ir.Issue{
				Kind:    ir.IssueKindTargetNotFoundInBinary,
				Message: msgV1PassProbeConfigurationNotFound,
			},
		})
	}
	if v2 == nil {
		program.Issues = append(program.Issues, ir.ProbeIssue{
			ProbeDefinition: v2Def,
			Issue: ir.Issue{
				Kind:    ir.IssueKindTargetNotFoundInBinary,
				Message: msgV2PassProbeConfigurationNotFound,
			},
		})
	}
	if v1 == nil && v2 == nil {
		return program, nil
	}
	program.Types = makeBaseTypesMap()
	program.MaxTypeID = ir.TypeID(len(program.Types))
	var regs abiRegs
	switch arch := executable.Architecture(); arch {
	case "amd64":
		regs = amd64AbiRegs
	case "arm64":
		regs = arm64AbiRegs
	default:
		return program, fmt.Errorf("unsupported architecture: %s", arch)
	}
	if v1 != nil {
		addProbe(regs, program, v1Def, v1)
	}
	if v2 != nil {
		addProbe(regs, program, v2Def, v2)
	}
	return program, nil
}

type abiRegs []uint8

// RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11
// See https://github.com/golang/go/blob/62deaf4f/src/cmd/compile/abi-internal.md?plain=1#L390
// https://gitlab.com/x86-psABIs/x86-64-ABI/-/blob/e1ce098331da5dbd66e1ffc74162380bcc213236/x86-64-ABI/low-level-sys-info.tex#L2508-2516
var amd64AbiRegs = abiRegs{0, 3, 2, 5, 4, 8, 9, 10, 11}

// https://github.com/golang/go/blob/62deaf4f/src/cmd/compile/abi-internal.md?plain=1#L516
var arm64AbiRegs = abiRegs{0, 1, 2, 3, 4, 5, 6, 7, 8}

func addProbe(
	abiRegs abiRegs,
	program *ir.Program,
	probeDef ir.ProbeDefinition,
	symbol *safeelf.Symbol,
) {
	pcRange := ir.PCRange{symbol.Value, symbol.Value + 1}
	subprogram := &ir.Subprogram{
		ID:                ir.SubprogramID(len(program.Subprograms) + 1),
		Name:              symbol.Name,
		OutOfLinePCRanges: []ir.PCRange{pcRange},
		Variables: []*ir.Variable{
			{
				Name: "runtimeID",
				Type: stringType,
				Locations: []ir.Location{{
					Range: pcRange,
					Pieces: []ir.Piece{
						{Size: 8, Op: ir.Register{RegNo: abiRegs[0]}},
						{Size: 8, Op: ir.Register{RegNo: abiRegs[1]}},
					},
				}},
				IsParameter: true,
			},
			{
				Name: "configPath",
				Type: stringType,
				Locations: []ir.Location{{
					Range: pcRange,
					Pieces: []ir.Piece{
						{Size: 8, Op: ir.Register{RegNo: abiRegs[2]}},
						{Size: 8, Op: ir.Register{RegNo: abiRegs[3]}},
					},
				}},
			},
			{
				Name: "configContent",
				Type: stringType,
				Locations: []ir.Location{{
					Range: pcRange,
					Pieces: []ir.Piece{
						{Size: 8, Op: ir.Register{RegNo: abiRegs[4]}},
						{Size: 8, Op: ir.Register{RegNo: abiRegs[5]}},
					},
				}},
			},
		},
	}
	program.Subprograms = append(program.Subprograms, subprogram)
	var offset = uint32(1) // for the presence bitset
	var exprs = make([]*ir.RootExpression, 0, len(subprogram.Variables))
	for _, variable := range subprogram.Variables {
		exprs = append(exprs, &ir.RootExpression{
			Name:   variable.Name,
			Offset: offset,
			Expression: ir.Expression{
				Type: stringType,
				Operations: []ir.ExpressionOp{
					&ir.LocationOp{
						Variable: variable,
						ByteSize: variable.Type.GetByteSize(),
					},
				},
			},
		})
		offset += variable.Type.GetByteSize()
	}

	program.MaxTypeID++
	rootType := &ir.EventRootType{
		TypeCommon: ir.TypeCommon{
			ID:       program.MaxTypeID,
			Name:     symbol.Name,
			ByteSize: offset,
		},
		Expressions: exprs,
	}
	program.Types[program.MaxTypeID] = rootType
	probe := &ir.Probe{
		ProbeDefinition: probeDef,
		Subprogram:      subprogram,
		Events: []*ir.Event{{
			ID:   ir.EventID(subprogram.ID),
			Type: rootType,
			InjectionPoints: []ir.InjectionPoint{
				{PC: symbol.Value, Frameless: true},
			},
		}},
	}
	program.Probes = append(program.Probes, probe)
}

var (
	msgV1PassProbeConfigurationNotFound = fmt.Sprintf(
		"symbol %s not found in binary", v1PassProbeConfiguration,
	)
	msgV2PassProbeConfigurationNotFound = fmt.Sprintf(
		"symbol %s not found in binary", v2PassProbeConfiguration,
	)
)

var (
	intType = &ir.BaseType{
		TypeCommon:       ir.TypeCommon{ID: 1, Name: "int", ByteSize: 8},
		GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Int},
	}
	strDataType = &ir.GoStringDataType{
		TypeCommon: ir.TypeCommon{ID: 2, Name: "string.str", ByteSize: 8192},
	}
	strDataPointerType = &ir.PointerType{
		TypeCommon: ir.TypeCommon{ID: 3, Name: "*string.str", ByteSize: 8},
		Pointee:    strDataType,
	}
	stringType = &ir.GoStringHeaderType{
		StructureType: &ir.StructureType{
			TypeCommon: ir.TypeCommon{ID: 4, Name: "string", ByteSize: 16},
			Fields: []ir.Field{
				{Name: "str", Offset: 0, Type: strDataPointerType},
				{Name: "len", Offset: 8, Type: intType},
			},
		},
		Data: strDataType,
	}
	baseTypes = []ir.Type{
		intType,
		strDataType,
		strDataPointerType,
		stringType,
	}
)

func makeBaseTypesMap() map[ir.TypeID]ir.Type {
	m := make(map[ir.TypeID]ir.Type)
	for _, t := range baseTypes {
		m[t.GetID()] = t
	}
	return m
}
