// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"encoding/binary"
	"fmt"
)

func makeInstruction(op Op) codeFragment {
	switch op := op.(type) {
	case CallOp:
		return callInstruction{target: op.FunctionID}

	case ReturnOp:
		return staticInstruction{
			opcode: OpcodeReturn,
			bytes:  []byte{},
		}

	case IllegalOp:
		return staticInstruction{
			opcode: OpcodeIllegal,
			bytes:  []byte{},
		}

	case IncrementOutputOffsetOp:
		return staticInstruction{
			opcode: OpcodeIncrementOutputOffset,
			bytes:  binary.LittleEndian.AppendUint32(nil, op.Value),
		}

	case ExprPrepareOp:
		return staticInstruction{
			opcode: OpcodeExprPrepare,
			bytes:  []byte{},
		}

	case ExprSaveOp:
		bytes := make([]byte, 0, 12)
		// Result offset and length.
		e := op.EventRootType.Expressions[op.ExprIdx]
		bytes = binary.LittleEndian.AppendUint32(bytes, e.Offset)
		bytes = binary.LittleEndian.AppendUint32(bytes, e.Expression.Type.GetByteSize())
		// Presence bit index.
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ExprIdx)
		return staticInstruction{
			opcode: OpcodeExprSave,
			bytes:  bytes,
		}

	case ExprDereferenceCfaOp:
		bytes := make([]byte, 0, 12)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.Offset))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Len)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.OutputOffset)
		return staticInstruction{
			opcode: OpcodeExprDereferenceCfa,
			bytes:  bytes,
		}

	case ExprReadRegisterOp:
		bytes := make([]byte, 0, 6)
		bytes = append(bytes, op.Register, op.Size)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.OutputOffset)
		return staticInstruction{
			opcode: OpcodeExprReadRegister,
			bytes:  bytes,
		}

	case ExprDereferencePtrOp:
		bytes := make([]byte, 0, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Bias)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Len)
		return staticInstruction{
			opcode: OpcodeExprDereferencePtr,
			bytes:  bytes,
		}

	case ProcessPointerOp:
		return staticInstruction{
			opcode: OpcodeProcessPointer,
			bytes:  binary.LittleEndian.AppendUint32(nil, uint32(op.Pointee.GetID())),
		}

	case ProcessArrayDataPrepOp:
		return staticInstruction{
			opcode: OpcodeProcessArrayDataPrep,
			bytes:  binary.LittleEndian.AppendUint32(nil, op.ArrayByteLen),
		}

	case ProcessSliceOp:
		bytes := make([]byte, 0, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.SliceData.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.SliceData.Element.GetByteSize())
		return staticInstruction{
			opcode: OpcodeProcessSlice,
			bytes:  bytes,
		}

	case ProcessSliceDataPrepOp:
		return staticInstruction{
			opcode: OpcodeProcessSliceDataPrep,
			bytes:  []byte{},
		}

	case ProcessSliceDataRepeatOp:
		return staticInstruction{
			opcode: OpcodeProcessSliceDataRepeat,
			bytes:  binary.LittleEndian.AppendUint32(nil, op.ElemByteLen),
		}

	case ProcessStringOp:
		return staticInstruction{
			opcode: OpcodeProcessString,
			bytes:  binary.LittleEndian.AppendUint32(nil, uint32(op.StringData.GetID())),
		}

	case ProcessGoEmptyInterfaceOp:
		return staticInstruction{
			opcode: OpcodeProcessGoEmptyInterface,
			bytes:  []byte{},
		}

	case ProcessGoInterfaceOp:
		return staticInstruction{
			opcode: OpcodeProcessGoInterface,
			bytes:  []byte{},
		}

	case ProcessGoHmapOp:
		return staticInstruction{
			opcode: OpcodeProcessGoHmap,
			bytes:  binary.LittleEndian.AppendUint32(nil, uint32(op.BucketsArray.GetID())),
		}

	case ProcessGoSwissMapOp:
		bytes := make([]byte, 0, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.TablePtrSlice.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.Group.GetID()))
		return staticInstruction{
			opcode: OpcodeProcessGoSwissMap,
			bytes:  bytes,
		}

	case ProcessGoSwissMapGroupsOp:
		return staticInstruction{
			opcode: OpcodeProcessGoSwissMapGroups,
			bytes:  binary.LittleEndian.AppendUint32(nil, uint32(op.Group.GetID())),
		}

	case ChasePointersOp:
		return staticInstruction{
			opcode: OpcodeChasePointers,
			bytes:  []byte{},
		}

	case PrepareEventRootOp:
		bytes := make([]byte, 0, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.EventRootType.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.EventRootType.GetByteSize())
		return staticInstruction{
			opcode: OpcodePrepareEventRoot,
			bytes:  bytes,
		}

	default:
		panic(fmt.Sprintf("unsupported op: %T", op))
	}
}
