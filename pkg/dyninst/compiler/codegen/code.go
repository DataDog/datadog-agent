// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package codegen implements the physical encoding of the IR program into eBPF stack machine program.
package codegen

import (
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/sm"
)

// CCodeSerializer serializes the stack machine code into C code.
type CCodeSerializer struct {
	out io.Writer
}

// CommentFunction comments a stack machine function prior to its body.
func (s *CCodeSerializer) CommentFunction(id sm.FunctionID, pc uint32) {
	fmt.Fprintf(s.out, "\t// 0x%x: %s\n", pc, id.String())
}

// SerializeInstruction serializes a stack machine instruction into the output stream.
func (s *CCodeSerializer) SerializeInstruction(name string, paramBytes []byte) {
	fmt.Fprintf(s.out, "\t\t%s, ", name)
	for _, b := range paramBytes {
		fmt.Fprintf(s.out, "0x%02x, ", b)
	}
	fmt.Fprintf(s.out, "\n")
}

// GenerateCCode generates C code, containing:
//   - stack machine code array
//   - type infos array
//   - type id lookup array
//   - and mix of auxiliary variables to use the above arrays.
func GenerateCCode(program sm.Program, out io.Writer) error {
	fmt.Fprintf(out, "const uint8_t stack_machine_code[] = {\n")
	metadata := sm.GenerateCode(program, &CCodeSerializer{out})
	fmt.Fprintf(out, "};\n")
	fmt.Fprintf(out, "const uint64_t stack_machine_code_len = %d;\n", metadata.Len)
	fmt.Fprintf(out, "const uint32_t stack_machine_code_max_op = %d;\n", metadata.MaxOpLen)
	fmt.Fprintf(out, "const uint32_t chase_pointers_entrypoint = 0x%x;\n\n", metadata.FunctionLoc[sm.ChasePointers{}])

	numProbes := 0
	fmt.Fprintf(out, "const probe_params_t probe_params[] = {\n")
	for _, f := range program.Functions {
		if f, ok := f.ID.(sm.ProcessEvent); ok {
			numProbes++
			fmt.Fprintf(out, "\t{.stack_machine_pc = 0x%x, .stream_id = %d, .frameless = false},\n", metadata.FunctionLoc[f], 0)
		}
	}
	fmt.Fprintf(out, "};\n")
	fmt.Fprintf(out, "const uint32_t num_probe_params = %d;\n", numProbes)

	generateTypeInfos(program, metadata.FunctionLoc, out)
	return nil
}
