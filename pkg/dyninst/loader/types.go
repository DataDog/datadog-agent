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

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/sm"
)

type typeInfo C.type_info_t
type probeParams C.probe_params_t
type throttlerParams C.throttler_params_t

func opcodeByte(opcode sm.Opcode) uint8 {
	switch opcode {
	case sm.OpcodeInvalid:
		return C.SM_OP_INVALID
	case sm.OpcodeCall:
		return C.SM_OP_CALL
	case sm.OpcodeReturn:
		return C.SM_OP_RETURN
	case sm.OpcodeIllegal:
		return C.SM_OP_ILLEGAL
	case sm.OpcodeIncrementOutputOffset:
		return C.SM_OP_INCREMENT_OUTPUT_OFFSET
	case sm.OpcodeExprPrepare:
		return C.SM_OP_EXPR_PREPARE
	case sm.OpcodeExprSave:
		return C.SM_OP_EXPR_SAVE
	case sm.OpcodeExprDereferenceCfa:
		return C.SM_OP_EXPR_DEREFERENCE_CFA
	case sm.OpcodeExprReadRegister:
		return C.SM_OP_EXPR_READ_REGISTER
	case sm.OpcodeExprDereferencePtr:
		return C.SM_OP_EXPR_DEREFERENCE_PTR
	case sm.OpcodeProcessPointer:
		return C.SM_OP_PROCESS_POINTER
	case sm.OpcodeProcessSlice:
		return C.SM_OP_PROCESS_SLICE
	case sm.OpcodeProcessArrayDataPrep:
		return C.SM_OP_PROCESS_ARRAY_DATA_PREP
	case sm.OpcodeProcessSliceDataPrep:
		return C.SM_OP_PROCESS_SLICE_DATA_PREP
	case sm.OpcodeProcessSliceDataRepeat:
		return C.SM_OP_PROCESS_SLICE_DATA_REPEAT
	case sm.OpcodeProcessString:
		return C.SM_OP_PROCESS_STRING
	case sm.OpcodeProcessGoEmptyInterface:
		return C.SM_OP_PROCESS_GO_EMPTY_INTERFACE
	case sm.OpcodeProcessGoInterface:
		return C.SM_OP_PROCESS_GO_INTERFACE
	case sm.OpcodeProcessGoHmap:
		return C.SM_OP_PROCESS_GO_HMAP
	case sm.OpcodeProcessGoSwissMap:
		return C.SM_OP_PROCESS_GO_SWISS_MAP
	case sm.OpcodeProcessGoSwissMapGroups:
		return C.SM_OP_PROCESS_GO_SWISS_MAP_GROUPS
	case sm.OpcodeChasePointers:
		return C.SM_OP_CHASE_POINTERS
	case sm.OpcodePrepareEventRoot:
		return C.SM_OP_PREPARE_EVENT_ROOT
	default:
		panic(fmt.Sprintf("unknown opcode: %s", opcode))
	}
}
