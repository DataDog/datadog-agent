// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

import (
	"encoding/binary"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/logical"
)

func makeInstruction(op logical.Op) encodable {
	switch op := op.(type) {
	case logical.CallOp:
		return callInstruction{target: op.FunctionID}

	case logical.ReturnOp:
		return staticInstruction{
			name:  "SM_OP_RETURN",
			bytes: []byte{},
		}

	case logical.IllegalOp:
		return staticInstruction{
			name:  "SM_OP_ILLEGAL",
			bytes: []byte{},
		}

	case logical.IncrementOutputOffsetOp:
		return staticInstruction{
			name:  "SM_OP_INCREMENT_OUTPUT_OFFSET",
			bytes: binary.LittleEndian.AppendUint32(nil, op.Value),
		}

	case logical.ExprPrepareOp:
		return staticInstruction{
			name:  "SM_OP_EXPR_PREPARE",
			bytes: []byte{},
		}

	case logical.ExprSaveOp:
		bytes := make([]byte, 12)
		// Result offset and length.
		e := op.EventRootType.Expressions[op.ExprIdx]
		bytes = binary.LittleEndian.AppendUint32(bytes, e.Offset)
		bytes = binary.LittleEndian.AppendUint32(bytes, e.Expression.Type.GetByteSize())
		// Presence bit index.
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ExprIdx)
		return staticInstruction{
			name:  "SM_OP_EXPR_SAVE",
			bytes: bytes,
		}

	case logical.ExprDereferenceCfaOp:
		bytes := make([]byte, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Offset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Len)
		return staticInstruction{
			name:  "SM_OP_EXPR_DEREFERENCE_CFA",
			bytes: bytes,
		}

	case logical.ExprReadRegisterOp:
		return staticInstruction{
			name:  "SM_OP_EXPR_DEREFERENCE_CFA",
			bytes: []byte{op.Register, op.Size},
		}

	case logical.ExprDereferencePtrOp:
		bytes := make([]byte, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Bias)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Len)
		return staticInstruction{
			name:  "SM_OP_EXPR_DEREFERENCE_PTR",
			bytes: bytes,
		}

	case logical.ProcessPointerOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_POINTER",
			bytes: binary.LittleEndian.AppendUint32(nil, uint32(op.Pointee.GetID())),
		}

	case logical.ProcessArrayPrepOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_ARRAY_PREP",
			bytes: binary.LittleEndian.AppendUint32(nil, op.Array.Count),
		}

	case logical.ProcessArrayRepeatOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_ARRAY_PREP",
			bytes: binary.LittleEndian.AppendUint32(nil, op.OffsetShift),
		}

	case logical.ProcessSliceOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_SLICE",
			bytes: binary.LittleEndian.AppendUint32(nil, uint32(op.SliceData.GetID())),
		}

	case logical.ProcessSliceDataPrepOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_SLICE_DATA_PREP",
			bytes: []byte{},
		}

	case logical.ProcessSliceDataRepeatOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_SLICE_DATA_REPEAT",
			bytes: binary.LittleEndian.AppendUint32(nil, op.OffsetShift),
		}

	case logical.ProcessStringOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_STRING",
			bytes: []byte{},
		}

	case logical.ProcessGoEmptyInterfaceOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_GO_EMPTY_INTERFACE",
			bytes: []byte{},
		}

	case logical.ProcessGoInterfaceOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_GO_INTERFACE",
			bytes: []byte{},
		}

	case logical.ProcessGoHmapOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_GO_HMAP",
			bytes: binary.LittleEndian.AppendUint32(nil, uint32(op.BucketsArray.GetID())),
		}

	case logical.ProcessGoSwissMapOp:
		bytes := make([]byte, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.TablePtrSlice.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.Group.GetID()))
		return staticInstruction{
			name:  "SM_OP_PROCESS_GO_SWISS_MAP",
			bytes: bytes,
		}

	case logical.ProcessGoSwissMapGroupsOp:
		return staticInstruction{
			name:  "SM_OP_PROCESS_GO_SWISS_MAP_GROUPS",
			bytes: binary.LittleEndian.AppendUint32(nil, uint32(op.Group.GetID())),
		}

	case logical.ChasePointersOp:
		return staticInstruction{
			name:  "SM_OP_CHASE_POINTERS",
			bytes: []byte{},
		}

	case logical.PrepareEventRootOp:
		bytes := make([]byte, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.EventRootType.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.EventRootType.GetByteSize())
		return staticInstruction{
			name:  "SM_OP_PREPARE_EVENT_ROOT",
			bytes: bytes,
		}

	default:
		panic(fmt.Sprintf("unsupported op: %T", op))
	}
}
