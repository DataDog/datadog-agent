// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// xferToX generates code to load a register value into the X register.
func xferToX(cs *CompilerState, a *Arth) *SList {
	s := NewStmt(int(bpf.BPF_LDX|bpf.BPF_MEM), uint32(a.Regno))
	return s
}

// xferToA generates code to load a register value into the A register.
func xferToA(cs *CompilerState, a *Arth) *SList {
	s := NewStmt(int(bpf.BPF_LD|bpf.BPF_MEM), uint32(a.Regno))
	return s
}

// GenLoadi generates code to load an immediate value into a virtual register.
// Port of gen_loadi_internal() from gencode.c.
func GenLoadi(cs *CompilerState, val uint32) *Arth {
	reg := cs.AllocReg()
	if reg < 0 {
		return nil
	}

	a := &Arth{}
	s := NewStmt(int(bpf.BPF_LD|bpf.BPF_IMM), val)
	s.Next = NewStmt(int(bpf.BPF_ST), uint32(reg))
	a.S = s
	a.Regno = reg
	return a
}

// GenLoadlen generates code to load the packet length into a virtual register.
// Port of gen_loadlen() from gencode.c.
func GenLoadlen(cs *CompilerState) *Arth {
	reg := cs.AllocReg()
	if reg < 0 {
		return nil
	}

	a := &Arth{}
	s := NewStmt(int(bpf.BPF_LD|bpf.BPF_LEN), 0)
	s.Next = NewStmt(int(bpf.BPF_ST), uint32(reg))
	a.S = s
	a.Regno = reg
	return a
}

// GenNeg negates an arithmetic expression.
// Port of gen_neg() from gencode.c.
func GenNeg(cs *CompilerState, a *Arth) *Arth {
	if a == nil {
		return nil
	}
	s := xferToA(cs, a)
	Sappend(a.S, s)
	s = NewStmt(int(bpf.BPF_ALU|bpf.BPF_NEG), 0)
	Sappend(a.S, s)
	s = NewStmt(int(bpf.BPF_ST), uint32(a.Regno))
	Sappend(a.S, s)
	return a
}

// GenArth generates code for an arithmetic operation between two expressions.
// code is the BPF ALU operation (BPF_ADD, BPF_SUB, etc.).
// Port of gen_arth() from gencode.c.
func GenArth(cs *CompilerState, code int, a0, a1 *Arth) *Arth {
	if a0 == nil || a1 == nil {
		return nil
	}

	// Check for constant division/modulus by zero and large shifts
	if code == int(bpf.BPF_DIV) {
		if a1.S != nil && a1.S.S.Code == int(bpf.BPF_LD|bpf.BPF_IMM) && a1.S.S.K == 0 {
			cs.SetError(fmt.Errorf("division by zero"))
			return nil
		}
	} else if code == int(bpf.BPF_MOD) {
		if a1.S != nil && a1.S.S.Code == int(bpf.BPF_LD|bpf.BPF_IMM) && a1.S.S.K == 0 {
			cs.SetError(fmt.Errorf("modulus by zero"))
			return nil
		}
	} else if code == int(bpf.BPF_LSH) || code == int(bpf.BPF_RSH) {
		if a1.S != nil && a1.S.S.Code == int(bpf.BPF_LD|bpf.BPF_IMM) && a1.S.S.K > 31 {
			cs.SetError(fmt.Errorf("shift by more than 31 bits"))
			return nil
		}
	}

	// Transfer a1 to X, a0 to A, perform operation
	s0 := xferToX(cs, a1)
	s1 := xferToA(cs, a0)
	s2 := NewStmt(int(bpf.BPF_ALU|bpf.BPF_X|uint16(code)), 0)

	Sappend(s1, s2)
	Sappend(s0, s1)
	Sappend(a1.S, s0)
	Sappend(a0.S, a1.S)

	cs.FreeReg(a0.Regno)
	cs.FreeReg(a1.Regno)

	s0 = NewStmt(int(bpf.BPF_ST), 0)
	a0.Regno = cs.AllocReg()
	s0.S.K = uint32(a0.Regno)
	Sappend(a0.S, s0)

	return a0
}

// GenRelation generates code for a comparison expression.
// code is BPF_JEQ, BPF_JGT, or BPF_JGE. reversed flips the sense.
// Port of gen_relation_internal() from gencode.c.
func GenRelation(cs *CompilerState, code int, a0, a1 *Arth, reversed int) *Block {
	if a0 == nil || a1 == nil {
		return nil
	}

	s0 := xferToX(cs, a1)
	s1 := xferToA(cs, a0)

	var b *Block
	if code == int(bpf.BPF_JEQ) {
		// For equality, subtract X from A, then compare with 0
		s2 := NewStmt(int(bpf.BPF_ALU|bpf.BPF_SUB|bpf.BPF_X), 0)
		Sappend(s1, s2)
		b = cs.NewBlock(JmpCode(code), 0)
	} else {
		b = cs.NewBlock(int(bpf.BPF_JMP|uint16(code)|bpf.BPF_X), 0)
	}

	if reversed != 0 {
		GenNot(b)
	}

	Sappend(s0, s1)
	Sappend(a1.S, s0)
	Sappend(a0.S, a1.S)

	b.Stmts = a0.S

	cs.FreeReg(a0.Regno)
	cs.FreeReg(a1.Regno)

	// AND together protocol checks from both operands
	var tmp *Block
	if a0.B != nil {
		if a1.B != nil {
			GenAnd(a0.B, a1.B)
			tmp = a1.B
		} else {
			tmp = a0.B
		}
	} else {
		tmp = a1.B
	}

	if tmp != nil {
		GenAnd(tmp, b)
	}

	return b
}

// GenLoad generates code to load a value from the packet at a computed offset.
// proto is the protocol layer, inst is the offset expression, size is 1/2/4.
// Port of gen_load_internal() from gencode.c.
func GenLoad(cs *CompilerState, proto int, inst *Arth, size uint32) *Arth {
	if inst == nil {
		return nil
	}

	var sizeCode uint16
	switch size {
	case 1:
		sizeCode = bpf.BPF_B
	case 2:
		sizeCode = bpf.BPF_H
	case 4:
		sizeCode = bpf.BPF_W
	default:
		cs.SetError(fmt.Errorf("data size must be 1, 2, or 4"))
		return nil
	}

	regno := cs.AllocReg()
	cs.FreeReg(inst.Regno)

	switch proto {
	case QLink:
		// Offset relative to link-layer header
		s := genAbsOffsetVarpart(cs, &cs.OffLinkhdr)
		if s != nil {
			Sappend(s, xferToA(cs, inst))
			Sappend(s, NewStmt(int(bpf.BPF_ALU|bpf.BPF_ADD|bpf.BPF_X), 0))
			Sappend(s, NewStmt(int(bpf.BPF_MISC|bpf.BPF_TAX), 0))
		} else {
			s = xferToX(cs, inst)
		}
		tmp := NewStmt(int(bpf.BPF_LD|bpf.BPF_IND|sizeCode), uint32(cs.OffLinkhdr.ConstPart))
		Sappend(s, tmp)
		Sappend(inst.S, s)

	case QIP, QARP, QRARP, QAtalk, QDecnet, QSCA, QLat, QMoprc, QMopdl, QIPv6:
		// Offset relative to network-layer header
		s := genAbsOffsetVarpart(cs, &cs.OffLinkpl)
		if s != nil {
			Sappend(s, xferToA(cs, inst))
			Sappend(s, NewStmt(int(bpf.BPF_ALU|bpf.BPF_ADD|bpf.BPF_X), 0))
			Sappend(s, NewStmt(int(bpf.BPF_MISC|bpf.BPF_TAX), 0))
		} else {
			s = xferToX(cs, inst)
		}
		tmp := NewStmt(int(bpf.BPF_LD|bpf.BPF_IND|sizeCode),
			uint32(cs.OffLinkpl.ConstPart+cs.OffNl))
		Sappend(s, tmp)
		Sappend(inst.S, s)

		// Add protocol check
		b := GenProtoAbbrev(cs, proto)
		if inst.B != nil && b != nil {
			GenAnd(inst.B, b)
		}
		inst.B = b

	case QSCTP, QTCP, QUDP, QICMP, QIGMP, QIGRP, QPIM, QVRRP, QCARP:
		// Offset relative to transport-layer header
		s := genLoadxIPhdrlen(cs)
		Sappend(s, xferToA(cs, inst))
		Sappend(s, NewStmt(int(bpf.BPF_ALU|bpf.BPF_ADD|bpf.BPF_X), 0))
		Sappend(s, NewStmt(int(bpf.BPF_MISC|bpf.BPF_TAX), 0))
		tmp := NewStmt(int(bpf.BPF_LD|bpf.BPF_IND|sizeCode),
			uint32(cs.OffLinkpl.ConstPart+cs.OffNl))
		Sappend(s, tmp)
		Sappend(inst.S, s)

		// Protocol check: must be IP + correct transport + not fragmented
		b := genIPfrag(cs)
		GenAnd(GenProtoAbbrev(cs, proto), b)
		if inst.B != nil {
			GenAnd(inst.B, b)
		}
		GenAnd(GenProtoAbbrev(cs, QIP), b)
		inst.B = b

	case QICMPv6:
		// ICMPv6: offset relative to IPv6 payload + 40 (fixed IPv6 header)
		b := GenProtoAbbrev(cs, QIPv6)
		if inst.B != nil {
			GenAnd(inst.B, b)
		}
		inst.B = b
		b = GenCmp(cs, OrLinkpl, 6, bpf.BPF_B, 58) // next-header == ICMPv6
		if inst.B != nil {
			GenAnd(inst.B, b)
		}
		inst.B = b

		s := genAbsOffsetVarpart(cs, &cs.OffLinkpl)
		if s != nil {
			Sappend(s, xferToA(cs, inst))
			Sappend(s, NewStmt(int(bpf.BPF_ALU|bpf.BPF_ADD|bpf.BPF_X), 0))
			Sappend(s, NewStmt(int(bpf.BPF_MISC|bpf.BPF_TAX), 0))
		} else {
			s = xferToX(cs, inst)
		}
		tmp := NewStmt(int(bpf.BPF_LD|bpf.BPF_IND|sizeCode),
			uint32(cs.OffLinkpl.ConstPart+cs.OffNl+40))
		Sappend(s, tmp)
		Sappend(inst.S, s)

	default:
		cs.SetError(fmt.Errorf("unsupported index operation for protocol %d", proto))
		return nil
	}

	inst.Regno = regno
	s := NewStmt(int(bpf.BPF_ST), uint32(regno))
	Sappend(inst.S, s)

	return inst
}

// GenLess generates code for "less N" (packet length <= N).
// Port of gen_less() from gencode.c.
func GenLess(cs *CompilerState, n int) *Block {
	b := genLen(cs, int(bpf.BPF_JGT), n)
	GenNot(b)
	return b
}

// GenGreater generates code for "greater N" (packet length >= N).
// Port of gen_greater() from gencode.c.
func GenGreater(cs *CompilerState, n int) *Block {
	return genLen(cs, int(bpf.BPF_JGE), n)
}

// genLen generates a packet length comparison.
// Port of gen_len() from gencode.c.
func genLen(cs *CompilerState, jmp int, n int) *Block {
	s := NewStmt(int(bpf.BPF_LD|bpf.BPF_LEN), 0)
	b := cs.NewBlock(JmpCode(jmp), uint32(n))
	b.Stmts = s
	return b
}

// GenBroadcast generates code for broadcast matching.
// Port of gen_broadcast() from gencode.c (Ethernet-focused subset).
func GenBroadcast(cs *CompilerState, proto int) *Block {
	ebroadcast := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	switch proto {
	case QDefault, QLink:
		switch cs.Linktype {
		case DLTEN10MB:
			return GenEhostop(cs, ebroadcast, QDst)
		default:
			cs.SetError(fmt.Errorf("not a broadcast link"))
			return nil
		}

	case QIP:
		if cs.Netmask == 0xFFFFFFFF {
			cs.SetError(fmt.Errorf("netmask not known, so 'ip broadcast' not supported"))
			return nil
		}
		b0 := GenLinktype(cs, EthertypeIP)
		hostmask := ^cs.Netmask
		// Match all-zeros host part OR all-ones host part
		b1 := GenMcmp(cs, OrLinkpl, 16, bpf.BPF_W, 0, hostmask)
		b2 := GenMcmp(cs, OrLinkpl, 16, bpf.BPF_W, 0xFFFFFFFF&hostmask, hostmask)
		GenOr(b1, b2)
		GenAnd(b0, b2)
		return b2

	default:
		cs.SetError(fmt.Errorf("only link-layer/IP broadcast filters supported"))
		return nil
	}
}

// GenMulticast generates code for multicast matching.
// Port of gen_multicast() from gencode.c (Ethernet-focused subset).
func GenMulticast(cs *CompilerState, proto int) *Block {
	switch proto {
	case QDefault, QLink:
		switch cs.Linktype {
		case DLTEN10MB:
			// ether[0] & 1 != 0
			return GenMcmp(cs, OrLinkhdr, 0, bpf.BPF_B, 0x01, 0x01)
		default:
			cs.SetError(fmt.Errorf("link-layer multicast not supported for DLT %d", cs.Linktype))
			return nil
		}

	case QIP:
		b0 := GenLinktype(cs, EthertypeIP)
		// Destination IP starts with 1110 (224.0.0.0/4)
		b1 := GenMcmp(cs, OrLinkpl, 16, bpf.BPF_B, 0xe0, 0xf0)
		GenAnd(b0, b1)
		return b1

	case QIPv6:
		b0 := GenLinktype(cs, EthertypeIPv6)
		// IPv6 multicast: dst addr first byte == 0xff
		b1 := GenCmp(cs, OrLinkpl, 24, bpf.BPF_B, 0xff)
		GenAnd(b0, b1)
		return b1

	default:
		cs.SetError(fmt.Errorf("multicast not supported for protocol %d", proto))
		return nil
	}
}

// GenByteop generates code for a byte operation expression.
// Port of gen_byteop() from gencode.c.
func GenByteop(cs *CompilerState, op int, idx int, val uint32) *Block {
	switch op {
	case '&':
		return GenMcmp(cs, OrLinkpl, uint32(idx), bpf.BPF_B, val, val)
	case '|':
		cs.SetError(fmt.Errorf("'byte' OR operation not supported"))
		return nil
	case '<':
		return GenCmpLt(cs, OrLinkpl, uint32(idx), bpf.BPF_B, val)
	case '>':
		return GenCmpGt(cs, OrLinkpl, uint32(idx), bpf.BPF_B, val)
	case '=':
		return GenCmp(cs, OrLinkpl, uint32(idx), bpf.BPF_B, val)
	default:
		cs.SetError(fmt.Errorf("unknown byte operation '%c'", op))
		return nil
	}
}

// GenInbound generates code for inbound/outbound matching.
// Port of gen_inbound() from gencode.c.
func GenInbound(cs *CompilerState, dir int) *Block {
	cs.SetError(fmt.Errorf("inbound/outbound not supported on this link type"))
	return nil
}

// GenIfindex generates code for interface index matching.
func GenIfindex(cs *CompilerState, ifindex int) *Block {
	cs.SetError(fmt.Errorf("ifindex not supported on this link type"))
	return nil
}
