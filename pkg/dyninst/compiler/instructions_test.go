// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestPrepareEventRootInstructionEncoding(t *testing.T) {
	rootType := &ir.EventRootType{
		TypeCommon: ir.TypeCommon{
			ID:       0x12345678,
			ByteSize: 0x90abcdef,
		},
	}

	inst, ok := makeInstruction(nil, PrepareEventRootOp{
		EventRootType: rootType,
	}).(staticInstruction)
	require.True(t, ok)
	require.Equal(t, OpcodePrepareEventRoot, inst.opcode)
	require.Len(t, inst.bytes, 8)
	require.Equal(t, uint32(0x12345678), binary.LittleEndian.Uint32(inst.bytes[0:4]))
	require.Equal(t, uint32(0x90abcdef), binary.LittleEndian.Uint32(inst.bytes[4:8]))
}

func TestGoContextChainInstructionEncoding(t *testing.T) {
	init, ok := makeInstruction(nil, GoContextChainInitOp{ImplTypeID: 0xdeadbeef}).(staticInstruction)
	require.True(t, ok)
	require.Equal(t, OpcodeGoContextChainInit, init.opcode)
	require.Len(t, init.bytes, 4)
	require.Equal(t, uint32(0xdeadbeef), binary.LittleEndian.Uint32(init.bytes[0:4]))

	hop, ok := makeInstruction(nil, GoContextChainHopOp{}).(staticInstruction)
	require.True(t, ok)
	require.Equal(t, OpcodeGoContextChainHop, hop.opcode)
	require.Empty(t, hop.bytes)
}
