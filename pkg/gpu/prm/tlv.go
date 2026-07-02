// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package prm

import (
	"encoding/binary"
	"fmt"
)

const (
	tlvTypeEnd = 0x0
	tlvTypeOp  = 0x1
	tlvTypeReg = 0x3

	opTLVLenDwords               = 4
	opTLVClassReg                = 1
	opTLVMethodQuery             = 1
	regTLVHeaderLenDwords        = 1
	endTLVLenDwords              = 1
	dwordSizeBytes               = 4
	ppcntRegID                   = 0x5008
	ppcntSizeBytes               = 256
	opTLVRequestBit       uint32 = 0
)

// PackPPCNTTLV builds the PPCNT request TLV for the given group and port.
func PackPPCNTTLV(group, port uint32) []byte {
	return packTLV(ppcntRegID, ppcntSizeBytes, createPPCNTByteArray(group, port))
}

func createPPCNTByteArray(group, port uint32) []byte {
	payload := make([]byte, ppcntSizeBytes)
	ppcntVal := (group & 0x3F) | (port << 16)
	binary.BigEndian.PutUint32(payload[0:dwordSizeBytes], ppcntVal)
	return payload
}

func packTLV(regID uint32, regSize int, regPayload []byte) []byte {
	ret := make([]byte, 0, (opTLVLenDwords+regTLVHeaderLenDwords+endTLVLenDwords)*dwordSizeBytes+regSize)
	ret = append(ret, packOpTLV(regID)...)
	ret = append(ret, packDWord(makeTLVHeader(tlvTypeReg, uint32(regSize/dwordSizeBytes+regTLVHeaderLenDwords)))...)
	if regPayload != nil {
		ret = append(ret, regPayload...)
	} else {
		ret = append(ret, make([]byte, regSize)...)
	}
	ret = append(ret, packDWord(makeTLVHeader(tlvTypeEnd, endTLVLenDwords))...)
	return ret
}

func packOpTLV(regID uint32) []byte {
	ret := make([]byte, 0, opTLVLenDwords*dwordSizeBytes)
	ret = append(ret, packDWord(makeTLVHeader(tlvTypeOp, opTLVLenDwords))...)
	ret = append(ret, packDWord(makeOpMethodAndReg(regID))...)
	ret = append(ret, packDWord(0)...)
	ret = append(ret, packDWord(0)...)
	return ret
}

func packDWord(value uint32) []byte {
	ret := make([]byte, dwordSizeBytes)
	binary.BigEndian.PutUint32(ret, value)
	return ret
}

func makeTLVHeader(tType, length uint32) uint32 {
	return ((tType & 0x1F) << 27) | ((length & 0x7FF) << 16)
}

func makeOpMethodAndReg(regID uint32) uint32 {
	return (opTLVClassReg & 0xF) |
		((opTLVMethodQuery & 0x7F) << 8) |
		((opTLVRequestBit & 0x1) << 15) |
		((regID & 0xFFFF) << 16)
}

// UnpackTLV parses a PPCNT response and returns its counters.
func UnpackTLV(buffer []byte) (map[string]uint64, error) {
	offset := opTLVLenDwords * dwordSizeBytes
	if len(buffer) < offset+dwordSizeBytes {
		return nil, fmt.Errorf("PRM response too short: %d", len(buffer))
	}

	regHeader := binary.BigEndian.Uint32(buffer[offset : offset+dwordSizeBytes])
	regLenDwords := (regHeader >> 16) & 0x7FF
	if regLenDwords < regTLVHeaderLenDwords {
		return nil, fmt.Errorf("invalid register TLV length: %d", regLenDwords)
	}

	offset += dwordSizeBytes
	regPayloadBytes := int(regLenDwords-regTLVHeaderLenDwords) * dwordSizeBytes
	if len(buffer) < offset+regPayloadBytes {
		return nil, fmt.Errorf("PRM register payload truncated: need %d bytes, have %d", offset+regPayloadBytes, len(buffer))
	}

	return unpackPPCNT(buffer[offset : offset+regPayloadBytes])
}

func unpackPPCNT(buffer []byte) (map[string]uint64, error) {
	if len(buffer) < 2*dwordSizeBytes {
		return nil, fmt.Errorf("PPCNT payload too short: %d", len(buffer))
	}

	group := binary.BigEndian.Uint32(buffer[0:dwordSizeBytes]) & 0x3F
	if group != PPCNTGroupPLR {
		return nil, fmt.Errorf("unexpected PPCNT group 0x%x", group)
	}

	return unpackPPCNTGrpX22PLR(buffer[2*dwordSizeBytes:])
}

func unpackPPCNTGrpX22PLR(buffer []byte) (map[string]uint64, error) {
	requiredLen := len(PLRCounterFields) * 2 * dwordSizeBytes
	if len(buffer) < requiredLen {
		return nil, fmt.Errorf("PLR payload too short: need %d bytes, have %d", requiredLen, len(buffer))
	}

	metrics := make(map[string]uint64, len(PLRCounterFields))
	offset := 0
	for _, field := range PLRCounterFields {
		high := binary.BigEndian.Uint32(buffer[offset : offset+dwordSizeBytes])
		offset += dwordSizeBytes
		low := binary.BigEndian.Uint32(buffer[offset : offset+dwordSizeBytes])
		offset += dwordSizeBytes
		metrics[field] = (uint64(high) << 32) | uint64(low)
	}

	return metrics, nil
}
