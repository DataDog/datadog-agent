// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

import (
	"cmp"
	"io"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/sm"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func generateTypeInfos(program sm.Program, functionLoc map[sm.FunctionID]uint32, out io.Writer) (err error) {
	defer func() {
		err = recoverFprintf()
	}()

	slices.SortFunc(program.Types, func(a, b ir.Type) int {
		return cmp.Compare(a.GetID(), b.GetID())
	})
	mustFprintf(out, "typedef enum type {\n")
	mustFprintf(out, "\tTYPE_NONE = 0,\n")
	for _, t := range program.Types {
		mustFprintf(out, "\tTYPE_%d = %d, // %s\n", t.GetID(), t.GetID(), t.GetName())
	}
	mustFprintf(out, "} type_t;\n\n")

	typeFunc := make(map[ir.TypeID]sm.ProcessType)
	for _, f := range program.Functions {
		if f, ok := f.ID.(sm.ProcessType); ok {
			typeFunc[f.Type.GetID()] = f
		}
	}
	mustFprintf(out, "const type_info_t type_info[] = {\n")
	for _, t := range program.Types {
		enqueuePC := uint32(0)
		if f, ok := typeFunc[t.GetID()]; ok {
			enqueuePC = functionLoc[f]
		}
		mustFprintf(out, "\t/* %d: %s\t*/{.byte_len = %d, .enqueue_pc = 0x%x},\n", t.GetID(), t.GetName(), t.GetByteSize(), enqueuePC)
	}
	mustFprintf(out, "};\n\n")
	mustFprintf(out, "const uint32_t type_ids[] = {")
	for _, t := range program.Types {
		mustFprintf(out, "%d, ", t.GetID())
	}
	mustFprintf(out, "};\n\n")
	mustFprintf(out, "const uint32_t num_types = %d;\n\n", len(program.Types))
	return nil
}
