// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

import (
	"fmt"
	"io"
	"os"
)

// Dump writes a human-readable representation of a BPF program to stdout.
// The option parameter controls the output format:
//   - option >= 3: machine-readable format (count + raw fields)
//   - option >= 2: C struct initializer format
//   - option >= 1: human-readable format using Image()
//
// Port of bpf_dump() from libpcap's bpf_dump.c.
func Dump(p *Program, option int) {
	FprintDump(os.Stdout, p, option)
}

// FprintDump writes a human-readable representation to the given writer.
func FprintDump(w io.Writer, p *Program, option int) {
	n := len(p.Instructions)

	if option > 2 {
		fmt.Fprintf(w, "%d\n", n)
		for _, insn := range p.Instructions {
			fmt.Fprintf(w, "%d %d %d %d\n", insn.Code, insn.Jt, insn.Jf, insn.K)
		}
		return
	}

	if option > 1 {
		for _, insn := range p.Instructions {
			fmt.Fprintf(w, "{ 0x%x, %d, %d, 0x%08x },\n",
				insn.Code, insn.Jt, insn.Jf, insn.K)
		}
		return
	}

	for i, insn := range p.Instructions {
		fmt.Fprintln(w, Image(insn, i))
	}
}
