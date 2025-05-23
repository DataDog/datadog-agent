// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package sm implements the eBPF program stack machine representation and generation.
package sm

import (
	"encoding/binary"
	"fmt"
)

// CodeMetadata contains metadata about the generated code.
type CodeMetadata struct {
	Len         uint32
	MaxOpLen    uint32
	FunctionLoc map[FunctionID]uint32
}

// CodeSerializer is the interface for serializing byte code into native
// stack machine code.
type CodeSerializer interface {
	// Optionally comment a function prior to its body.
	CommentFunction(id FunctionID, pc uint32)
	// Serialize an instruction into the output stream.
	SerializeInstruction(name string, paramBytes []byte)
}

// GenerateCode generates the byte code and feeds it to CodeSerializer.
func GenerateCode(program Program, out CodeSerializer) CodeMetadata {
	t := codeTracker{
		functionLoc: make(map[FunctionID]uint32, len(program.Functions)),
	}

	var fs []codeFragment
	pc := uint32(0)
	maxOpLen := uint32(0)
	appendFragment := func(f codeFragment) {
		fs = append(fs, f)
		pc += f.codeByteLen()
		maxOpLen = max(maxOpLen, f.codeByteLen())
	}

	appendFragment(makeInstruction(IllegalOp{}))

	for _, f := range program.Functions {
		t.functionLoc[f.ID] = pc
		appendFragment(functionComment{id: f.ID})
		for _, op := range f.Ops {
			appendFragment(makeInstruction(op))
		}
	}

	for range maxOpLen {
		appendFragment(makeInstruction(IllegalOp{}))
	}

	for _, f := range fs {
		f.encode(t, out)
	}

	return CodeMetadata{
		Len:         pc,
		MaxOpLen:    maxOpLen,
		FunctionLoc: t.functionLoc,
	}
}

// tracker aggregates information about the final generated code,
// before it is generated.
type codeTracker struct {
	// PC of the first instruction of the function, used for call ops.
	functionLoc map[FunctionID]uint32
}

// codeFragment is part of the code can be serialized into byte code.
// Each code fragment must be able to declare apriori how many bytes it will
// generate.
type codeFragment interface {
	codeByteLen() uint32
	encode(t codeTracker, out CodeSerializer)
}

// functionComment is a code fragment that comments a function, itself containing no code.
type functionComment struct {
	id FunctionID
}

func (f functionComment) codeByteLen() uint32 {
	return 0
}

func (f functionComment) encode(t codeTracker, out CodeSerializer) {
	out.CommentFunction(f.id, t.functionLoc[f.id])
}

// staticInstruction is a code fragment encoding logical ops, with all bytes known apriori.
type staticInstruction struct {
	name  string
	bytes []byte
}

func (i staticInstruction) codeByteLen() uint32 {
	// First byte is the op code.
	return 1 + uint32(len(i.bytes))
}

func (i staticInstruction) encode(_ codeTracker, out CodeSerializer) {
	out.SerializeInstruction(i.name, i.bytes)
}

// callInstruction is a custom code fragment for logical CallOp, requiring
// known code layout to encode itself.
type callInstruction struct {
	target FunctionID
}

func (i callInstruction) codeByteLen() uint32 {
	return 1 + 4
}

func (i callInstruction) encode(t codeTracker, out CodeSerializer) {
	si := staticInstruction{
		name:  "SM_OP_CALL",
		bytes: binary.LittleEndian.AppendUint32(nil, t.functionLoc[i.target]),
	}
	if i.codeByteLen() != si.codeByteLen() {
		panic(fmt.Sprintf("callInstruction codeByteLen mismatch: %d != %d", i.codeByteLen(), si.codeByteLen()))
	}
	si.encode(t, out)
}
