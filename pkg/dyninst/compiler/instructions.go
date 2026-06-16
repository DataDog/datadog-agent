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

// makeInstruction builds a codeFragment for op. functionID identifies the
// enclosing function body and is only used by fragments that need
// function-scoped references (jumps and labels); it may be nil for ops
// that never appear inside a function (e.g. the leading/trailing
// IllegalOp guards).
func makeInstruction(functionID FunctionID, op Op) codeFragment {
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
		// Expression index into the ExprStatusArray.
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
		bytes := make([]byte, 0, 13)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Bias)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Len)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ExprStatusIdx)
		nullAsZero := uint8(0)
		if op.NullAsZero {
			nullAsZero = 1
		}
		bytes = append(bytes, nullAsZero)
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

	case GoContextChainInitOp:
		return staticInstruction{
			opcode: OpcodeGoContextChainInit,
			bytes:  binary.LittleEndian.AppendUint32(nil, uint32(op.ImplTypeID)),
		}

	case GoContextChainHopOp:
		return staticInstruction{
			opcode: OpcodeGoContextChainHop,
			bytes:  []byte{},
		}

	case ProcessGoDictTypeOp:
		bytes := make([]byte, 0, 9)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.DictIndex))
		bytes = append(bytes, op.DictRegister)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.OutputOffset)
		return staticInstruction{
			opcode: OpcodeProcessGoDictType,
			bytes:  bytes,
		}

	case CallDictResolvedOp:
		return callDictResolvedInstruction{
			outputOffset: op.OutputOffset,
			fallback:     op.FallbackFunc,
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

	case ProcessGoTimeOp:
		// Layout: wall_off (u32), ext_off (u32), loc_off (u32),
		// cache_resolved (u8), cache_start_off (u32), cache_end_off (u32),
		// cache_zone_off (u32), zone_offset_field_off (u32),
		// zone_offset_field_size (u32).
		bytes := make([]byte, 0, 33)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.WallFieldOffset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ExtFieldOffset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.LocFieldOffset)
		var resolved uint8
		if op.CacheResolved {
			resolved = 1
		}
		bytes = append(bytes, resolved)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.CacheStartOffset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.CacheEndOffset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.CacheZoneOffset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ZoneOffsetFieldOffset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ZoneOffsetFieldSize)
		return staticInstruction{
			opcode: OpcodeProcessGoTime,
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

	case ExprLoadDurationOp:
		return staticInstruction{
			opcode: OpcodeExprLoadDuration,
			bytes:  binary.LittleEndian.AppendUint32(nil, op.ExprStatusIdx),
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

	case ExprCmpBaseOp:
		return staticInstruction{
			opcode: OpcodeExprCmpBase,
			bytes:  []byte{op.ByteSize, uint8(op.Op), uint8(op.Kind)},
		}

	case ExprCmpStringOp:
		return staticInstruction{
			opcode: OpcodeExprCmpString,
			bytes:  []byte{uint8(op.Op)},
		}

	case ExprSliceBoundsCheckOp:
		bytes := make([]byte, 0, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.Index)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ExprStatusIdx)
		return staticInstruction{
			opcode: OpcodeExprSliceBoundsCheck,
			bytes:  bytes,
		}

	case SwissMapSetupOp:
		isStr := uint8(0)
		if op.IsStringKey {
			isStr = 1
		}
		bytes := make([]byte, 0, 32+len(op.KeyData))
		bytes = append(bytes, isStr)
		bytes = append(bytes, op.KeyByteSize)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ValByteSize)
		bytes = append(bytes, op.SeedOffset)
		bytes = append(bytes, op.DirPtrOffset)
		bytes = append(bytes, op.DirLenOffset)
		bytes = append(bytes, op.GlobalShiftOffset)
		bytes = append(bytes, op.CtrlOffset)
		bytes = append(bytes, op.SlotsOffset)
		bytes = binary.LittleEndian.AppendUint16(bytes, op.SlotSize)
		bytes = append(bytes, op.KeyInSlotOffset)
		bytes = binary.LittleEndian.AppendUint16(bytes, op.ValInSlotOffset)
		bytes = append(bytes, op.TableGroupsFieldOffset)
		bytes = append(bytes, op.GroupsDataFieldOffset)
		bytes = append(bytes, op.GroupsLenMaskFieldOffset)
		bytes = binary.LittleEndian.AppendUint16(bytes, op.GroupByteSize)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.HeaderByteSize)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ExprStatusIdx)
		existenceOnly := uint8(0)
		if op.ExistenceOnly {
			existenceOnly = 1
		}
		bytes = append(bytes, existenceOnly)
		bytes = binary.LittleEndian.AppendUint16(bytes, uint16(len(op.KeyData)))
		bytes = append(bytes, op.KeyData...)
		return staticInstruction{
			opcode: OpcodeSwissMapSetup,
			bytes:  bytes,
		}

	case SwissMapAesencOp:
		return staticInstruction{opcode: OpcodeSwissMapAesenc}

	case SwissMapHashFinishOp:
		return staticInstruction{opcode: OpcodeSwissMapHashFinish}

	case SwissMapProbeOp:
		return staticInstruction{opcode: OpcodeSwissMapProbe}

	case SwissMapCheckSlotOp:
		return staticInstruction{opcode: OpcodeSwissMapCheckSlot}

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

	case CondNotOp:
		return staticInstruction{
			opcode: OpcodeCondNot,
			bytes:  []byte{},
		}

	case CondJumpOp:
		opcode := OpcodeCondJumpIfFalse
		if op.Cond {
			opcode = OpcodeCondJumpIfTrue
		}
		return jumpInstruction{
			opcode:     opcode,
			functionID: functionID,
			label:      op.Label,
		}

	case CondLabelOp:
		return labelMarker{functionID: functionID, id: op.ID}

	case ConditionStateInitOp:
		return staticInstruction{
			opcode: OpcodeConditionStateInit,
			bytes:  []byte{},
		}

	case ConditionLeafRecordOp:
		return staticInstruction{
			opcode: OpcodeConditionLeafRecord,
			bytes:  []byte{op.LeafIdx},
		}

	case ConditionLeafLoadOp:
		// Encoded as: opcode + uint8 leaf_idx + uint32 error_target.
		// We compose it using leafLoadInstruction below so the layout
		// pass can resolve the label PC.
		return leafLoadInstruction{
			functionID: functionID,
			leafIdx:    op.LeafIdx,
			label:      op.Label,
		}

	case ConditionCheckPreserveErrorOp:
		return staticInstruction{
			opcode: OpcodeConditionCheckPreserveError,
			bytes:  []byte{},
		}

	case ConditionLeafCompleteOp:
		return staticInstruction{
			opcode: OpcodeConditionLeafComplete,
			bytes:  []byte{},
		}

	case ExprAdvanceOffsetOp:
		return staticInstruction{
			opcode: OpcodeExprAdvanceOffset,
			bytes:  binary.LittleEndian.AppendUint32(nil, op.Offset),
		}

	case ExprLoadAddressOp:
		// kind (u8) + u32 cfa_offset + u32 pointer_bias
		bytes := make([]byte, 0, 9)
		bytes = append(bytes, uint8(op.LocationKind))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.CfaOffset)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.PointerBias)
		return staticInstruction{
			opcode: OpcodeExprLoadAddress,
			bytes:  bytes,
		}

	case ArrayLoopBeginOp:
		// u8 quantifier + u32 elem_size + u32 compile_time_len + u32 end_label_pc
		prefix := make([]byte, 0, 1+2*4)
		prefix = append(prefix, uint8(op.Quantifier))
		prefix = binary.LittleEndian.AppendUint32(prefix, op.ElemByteSize)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.CompileTimeLen)
		return paramJumpInstruction{
			opcode:     OpcodeArrayLoopBegin,
			functionID: functionID,
			prefix:     prefix,
			label:      op.EndLabel,
		}

	case ArrayLoopEndOp:
		// u32 body_label_pc (quantifier / elem_size come from slice_loop_state).
		return paramJumpInstruction{
			opcode:     OpcodeArrayLoopEnd,
			functionID: functionID,
			label:      op.BodyLabel,
		}

	case SliceLoopBeginOp:
		// u8 quantifier + u32 elem_size + u32 end_label_pc
		prefix := make([]byte, 0, 1+4)
		prefix = append(prefix, uint8(op.Quantifier))
		prefix = binary.LittleEndian.AppendUint32(prefix, op.ElemByteSize)
		return paramJumpInstruction{
			opcode:     OpcodeSliceLoopBegin,
			functionID: functionID,
			prefix:     prefix,
			label:      op.EndLabel,
		}

	case SliceLoopEndOp:
		// u32 body_label_pc (quantifier / elem_size come from slice_loop_state).
		return paramJumpInstruction{
			opcode:     OpcodeSliceLoopEnd,
			functionID: functionID,
			label:      op.BodyLabel,
		}

	case SwissMapLoopBeginOp:
		// u8 quantifier + u32 key_size + u32 val_size
		// + 5 u8 offsets (dir_ptr, dir_len, ctrl, slots, key_in_slot)
		// + u16 val_in_slot + u16 slot_size + u16 group_byte_size
		// + 3 u8 offsets (table_groups, groups_data, groups_len_mask)
		// + u32 end_label_pc.
		prefix := make([]byte, 0, 32)
		prefix = append(prefix, uint8(op.Quantifier))
		prefix = binary.LittleEndian.AppendUint32(prefix, op.KeyByteSize)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.ValByteSize)
		prefix = append(prefix,
			op.DirPtrOffset, op.DirLenOffset,
			op.CtrlOffset, op.SlotsOffset, op.KeyInSlotOffset,
		)
		prefix = binary.LittleEndian.AppendUint16(prefix, op.ValInSlotOffset)
		prefix = binary.LittleEndian.AppendUint16(prefix, op.SlotSize)
		prefix = binary.LittleEndian.AppendUint16(prefix, op.GroupByteSize)
		prefix = append(prefix,
			op.TableGroupsFieldOffset,
			op.GroupsDataFieldOffset,
			op.GroupsLenMaskFieldOffset,
		)
		return paramJumpInstruction{
			opcode:     OpcodeSwissMapLoopBegin,
			functionID: functionID,
			prefix:     prefix,
			label:      op.EndLabel,
		}

	case SwissMapLoopEndOp:
		// u32 body_label_pc (quantifier / key / val come from swissmap_loop_state).
		return paramJumpInstruction{
			opcode:     OpcodeSwissMapLoopEnd,
			functionID: functionID,
			label:      op.BodyLabel,
		}

	case PanicUnwindPrepareOp:
		return staticInstruction{
			opcode: OpcodePanicUnwindPrepare,
			bytes:  []byte{},
		}

	case PanicUnwindEvictSlotsOp:
		return staticInstruction{
			opcode: OpcodePanicUnwindEvictSlots,
			bytes:  []byte{},
		}

	case EmitFilterSliceMarkerOp:
		// u32 filter_data_type_id + u32 elem_byte_size
		bytes := make([]byte, 0, 8)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.FilterDataTypeID))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ElemByteSize)
		return staticInstruction{
			opcode: OpcodeEmitFilterSliceMarker,
			bytes:  bytes,
		}

	case EmitFilterMapMarkerOp:
		// u32 filter_data_type_id + u32 swiss_header_size + u32 used_field_offset
		bytes := make([]byte, 0, 12)
		bytes = binary.LittleEndian.AppendUint32(bytes, uint32(op.FilterDataTypeID))
		bytes = binary.LittleEndian.AppendUint32(bytes, op.SwissHeaderSize)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.UsedFieldOffset)
		return staticInstruction{
			opcode: OpcodeEmitFilterMapMarker,
			bytes:  bytes,
		}

	case InitFilterSliceLoopOp:
		// u32 elem_byte_size + u32 iter_scratch_budget + u32 end_label_pc
		prefix := make([]byte, 0, 8)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.ElemByteSize)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.IterScratchBudget)
		return paramJumpInstruction{
			opcode:     OpcodeInitFilterSliceLoop,
			functionID: functionID,
			prefix:     prefix,
			label:      op.EndLabel,
		}

	case EmitFilterSliceElementOp:
		// u32 elem_byte_size
		return staticInstruction{
			opcode: OpcodeEmitFilterSliceElement,
			bytes:  binary.LittleEndian.AppendUint32(nil, op.ElemByteSize),
		}

	case FilterSliceAdvanceOp:
		// u32 elem_byte_size + u32 body_label_pc
		prefix := make([]byte, 0, 4)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.ElemByteSize)
		return paramJumpInstruction{
			opcode:     OpcodeFilterSliceAdvance,
			functionID: functionID,
			prefix:     prefix,
			label:      op.BodyLabel,
		}

	case InitFilterMapLoopOp:
		// u32 key_byte_size + u32 val_byte_size + u32 val_offset_in_pair +
		// u32 iter_scratch_budget + swiss-map layout immediates +
		// u32 end_label_pc
		prefix := make([]byte, 0, 36)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.KeyByteSize)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.ValByteSize)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.ValOffsetInPair)
		prefix = binary.LittleEndian.AppendUint32(prefix, op.IterScratchBudget)
		prefix = append(prefix,
			op.DirPtrOffset, op.DirLenOffset,
			op.CtrlOffset, op.SlotsOffset, op.KeyInSlotOffset,
		)
		prefix = binary.LittleEndian.AppendUint16(prefix, op.ValInSlotOffset)
		prefix = binary.LittleEndian.AppendUint16(prefix, op.SlotSize)
		prefix = binary.LittleEndian.AppendUint16(prefix, op.GroupByteSize)
		prefix = append(prefix,
			op.TableGroupsFieldOffset,
			op.GroupsDataFieldOffset,
			op.GroupsLenMaskFieldOffset,
		)
		return paramJumpInstruction{
			opcode:     OpcodeInitFilterMapLoop,
			functionID: functionID,
			prefix:     prefix,
			label:      op.EndLabel,
		}

	case EmitFilterMapElementOp:
		// u32 key_byte_size + u32 val_byte_size + u32 val_offset_in_pair
		bytes := make([]byte, 0, 12)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.KeyByteSize)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ValByteSize)
		bytes = binary.LittleEndian.AppendUint32(bytes, op.ValOffsetInPair)
		return staticInstruction{
			opcode: OpcodeEmitFilterMapElement,
			bytes:  bytes,
		}

	case FilterMapAdvanceOp:
		// u32 body_label_pc (all other state lives in filter_loop_state).
		return paramJumpInstruction{
			opcode:     OpcodeFilterMapAdvance,
			functionID: functionID,
			label:      op.BodyLabel,
		}

	default:
		panic(fmt.Sprintf("unsupported op: %T", op))
	}
}
