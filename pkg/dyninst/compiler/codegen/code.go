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

// DispatchingSerializer is a CodeSerializer that dispatches to a list of other CodeSerializers.
type DispatchingSerializer struct {
	Serializers []sm.CodeSerializer
}

// CommentBlock implements CodeSerializer.
func (s DispatchingSerializer) CommentBlock(comment string) error {
	for _, serializer := range s.Serializers {
		if err := serializer.CommentBlock(comment); err != nil {
			return err
		}
	}
	return nil
}

// CommentFunction implements CodeSerializer.
func (s DispatchingSerializer) CommentFunction(id sm.FunctionID, pc uint32) error {
	for _, serializer := range s.Serializers {
		if err := serializer.CommentFunction(id, pc); err != nil {
			return err
		}
	}
	return nil
}

// SerializeInstruction implements CodeSerializer.
func (s DispatchingSerializer) SerializeInstruction(opcode sm.Opcode, paramBytes []byte, comment string) error {
	for _, serializer := range s.Serializers {
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
func (s *DebugSerializer) CommentFunction(id sm.FunctionID, pc uint32) error {
	_, err := fmt.Fprintf(s.Out, "// 0x%x: %s\n", pc, id.String())
	return err
}

// SerializeInstruction implements CodeSerializer.
func (s *DebugSerializer) SerializeInstruction(opcode sm.Opcode, paramBytes []byte, comment string) error {
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
