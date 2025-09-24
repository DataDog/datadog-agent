// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ignore

// Package loader supports setting up the eBPF program.
package loader

// #define CGO
// #define bool _Bool
// #define int64_t long long
// #define uint8_t unsigned char
// #define uint16_t unsigned short
// #define uint32_t unsigned int
// #define uint64_t unsigned long long
// #include "../ebpf/types.h"
import "C"
import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
)

type typeInfo C.type_info_t
type probeParams C.probe_params_t
type throttlerParams C.throttler_params_t

func opcodeByte(opcode compiler.Opcode) uint8 {
	switch opcode {
	case compiler.OpcodeInvalid:
		return C.SM_OP_INVALID
	case compiler.OpcodeCall:
		return C.SM_OP_CALL
	case compiler.OpcodeReturn:
		return C.SM_OP_RETURN
	case compiler.OpcodeIllegal:
		return C.SM_OP_ILLEGAL
	case compiler.OpcodeIncrementOutputOffset:
		return C.SM_OP_INCREMENT_OUTPUT_OFFSET
	case compiler.OpcodeExprPrepare:
		return C.SM_OP_EXPR_PREPARE
	case compiler.OpcodeExprSave:
		return C.SM_OP_EXPR_SAVE
	case compiler.OpcodeExprDereferenceCfa:
		return C.SM_OP_EXPR_DEREFERENCE_CFA
	case compiler.OpcodeExprReadRegister:
		return C.SM_OP_EXPR_READ_REGISTER
	case compiler.OpcodeExprDereferencePtr:
		return C.SM_OP_EXPR_DEREFERENCE_PTR
	case compiler.OpcodeProcessPointer:
		return C.SM_OP_PROCESS_POINTER
	case compiler.OpcodeProcessSlice:
		return C.SM_OP_PROCESS_SLICE
	case compiler.OpcodeProcessArrayDataPrep:
		return C.SM_OP_PROCESS_ARRAY_DATA_PREP
	case compiler.OpcodeProcessSliceDataPrep:
		return C.SM_OP_PROCESS_SLICE_DATA_PREP
	case compiler.OpcodeProcessSliceDataRepeat:
		return C.SM_OP_PROCESS_SLICE_DATA_REPEAT
	case compiler.OpcodeProcessString:
		return C.SM_OP_PROCESS_STRING
	case compiler.OpcodeProcessGoEmptyInterface:
		return C.SM_OP_PROCESS_GO_EMPTY_INTERFACE
	case compiler.OpcodeProcessGoInterface:
		return C.SM_OP_PROCESS_GO_INTERFACE
	case compiler.OpcodeProcessGoHmap:
		return C.SM_OP_PROCESS_GO_HMAP
	case compiler.OpcodeProcessGoSwissMap:
		return C.SM_OP_PROCESS_GO_SWISS_MAP
	case compiler.OpcodeProcessGoSwissMapGroups:
		return C.SM_OP_PROCESS_GO_SWISS_MAP_GROUPS
	case compiler.OpcodeChasePointers:
		return C.SM_OP_CHASE_POINTERS
	case compiler.OpcodePrepareEventRoot:
		return C.SM_OP_PREPARE_EVENT_ROOT
	default:
		panic(fmt.Sprintf("unknown opcode: %s", opcode))
	}
}
