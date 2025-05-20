// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

import (
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/logical"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func generateTypeInfos(program logical.Program, functionLoc map[logical.FunctionID]uint32, out io.Writer) {
	fmt.Fprintf(out, "typedef enum type {\n")
	fmt.Fprintf(out, "\tTYPE_NONE = 0,\n")
	for _, t := range program.Types {
		fmt.Fprintf(out, "\tTYPE_%d = %d, // %s\n", t.GetID(), t.GetID(), t.GetName())
	}
	fmt.Fprintf(out, "} type_t;\n\n")

	typeFunc := make(map[ir.TypeID]logical.ProcessType)
	for _, f := range program.Functions {
		if f, ok := f.ID.(logical.ProcessType); ok {
			typeFunc[f.Type.GetID()] = f
		}
	}
	fmt.Fprintf(out, "const type_info_t type_info[] = {\n")
	for _, t := range program.Types {
		enqueuePC := uint32(0)
		if f, ok := typeFunc[t.GetID()]; ok {
			enqueuePC = functionLoc[f]
		}
		fmt.Fprintf(out, "\t/* %d: %s\t*/{.byte_len = %d, .enqueue_pc = 0x%x},\n", t.GetID(), t.GetName(), t.GetByteSize(), enqueuePC)
	}
	fmt.Fprintf(out, "};\n\n")
	fmt.Fprintf(out, "const uint32_t type_ids[] = {")
	for _, t := range program.Types {
		fmt.Fprintf(out, "%d, ", t.GetID())
	}
	fmt.Fprintf(out, "};\n\n")
	fmt.Fprintf(out, "const uint32_t num_types = %d;\n\n", len(program.Types))
}
