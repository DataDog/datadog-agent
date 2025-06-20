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

// BPFAttachPoint specifies how the eBPF program should be attached to the user process.
type BPFAttachPoint struct {
	// User process PC to attach at.
	PC uint64
	// Cookie to provide to the entrypoint function.
	Cookie uint64
}

// cCodeSerializer serializes the stack machine code into C code.
type cCodeSerializer struct {
	out io.Writer
}

// CommentBlock implements CodeSerializer.
func (s *cCodeSerializer) CommentBlock(comment string) error {
	_, err := fmt.Fprintf(s.out, "\n\t// %s\n", comment)
	return err
}

// CommentFunction implements CodeSerializer.
func (s *cCodeSerializer) CommentFunction(id sm.FunctionID, pc uint32) error {
	_, err := fmt.Fprintf(s.out, "\n\t// 0x%x: %s\n", pc, id.String())
	return err
}

// SerializeInstruction implements CodeSerializer.
func (s *cCodeSerializer) SerializeInstruction(name string, paramBytes []byte, comment string) error {
	_, err := fmt.Fprintf(s.out, "\t\t%s, ", name)
	if err != nil {
		return err
	}
	for _, b := range paramBytes {
		_, err := fmt.Fprintf(s.out, "0x%02x, ", b)
		if err != nil {
			return err
		}
	}
	if comment != "" {
		_, err = fmt.Fprintf(s.out, "// %s\n", comment)
	} else {
		_, err = fmt.Fprintf(s.out, "\n")
	}
	return err
}

// GenerateCCode generates C code, containing:
//   - stack machine code array
//   - type infos array
//   - type id lookup array
//   - and mix of auxiliary variables to use the above arrays.
func GenerateCCode(program sm.Program, out io.Writer) (attachpoints []BPFAttachPoint, err error) {
	defer func() {
		err = recoverFprintf()
	}()
	mustFprintf(out, "const uint8_t stack_machine_code[] = {\n")
	metadata, err := sm.GenerateCode(program, &cCodeSerializer{out})
	if err != nil {
		return nil, err
	}
	mustFprintf(out, "};\n")
	mustFprintf(out, "const uint64_t stack_machine_code_len = %d;\n", metadata.Len)
	mustFprintf(out, "const uint32_t stack_machine_code_max_op = %d;\n", metadata.MaxOpLen)
	mustFprintf(out, "const uint32_t chase_pointers_entrypoint = 0x%x;\n\n", metadata.FunctionLoc[sm.ChasePointers{}])
	mustFprintf(out, "const uint32_t prog_id = %d;\n\n", program.ID)

	mustFprintf(out, "const probe_params_t probe_params[] = {\n")
	for _, f := range program.Functions {
		if f, ok := f.ID.(sm.ProcessEvent); ok {
			attachpoints = append(attachpoints, BPFAttachPoint{
				PC:     f.InjectionPC,
				Cookie: uint64(len(attachpoints)),
			})
			mustFprintf(
				out, "\t{.throttler_idx = %d, .stack_machine_pc = 0x%x, .pointer_chasing_limit = %d, .frameless = false},\n",
				f.ThrottlerIdx, metadata.FunctionLoc[f], f.PointerChasingLimit,
			)
		}
	}
	mustFprintf(out, "};\n")
	mustFprintf(out, "const uint32_t num_probe_params = %d;\n", len(attachpoints))

	err = generateTypeInfos(program, metadata.FunctionLoc, out)
	if err != nil {
		return nil, err
	}

	mustFprintf(out, "const throttler_params_t throttler_params[] = {\n")
	for _, t := range program.Throttlers {
		mustFprintf(out, "\t{.period_ns = %d, .budget = %d},\n", t.PeriodNs, t.Budget)
	}
	mustFprintf(out, "};\n")
	mustFprintf(out, "#define NUM_THROTTLERS %d\n", len(program.Throttlers))

	return attachpoints, nil
}

func mustFprintf(out io.Writer, fs string, args ...any) {
	_, err := fmt.Fprintf(out, fs, args...)
	if err != nil {
		panic(fmt.Errorf("failed to write to output: %w", err))
	}
}

func recoverFprintf() error {
	switch r := recover().(type) {
	case nil:
	case error:
		return r
	default:
		panic(r)
	}
	return nil
}
