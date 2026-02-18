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
	"errors"
	"fmt"
	"slices"
	"sort"

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

	goRuntimeTypeIDs goRuntimeTypeIDs
	goModuledataInfo ir.GoModuledataInfo
	commonTypes      ir.CommonTypes
}

type goRuntimeTypeIDs struct {
	goRuntimeTypes []uint64
	typeIDs        []uint64
}

var _ sort.Interface = (*goRuntimeTypeIDs)(nil)

func (g *goRuntimeTypeIDs) Len() int { return len(g.goRuntimeTypes) }
func (g *goRuntimeTypeIDs) Less(i int, j int) bool {
	return g.goRuntimeTypes[i] < g.goRuntimeTypes[j]
}
func (g *goRuntimeTypeIDs) Swap(i int, j int) {
	grts, tids := g.goRuntimeTypes, g.typeIDs
	grts[i], grts[j] = grts[j], grts[i]
	tids[i], tids[j] = tids[j], tids[i]
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
		return nil, errors.New("serialized program is missing ChasePointers function")
	}

	slices.SortFunc(program.Types, func(a, b ir.Type) int {
		return cmp.Compare(a.GetID(), b.GetID())
	})
	serialized.typeIDs = make([]uint64, len(program.Types))
	serialized.typeInfos = make([]typeInfo, len(program.Types))
	serialized.goRuntimeTypeIDs = goRuntimeTypeIDs{
		goRuntimeTypes: make([]uint64, 0, len(program.Types)),
		typeIDs:        make([]uint64, 0, len(program.Types)),
	}
	grts := &serialized.goRuntimeTypeIDs
	for i, t := range program.Types {
		typeID := uint64(t.GetID())
		serialized.typeIDs[i] = typeID
		serialized.typeInfos[i] = typeInfo{
			Dynamic_size_class: uint32(t.GetDynamicSizeClass()),
			Byte_len:           t.GetByteSize(),
			Enqueue_pc:         metadata.FunctionLoc[compiler.ProcessType{Type: t}],
		}
		if goRuntimeType, ok := t.GetGoRuntimeType(); ok {
			grts.goRuntimeTypes = append(grts.goRuntimeTypes, uint64(goRuntimeType))
			// If the t is a reference type, the value is not indirected when
			// put into an interface box. Put differently, the data being
			// pointed to is the pointee of the reference type. For all other
			// kinds of types, the box will contain a pointer to that type.
			switch t := t.(type) {
			case *ir.PointerType:
				grts.typeIDs = append(grts.typeIDs, uint64(t.Pointee.GetID()))
			case *ir.GoMapType:
				grts.typeIDs = append(grts.typeIDs, uint64(t.HeaderType.GetID()))
			case *ir.GoChannelType, *ir.GoSubroutineType:
				// TODO: Handle these kinds of types. Right now this is going to
				// be incorrect when we stumble upon them, but we don't have
				// anything more correct to say is underneath the pointer.
				grts.typeIDs = append(grts.typeIDs, typeID)
			default:
				grts.typeIDs = append(grts.typeIDs, typeID)
			}
		}
	}
	sort.Sort(grts)
	serialized.goModuledataInfo = program.GoModuledataInfo
	serialized.commonTypes = program.CommonTypes

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
				Pointer_chasing_limit: f.PointerChasingLimit,
				Collection_size_limit: f.CollectionSizeLimit,
				String_size_limit:     f.StringSizeLimit,
				Frameless:             f.Frameless,
				Has_associated_return: f.HasAssociatedReturn,
				No_return_reason:      int8(f.NoReturnReason),
				Kind:                  int8(f.EventKind),
				Probe_id:              f.ProbeID,
				Top_pc_offset:         int8(f.TopPCOffset),
				X__padding:            [3]int8{},
			})
			serialized.bpfAttachPoints = append(serialized.bpfAttachPoints, BPFAttachPoint{
				PC:     f.InjectionPC,
				Cookie: uint64(len(serialized.bpfAttachPoints)),
			})
		}
	}

	return serialized, nil
}
