// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"encoding/binary"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// GenCmp generates a block that compares a packet field with a value.
// It loads a value of the given size at the given offset (relative to offrel)
// and checks if it equals v.
// Port of gen_cmp() from gencode.c.
func GenCmp(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16, v uint32) *Block {
	return GenNcmp(cs, offrel, offset, size, 0xffffffff, int(bpf.BPF_JEQ), 0, v)
}

// GenCmpGt generates a "greater than" comparison.
func GenCmpGt(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16, v uint32) *Block {
	return GenNcmp(cs, offrel, offset, size, 0xffffffff, int(bpf.BPF_JGT), 0, v)
}

// GenCmpGe generates a "greater than or equal" comparison.
func GenCmpGe(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16, v uint32) *Block {
	return GenNcmp(cs, offrel, offset, size, 0xffffffff, int(bpf.BPF_JGE), 0, v)
}

// GenCmpLt generates a "less than" comparison (reversed JGE).
func GenCmpLt(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16, v uint32) *Block {
	return GenNcmp(cs, offrel, offset, size, 0xffffffff, int(bpf.BPF_JGE), 1, v)
}

// GenCmpLe generates a "less than or equal" comparison (reversed JGT).
func GenCmpLe(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16, v uint32) *Block {
	return GenNcmp(cs, offrel, offset, size, 0xffffffff, int(bpf.BPF_JGT), 1, v)
}

// GenMcmp generates a masked comparison: (load & mask) == v.
// Port of gen_mcmp() from gencode.c.
func GenMcmp(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16, v uint32, mask uint32) *Block {
	return GenNcmp(cs, offrel, offset, size, mask, int(bpf.BPF_JEQ), 0, v)
}

// GenBcmp generates a byte-array comparison.
// It breaks the comparison into 4-byte, 2-byte, and 1-byte chunks.
// Port of gen_bcmp() from gencode.c.
func GenBcmp(cs *CompilerState, offrel OffsetRel, offset uint32, data []byte) *Block {
	size := len(data)
	var b *Block

	// Compare 4-byte chunks from the end
	for size >= 4 {
		p := data[size-4 : size]
		v := binary.BigEndian.Uint32(p)
		tmp := GenCmp(cs, offrel, offset+uint32(size-4), bpf.BPF_W, v)
		if b != nil {
			GenAnd(b, tmp)
		}
		b = tmp
		size -= 4
	}

	// Compare 2-byte chunks
	for size >= 2 {
		p := data[size-2 : size]
		v := uint32(binary.BigEndian.Uint16(p))
		tmp := GenCmp(cs, offrel, offset+uint32(size-2), bpf.BPF_H, v)
		if b != nil {
			GenAnd(b, tmp)
		}
		b = tmp
		size -= 2
	}

	// Compare remaining byte
	if size > 0 {
		tmp := GenCmp(cs, offrel, offset, bpf.BPF_B, uint32(data[0]))
		if b != nil {
			GenAnd(b, tmp)
		}
		b = tmp
	}

	return b
}

// GenNcmp generates a masked comparison with a configurable jump type.
// It loads a value at offset, optionally masks it, and creates a conditional
// jump block.
// Port of gen_ncmp() from gencode.c.
func GenNcmp(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16, mask uint32, jtype int, reverse int, v uint32) *Block {
	s := GenLoadA(cs, offrel, offset, size)
	if s == nil {
		return nil
	}

	if mask != 0xffffffff {
		s2 := NewStmt(int(bpf.BPF_ALU|bpf.BPF_AND|bpf.BPF_K), mask)
		Sappend(s, s2)
	}

	b := cs.NewBlock(JmpCode(jtype), v)
	b.Stmts = s
	if reverse != 0 && (jtype == int(bpf.BPF_JGT) || jtype == int(bpf.BPF_JGE)) {
		GenNot(b)
	}
	return b
}

// GenLoadA generates instructions to load a packet value into the A register.
// The value is at the given offset relative to offrel, with the given size.
// Port of gen_load_a() from gencode.c.
func GenLoadA(cs *CompilerState, offrel OffsetRel, offset uint32, size uint16) *SList {
	switch offrel {
	case OrPacket:
		s := NewStmt(int(bpf.BPF_LD|bpf.BPF_ABS|size), offset)
		return s

	case OrLinkhdr:
		return genLoadAbsOffsetRel(cs, &cs.OffLinkhdr, offset, size)

	case OrPrevlinkhdr:
		return genLoadAbsOffsetRel(cs, &cs.OffPrevlinkhdr, offset, size)

	case OrLLC:
		return genLoadAbsOffsetRel(cs, &cs.OffLinkpl, offset, size)

	case OrPrevmplshdr:
		return genLoadAbsOffsetRel(cs, &cs.OffLinkpl, uint32(cs.OffNl-4)+offset, size)

	case OrLinkpl:
		return genLoadAbsOffsetRel(cs, &cs.OffLinkpl, uint32(cs.OffNl)+offset, size)

	case OrLinkplNosnap:
		return genLoadAbsOffsetRel(cs, &cs.OffLinkpl, uint32(cs.OffNlNosnap)+offset, size)

	case OrLinktype:
		return genLoadAbsOffsetRel(cs, &cs.OffLinktype, offset, size)

	case OrTranIPv4:
		// Load IP header length into X, then do indirect load
		s := genLoadxIPhdrlen(cs)
		s2 := NewStmt(int(bpf.BPF_LD|bpf.BPF_IND|size),
			uint32(cs.OffLinkpl.ConstPart)+uint32(cs.OffNl)+offset)
		Sappend(s, s2)
		return s

	case OrTranIPv6:
		// IPv6 header is fixed 40 bytes
		return genLoadAbsOffsetRel(cs, &cs.OffLinkpl, uint32(cs.OffNl)+40+offset, size)
	}

	return nil
}

// genLoadAbsOffsetRel loads a value at a possibly variable absolute offset.
// If the offset has a variable part, generates an indirect load using X register.
// Port of gen_load_absoffsetrel() from gencode.c.
func genLoadAbsOffsetRel(cs *CompilerState, absOff *AbsOffset, offset uint32, size uint16) *SList {
	s := genAbsOffsetVarpart(cs, absOff)

	if s != nil {
		// Variable offset — use indirect load via X register
		s2 := NewStmt(int(bpf.BPF_LD|bpf.BPF_IND|size),
			uint32(absOff.ConstPart)+offset)
		Sappend(s, s2)
	} else {
		// Fixed offset — use absolute load
		s = NewStmt(int(bpf.BPF_LD|bpf.BPF_ABS|size),
			uint32(absOff.ConstPart)+offset)
	}
	return s
}

// genAbsOffsetVarpart generates code to load the variable part of an
// absolute offset into the X register (if any).
// Port of gen_abs_offset_varpart() from gencode.c.
func genAbsOffsetVarpart(cs *CompilerState, off *AbsOffset) *SList {
	if off.IsVariable {
		if off.Reg == -1 {
			off.Reg = cs.AllocReg()
		}
		s := NewStmt(int(bpf.BPF_LDX|bpf.BPF_MEM), uint32(off.Reg))
		return s
	}
	return nil
}

// genLoadxIPhdrlen generates code to load the IPv4 header length into
// the X register (for transport-layer offset calculations).
// Port of gen_loadx_iphdrlen() from gencode.c.
func genLoadxIPhdrlen(cs *CompilerState) *SList {
	s := genAbsOffsetVarpart(cs, &cs.OffLinkpl)
	if s != nil {
		// Variable link-layer payload offset — can't use MSH, must compute manually
		s2 := NewStmt(int(bpf.BPF_LD|bpf.BPF_IND|bpf.BPF_B),
			uint32(cs.OffLinkpl.ConstPart)+uint32(cs.OffNl))
		Sappend(s, s2)
		s2 = NewStmt(int(bpf.BPF_ALU|bpf.BPF_AND|bpf.BPF_K), 0xf)
		Sappend(s, s2)
		s2 = NewStmt(int(bpf.BPF_ALU|bpf.BPF_LSH|bpf.BPF_K), 2)
		Sappend(s, s2)
		// Add the variable part of the link-layer payload offset
		s2 = NewStmt(int(bpf.BPF_ALU|bpf.BPF_ADD|bpf.BPF_X), 0)
		Sappend(s, s2)
		// Move result to X
		s2 = NewStmt(int(bpf.BPF_MISC|bpf.BPF_TAX), 0)
		Sappend(s, s2)
	} else {
		// Fixed link-layer payload offset — use MSH for IP header length
		s = NewStmt(int(bpf.BPF_LDX|bpf.BPF_MSH|bpf.BPF_B),
			uint32(cs.OffLinkpl.ConstPart)+uint32(cs.OffNl))
	}
	return s
}
