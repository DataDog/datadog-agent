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
		// Presence bit index (2 bits per expression).
		bytes = binary.LittleEndian.AppendUint32(bytes, 2*op.ExprIdx)
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
		bytes := make([]byte, 0, 12)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Bias)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Len)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.NilBitIdx)
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
		bytes := make([]byte, 0, 12)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.BucketsType.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.BucketType.GetByteSize())
		bytes = append(bytes, op.FlagsOffset)
		bytes = append(bytes, op.BOffset)
		bytes = append(bytes, op.BucketsOffset)
		bytes = append(bytes, op.OldBucketsOffset)
		return staticInstruction{
			opcode: OpcodeProcessGoHmap,
			bytes:  bytes,
		}

	case ProcessGoSwissMapOp:
		bytes := make([]byte, 0, 16)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.TablePtrSlice.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.Group.GetID()))
		bytes = append(bytes, op.DirPtrOffset)
		bytes = append(bytes, op.DirLenOffset)
		return staticInstruction{
			opcode: OpcodeProcessGoSwissMap,
			bytes:  bytes,
		}

	case ProcessGoSwissMapGroupsOp:
		bytes := make([]byte, 0, 16)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.GroupSlice.GetID()))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Group.GetByteSize())
		bytes = append(bytes, op.DataOffset)
		bytes = append(bytes, op.LengthMaskOffset)
		return staticInstruction{
			opcode: OpcodeProcessGoSwissMapGroups,
			bytes:  bytes,
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

	case ExprPushOffsetOp:
		return staticInstruction{
			opcode: OpcodeExprPushOffset,
			bytes:  binary.LittleEndian.AppendUint32(nil, op.ByteSize),
		}

	case ExprLoadLiteralOp:
		bytes := make([]byte, 0, 2+len(op.Data))
		bytes = binary.LittleEndian.AppendUint16(bytes, uint16(len(op.Data)))
		bytes = append(bytes, op.Data...)
		return staticInstruction{
			opcode: OpcodeExprLoadLiteral,
			bytes:  bytes,
		}

	case ExprReadStringOp:
		return staticInstruction{
			opcode: OpcodeExprReadString,
			bytes:  binary.LittleEndian.AppendUint16(nil, op.MaxLen),
		}

	case ExprCmpEqBaseOp:
		return staticInstruction{
			opcode: OpcodeExprCmpEqBase,
			bytes:  []byte{op.ByteSize},
		}

	case ExprCmpEqStringOp:
		return staticInstruction{
			opcode: OpcodeExprCmpEqString,
			bytes:  []byte{},
		}

	case ConditionBeginOp:
		return staticInstruction{
			opcode: OpcodeConditionBegin,
			bytes:  []byte{},
		}

	case ConditionCheckOp:
		return staticInstruction{
			opcode: OpcodeConditionCheck,
			bytes:  []byte{},
		}

	default:
		panic(fmt.Sprintf("unsupported op: %T", op))
	}
}
