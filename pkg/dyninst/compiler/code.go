// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/pkg/errors"
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
	// Optionally comment a block of following instructions.
	CommentBlock(comment string) error
	// Optionally comment a function prior to its body.
	CommentFunction(id FunctionID, pc uint32) error
	// Serialize an instruction into the output stream.
	SerializeInstruction(opcode Opcode, paramBytes []byte, comment string) error
}

// GenerateCode generates the byte code and feeds it to CodeSerializer.
func GenerateCode(program Program, out CodeSerializer) (CodeMetadata, error) {
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

	appendFragment(blockComment{"Extra illegal ops to simplify code bound checks"})
	for range maxOpLen {
		appendFragment(makeInstruction(IllegalOp{}))
	}

	for _, f := range fs {
		err := f.encode(t, out)
		if err != nil {
			return CodeMetadata{}, err
		}
	}

	return CodeMetadata{
		Len:         pc,
		MaxOpLen:    maxOpLen,
		FunctionLoc: t.functionLoc,
	}, nil
}

// NewDispatchingSerializer creates a CodeSerializer that dispatches to a list of other CodeSerializers.
func NewDispatchingSerializer(serializers ...CodeSerializer) CodeSerializer {
	return &dispatchingSerializer{
		serializers: serializers,
	}
}

type dispatchingSerializer struct {
	serializers []CodeSerializer
}

// CommentBlock implements CodeSerializer.
func (s *dispatchingSerializer) CommentBlock(comment string) error {
	for _, serializer := range s.serializers {
		if err := serializer.CommentBlock(comment); err != nil {
			return err
		}
	}
	return nil
}

// CommentFunction implements CodeSerializer.
func (s *dispatchingSerializer) CommentFunction(id FunctionID, pc uint32) error {
	for _, serializer := range s.serializers {
		if err := serializer.CommentFunction(id, pc); err != nil {
			return err
		}
	}
	return nil
}

// SerializeInstruction implements CodeSerializer.
func (s *dispatchingSerializer) SerializeInstruction(opcode Opcode, paramBytes []byte, comment string) error {
	for _, serializer := range s.serializers {
		if err := serializer.SerializeInstruction(opcode, paramBytes, comment); err != nil {
			return err
		}
	}
	return nil
}

// DebugSerializer serializes the stack machine code into human readable format.
type DebugSerializer struct {
	Out io.Writer
}

// CommentBlock implements CodeSerializer.
func (s *DebugSerializer) CommentBlock(comment string) error {
	_, err := fmt.Fprintf(s.Out, "// %s\n", comment)
	return err
}

// CommentFunction implements CodeSerializer.
func (s *DebugSerializer) CommentFunction(id FunctionID, pc uint32) error {
	_, err := fmt.Fprintf(s.Out, "// 0x%x: %s\n", pc, id.String())
	return err
}

// SerializeInstruction implements CodeSerializer.
func (s *DebugSerializer) SerializeInstruction(opcode Opcode, paramBytes []byte, comment string) error {
	_, err := fmt.Fprintf(s.Out, "\t%s ", opcode.String())
	if err != nil {
		return err
	}
	for _, b := range paramBytes {
		_, err := fmt.Fprintf(s.Out, "%02x ", b)
		if err != nil {
			return err
		}
	}
	if comment != "" {
		_, err = fmt.Fprintf(s.Out, "// %s\n", comment)
	} else {
		_, err = fmt.Fprintf(s.Out, "\n")
	}
	return err
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
	encode(t codeTracker, out CodeSerializer) error
}

// comment is a code fragment with free-form comment.
type blockComment struct {
	comment string
}

func (c blockComment) codeByteLen() uint32 {
	return 0
}

func (c blockComment) encode(_ codeTracker, out CodeSerializer) error {
	return out.CommentBlock(c.comment)
}

// functionComment is a code fragment that comments a function, itself containing no code.
type functionComment struct {
	id FunctionID
}

func (f functionComment) codeByteLen() uint32 {
	return 0
}

func (f functionComment) encode(t codeTracker, out CodeSerializer) error {
	return out.CommentFunction(f.id, t.functionLoc[f.id])
}

// staticInstruction is a code fragment encoding logical ops, with all bytes known apriori.
type staticInstruction struct {
	opcode  Opcode
	bytes   []byte
	comment string
}

func (i staticInstruction) codeByteLen() uint32 {
	// First byte is the op code.
	return 1 + uint32(len(i.bytes))
}

func (i staticInstruction) encode(_ codeTracker, out CodeSerializer) error {
	return out.SerializeInstruction(i.opcode, i.bytes, i.comment)
}

// callInstruction is a custom code fragment for logical CallOp, requiring
// known code layout to encode itself.
type callInstruction struct {
	target FunctionID
}

func (i callInstruction) codeByteLen() uint32 {
	return 1 + 4
}

func (i callInstruction) encode(t codeTracker, out CodeSerializer) error {
	si := staticInstruction{
		opcode:  OpcodeCall,
		bytes:   binary.LittleEndian.AppendUint32(nil, t.functionLoc[i.target]),
		comment: i.target.String(),
	}
	if i.codeByteLen() != si.codeByteLen() {
		return errors.Errorf("internal: callInstruction codeByteLen mismatch: %d != %d", i.codeByteLen(), si.codeByteLen())
	}
	return si.encode(t, out)
}
