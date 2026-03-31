// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// Atomuse returns the register/memory atom READ by statement s.
// Returns -1 if no atom is read.
// Port of atomuse() from optimize.c.
func Atomuse(s *codegen.Stmt) int {
	c := s.Code
	if c == codegen.NOP {
		return -1
	}

	switch bpf.Class(uint16(c)) {
	case bpf.BPF_RET:
		if bpf.Rval(uint16(c)) == bpf.BPF_A {
			return AAtom
		}
		return -1

	case bpf.BPF_LD, bpf.BPF_LDX:
		if bpf.Mode(uint16(c)) == bpf.BPF_IND {
			return XAtom
		}
		if bpf.Mode(uint16(c)) == bpf.BPF_MEM {
			return int(s.K)
		}
		return -1

	case bpf.BPF_ST:
		return AAtom

	case bpf.BPF_STX:
		return XAtom

	case bpf.BPF_JMP, bpf.BPF_ALU:
		if bpf.Src(uint16(c)) == bpf.BPF_X {
			return AXAtom
		}
		return AAtom

	case bpf.BPF_MISC:
		if bpf.MiscOp(uint16(c)) == bpf.BPF_TXA {
			return XAtom
		}
		return AAtom
	}
	return -1
}

// Atomdef returns the register/memory atom WRITTEN by statement s.
// Returns -1 if no atom is defined.
// Port of atomdef() from optimize.c.
func Atomdef(s *codegen.Stmt) int {
	if s.Code == codegen.NOP {
		return -1
	}

	switch bpf.Class(uint16(s.Code)) {
	case bpf.BPF_LD, bpf.BPF_ALU:
		return AAtom

	case bpf.BPF_LDX:
		return XAtom

	case bpf.BPF_ST, bpf.BPF_STX:
		return int(s.K)

	case bpf.BPF_MISC:
		if bpf.MiscOp(uint16(s.Code)) == bpf.BPF_TAX {
			return XAtom
		}
		return AAtom // TXA
	}
	return -1
}

// Slength counts the number of non-NOP statements in an SList chain.
func Slength(s *codegen.SList) uint {
	var n uint
	for ; s != nil; s = s.Next {
		if s.S.Code != codegen.NOP {
			n++
		}
	}
	return n
}

// CountBlocks counts the number of reachable blocks via DFS.
func CountBlocks(ic *codegen.ICode, p *codegen.Block) int {
	if p == nil || ic.IsMarked(p) {
		return 0
	}
	ic.MarkBlock(p)
	return CountBlocks(ic, codegen.JT(p)) + CountBlocks(ic, codegen.JF(p)) + 1
}

// NumberBlocks assigns sequential IDs to all reachable blocks via DFS
// and populates the blocks slice.
func NumberBlocks(ic *codegen.ICode, p *codegen.Block, blocks []*codegen.Block, n *uint) {
	if p == nil || ic.IsMarked(p) {
		return
	}
	ic.MarkBlock(p)
	id := *n
	*n++
	p.ID = id
	if int(id) < len(blocks) {
		blocks[id] = p
	}
	NumberBlocks(ic, codegen.JT(p), blocks, n)
	NumberBlocks(ic, codegen.JF(p), blocks, n)
}

// FindInedges populates the InEdges list for each block (predecessors).
// Port of find_inedges() from optimize.c.
func FindInedges(os *OptState) {
	// Clear all in-edge lists
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if b != nil {
			b.InEdges = nil
		}
	}

	// For each block, add its edges to successors' in-edge lists
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if b == nil {
			continue
		}
		if codegen.JT(b) != nil {
			b.Et.Next = codegen.JT(b).InEdges
			codegen.JT(b).InEdges = &b.Et
		}
		if codegen.JF(b) != nil {
			b.Ef.Next = codegen.JF(b).InEdges
			codegen.JF(b).InEdges = &b.Ef
		}
	}
}

// Bitset operations on uint32 atom sets (used for def/kill/use tracking).
// These operate on the 32-bit atomset fields in Block.

// AtomMask returns the bitmask for atom n.
func AtomMask(n int) uint32 {
	return 1 << uint(n)
}

// AtomElem tests if atom n is in the set d.
func AtomElem(d uint32, n int) bool {
	return d&AtomMask(n) != 0
}
