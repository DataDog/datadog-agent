// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package loader supports setting up the eBPF program.
package loader

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

type serializedProgram struct {
	programID               uint32
	code                    []byte
	maxOpLen                uint32
	chasePointersEntrypoint uint32

	typeIDs   []uint64
	typeInfos []typeInfo

	throttlerParams []throttlerParams

	probeParams     []probeParams
	bpfAttachPoints []BPFAttachPoint
}

// BPFAttachPoint specifies how the eBPF program should be attached to the user process.
type BPFAttachPoint struct {
	// User process PC to attach at.
	PC uint64
	// Cookie to provide to the entrypoint function.
	Cookie uint64
}

// ByteSerializer serializes the code into a byte array.
type ByteSerializer struct {
	buf *bytes.Buffer
}

// CommentBlock implements CodeSerializer.
func (s *ByteSerializer) CommentBlock(_ string) error {
	return nil
}

// CommentFunction implements CodeSerializer.
func (s *ByteSerializer) CommentFunction(_ compiler.FunctionID, _ uint32) error {
	return nil
}

// SerializeInstruction implements CodeSerializer.
func (s *ByteSerializer) SerializeInstruction(opcode compiler.Opcode, paramBytes []byte, _ string) error {
	s.buf.WriteByte(opcodeByte(opcode))
	s.buf.Write(paramBytes)
	return nil
}

func serializeProgram(
	program compiler.Program,
	additionalSerializer compiler.CodeSerializer,
) (*serializedProgram, error) {
	buf := &bytes.Buffer{}
	serializer := compiler.CodeSerializer(&ByteSerializer{
		buf: buf,
	})

	if additionalSerializer != nil {
		serializer = compiler.NewDispatchingSerializer(serializer, additionalSerializer)
	}

	metadata, err := compiler.GenerateCode(program, serializer)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize code: %w", err)
	}
	serialized := &serializedProgram{
		programID: program.ID,
		code:      buf.Bytes(),
		maxOpLen:  metadata.MaxOpLen,
	}
	var ok bool
	serialized.chasePointersEntrypoint, ok = metadata.FunctionLoc[compiler.ChasePointers{}]
	if !ok {
		return nil, fmt.Errorf("serialized program is missing ChasePointers function")
	}

	slices.SortFunc(program.Types, func(a, b ir.Type) int {
		return cmp.Compare(a.GetID(), b.GetID())
	})
	serialized.typeIDs = make([]uint64, len(program.Types))
	serialized.typeInfos = make([]typeInfo, len(program.Types))
	for i, t := range program.Types {
		serialized.typeIDs[i] = uint64(t.GetID())
		serialized.typeInfos[i] = typeInfo{
			Byte_len:   t.GetByteSize(),
			Enqueue_pc: metadata.FunctionLoc[compiler.ProcessType{Type: t}],
		}
	}

	serialized.throttlerParams = make([]throttlerParams, len(program.Throttlers))
	for i, t := range program.Throttlers {
		serialized.throttlerParams[i] = throttlerParams{
			Ns:     t.PeriodNs,
			Budget: t.Budget,
		}
	}

	for _, p := range program.Functions {
		if f, ok := p.ID.(compiler.ProcessEvent); ok {
			serialized.probeParams = append(serialized.probeParams, probeParams{
				Throttler_idx:         uint32(f.ThrottlerIdx),
				Stack_machine_pc:      metadata.FunctionLoc[f],
				Pointer_chasing_limit: uint32(f.PointerChasingLimit),
				Frameless:             f.Frameless,
			})
			serialized.bpfAttachPoints = append(serialized.bpfAttachPoints, BPFAttachPoint{
				PC:     f.InjectionPC,
				Cookie: uint64(len(serialized.bpfAttachPoints)),
			})
		}
	}

	return serialized, nil
}
