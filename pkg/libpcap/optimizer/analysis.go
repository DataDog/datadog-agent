// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// Bitset operations on []uint32 slices.

func setInsert(p []uint32, a uint) {
	p[a/32] |= 1 << (a % 32)
}

func setMember(p []uint32, a uint) bool {
	return p[a/32]&(1<<(a%32)) != 0
}

func setIntersect(a, b []uint32, n uint) {
	for i := uint(0); i < n; i++ {
		a[i] &= b[i]
	}
}

func setUnion(a, b []uint32, n uint) {
	for i := uint(0); i < n; i++ {
		a[i] |= b[i]
	}
}

// FindLevels assigns level numbers to blocks based on graph depth.
// Level 0 = leaves (return blocks), level N = root.
// Builds level-linked lists in os.Levels via Block.Link.
// Port of find_levels() from optimize.c.
func FindLevels(os *OptState, ic *codegen.ICode) {
	// Clear levels
	for i := range os.Levels {
		os.Levels[i] = nil
	}
	ic.UnMarkAll()
	findLevelsR(os, ic, ic.Root)
}

func findLevelsR(os *OptState, ic *codegen.ICode, b *codegen.Block) {
	if b == nil || ic.IsMarked(b) {
		return
	}
	ic.MarkBlock(b)
	b.Link = nil

	var level int
	if codegen.JT(b) != nil {
		findLevelsR(os, ic, codegen.JT(b))
		findLevelsR(os, ic, codegen.JF(b))
		jt := codegen.JT(b).Level
		jf := codegen.JF(b).Level
		if jt > jf {
			level = jt + 1
		} else {
			level = jf + 1
		}
	} else {
		level = 0
	}
	b.Level = level
	if level < len(os.Levels) {
		b.Link = os.Levels[level]
		os.Levels[level] = b
	}
}

// FindDom computes block dominator sets.
// A block X dominates block Y if every path from root to Y passes through X.
// Port of find_dom() from optimize.c.
func FindDom(os *OptState, root *codegen.Block) {
	// Initialize all dom sets to {all nodes}
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if b == nil {
			continue
		}
		for j := uint(0); j < os.NodeWords; j++ {
			b.Dom[j] = 0xFFFFFFFF
		}
	}

	// Root starts empty
	for j := uint(0); j < os.NodeWords; j++ {
		root.Dom[j] = 0
	}

	// Top-down: from root level to leaves
	for level := root.Level; level >= 0; level-- {
		for b := os.Levels[level]; b != nil; b = b.Link {
			setInsert(b.Dom, uint(b.ID))
			if codegen.JT(b) == nil {
				continue
			}
			setIntersect(codegen.JT(b).Dom, b.Dom, os.NodeWords)
			setIntersect(codegen.JF(b).Dom, b.Dom, os.NodeWords)
		}
	}
}

// FindEdom computes edge dominator sets.
// Port of find_edom() from optimize.c.
func FindEdom(os *OptState, root *codegen.Block) {
	// Initialize all edge dom sets to {all edges}
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if b == nil {
			continue
		}
		for j := uint(0); j < os.EdgeWords; j++ {
			b.Et.Edom[j] = 0xFFFFFFFF
			b.Ef.Edom[j] = 0xFFFFFFFF
		}
	}

	// Root edges start empty
	for j := uint(0); j < os.EdgeWords; j++ {
		root.Et.Edom[j] = 0
		root.Ef.Edom[j] = 0
	}

	// Top-down: propagate edge dominators
	for level := root.Level; level >= 0; level-- {
		for b := os.Levels[level]; b != nil; b = b.Link {
			propEdom(os, &b.Et)
			propEdom(os, &b.Ef)
		}
	}
}

func propEdom(os *OptState, ep *codegen.Edge) {
	setInsert(ep.Edom, uint(ep.ID))
	if ep.Succ != nil {
		setIntersect(ep.Succ.Et.Edom, ep.Edom, os.EdgeWords)
		setIntersect(ep.Succ.Ef.Edom, ep.Edom, os.EdgeWords)
	}
}

// FindClosure computes the backwards transitive closure.
// block.Closure = set of blocks that can reach this block.
// Port of find_closure() from optimize.c.
func FindClosure(os *OptState, root *codegen.Block) {
	// Initialize all closure sets to empty
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if b == nil {
			continue
		}
		for j := uint(0); j < os.NodeWords; j++ {
			b.Closure[j] = 0
		}
	}

	// Top-down: from root level to leaves
	for level := root.Level; level >= 0; level-- {
		for b := os.Levels[level]; b != nil; b = b.Link {
			setInsert(b.Closure, uint(b.ID))
			if codegen.JT(b) == nil {
				continue
			}
			setUnion(codegen.JT(b).Closure, b.Closure, os.NodeWords)
			setUnion(codegen.JF(b).Closure, b.Closure, os.NodeWords)
		}
	}
}

// ComputeLocalUD computes the local use/def/kill sets for a single block.
// Port of compute_local_ud() from optimize.c.
func ComputeLocalUD(b *codegen.Block) {
	var def, use, killed uint32

	for s := b.Stmts; s != nil; s = s.Next {
		if s.S.Code == codegen.NOP {
			continue
		}
		atom := Atomuse(&s.S)
		if atom >= 0 {
			if atom == AXAtom {
				if !AtomElem(def, XAtom) {
					use |= AtomMask(XAtom)
				}
				if !AtomElem(def, AAtom) {
					use |= AtomMask(AAtom)
				}
			} else if atom < codegen.NAtoms {
				if !AtomElem(def, atom) {
					use |= AtomMask(atom)
				}
			}
		}
		atom = Atomdef(&s.S)
		if atom >= 0 {
			if !AtomElem(use, atom) {
				killed |= AtomMask(atom)
			}
			def |= AtomMask(atom)
		}
	}

	// Branch statement also uses registers
	if bpf.Class(uint16(b.S.Code)) == bpf.BPF_JMP {
		atom := Atomuse(&b.S)
		if atom >= 0 {
			if atom == AXAtom {
				if !AtomElem(def, XAtom) {
					use |= AtomMask(XAtom)
				}
				if !AtomElem(def, AAtom) {
					use |= AtomMask(AAtom)
				}
			} else if atom < codegen.NAtoms {
				if !AtomElem(def, atom) {
					use |= AtomMask(atom)
				}
			}
		}
	}

	b.Def = def
	b.Kill = killed
	b.InUse = use
}

// FindUD computes use-def sets for all blocks.
// Pass 1: compute local use/def/kill for each block.
// Pass 2: propagate out_use and in_use bottom-up.
// Port of find_ud() from optimize.c.
func FindUD(os *OptState, root *codegen.Block) {
	maxlevel := root.Level

	// Pass 1: compute local use-def
	for i := maxlevel; i >= 0; i-- {
		for p := os.Levels[i]; p != nil; p = p.Link {
			ComputeLocalUD(p)
			p.OutUse = 0
		}
	}

	// Pass 2: propagate use information bottom-up
	for i := 1; i <= maxlevel; i++ {
		for p := os.Levels[i]; p != nil; p = p.Link {
			if codegen.JT(p) != nil {
				p.OutUse |= codegen.JT(p).InUse | codegen.JF(p).InUse
			}
			p.InUse |= p.OutUse &^ p.Kill
		}
	}
}

// OptInit initializes the optimizer state for the given intermediate code.
// Allocates all data structures.
// Port of opt_init() from optimize.c.
func OptInit(os *OptState, ic *codegen.ICode) error {
	// Count and number blocks
	ic.UnMarkAll()
	n := CountBlocks(ic, ic.Root)
	if n == 0 {
		return nil
	}
	os.NBlocks = uint(n)
	os.Blocks = make([]*codegen.Block, n)

	ic.UnMarkAll()
	var count uint
	NumberBlocks(ic, ic.Root, os.Blocks, &count)

	// Edges: 2 per block (et, ef)
	os.NEdges = 2 * os.NBlocks
	os.Edges = make([]*codegen.Edge, os.NEdges)
	edgeIdx := uint(0)
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if b == nil {
			continue
		}
		b.Et.ID = edgeIdx
		os.Edges[edgeIdx] = &b.Et
		edgeIdx++
		b.Ef.ID = edgeIdx
		os.Edges[edgeIdx] = &b.Ef
		edgeIdx++
	}

	// Levels array
	os.Levels = make([]*codegen.Block, os.NBlocks)

	// Bitset sizing
	os.NodeWords = (os.NBlocks + 31) / 32
	os.EdgeWords = (os.NEdges + 31) / 32

	// Allocate dom/closure/edom sets for each block and edge
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if b == nil {
			continue
		}
		b.Dom = make([]uint32, os.NodeWords)
		b.Closure = make([]uint32, os.NodeWords)
		b.Et.Edom = make([]uint32, os.EdgeWords)
		b.Ef.Edom = make([]uint32, os.EdgeWords)
	}

	// Count total statements for value numbering sizing
	ic.UnMarkAll()
	totalStmts := codegen.CountStmts(ic, ic.Root)
	maxval := totalStmts + os.NBlocks + 2 // some headroom
	os.vmap = make([]vmapinfo, maxval+1)
	os.vnodes = make([]valnode, maxval+1)
	os.maxval = uint32(maxval)

	return nil
}
