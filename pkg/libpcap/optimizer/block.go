// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"math/bits"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// OptBlk optimizes a single basic block given data flow values from predecessors.
// Port of opt_blk() from optimize.c.
func (os *OptState) OptBlk(b *codegen.Block, doStmts bool) {
	// Initialize atom values from predecessors
	p := b.InEdges
	if p == nil {
		// No predecessors — everything is unknown
		for i := range b.Val {
			b.Val[i] = 0
		}
	} else {
		// Inherit from first predecessor
		copy(b.Val[:], p.Pred.Val[:])
		// Merge with other predecessors: if different, mark unknown
		for p = p.Next; p != nil; p = p.Next {
			for i := 0; i < codegen.NAtoms; i++ {
				if b.Val[i] != p.Pred.Val[i] {
					b.Val[i] = 0
				}
			}
		}
	}

	aval := b.Val[AAtom]
	xval := b.Val[XAtom]

	// Process all statements
	for s := b.Stmts; s != nil; s = s.Next {
		os.OptStmt(&s.S, b.Val[:], doStmts)
	}

	// Special case: eliminate all statements if the block doesn't change
	// anything used, or if it's a return block
	if doStmts &&
		((b.OutUse == 0 &&
			aval != ValUnknown && b.Val[AAtom] == aval &&
			xval != ValUnknown && b.Val[XAtom] == xval) ||
			bpf.Class(uint16(b.S.Code)) == bpf.BPF_RET) {
		if b.Stmts != nil {
			b.Stmts = nil
			os.NonBranchMovementPerformed = true
			os.Done = false
		}
	} else {
		os.OptPeep(b)
		os.OptDeadstores(b)
	}

	// Set up values for branch optimizer
	if bpf.Src(uint16(b.S.Code)) == bpf.BPF_K {
		b.Oval = int(os.K(b.S.K))
	} else {
		b.Oval = int(b.Val[XAtom])
	}
	b.Et.Code = b.S.Code
	b.Ef.Code = -b.S.Code
}

// OptBlks runs block optimization on all blocks in level order.
// Port of opt_blks() from optimize.c.
func (os *OptState) OptBlks(ic *codegen.ICode, doStmts bool) {
	os.initVal()
	maxlevel := ic.Root.Level

	FindInedges(os)
	for i := maxlevel; i >= 0; i-- {
		for p := os.Levels[i]; p != nil; p = p.Link {
			os.OptBlk(p, doStmts)
		}
	}

	if doStmts {
		return
	}

	// Jump optimization
	for i := 1; i <= maxlevel; i++ {
		for p := os.Levels[i]; p != nil; p = p.Link {
			os.optJ(&p.Et)
			os.optJ(&p.Ef)
		}
	}

	// Predicate assertion (pullup) — these are in Task 5
	FindInedges(os)
	for i := 1; i <= maxlevel; i++ {
		for p := os.Levels[i]; p != nil; p = p.Link {
			os.orPullup(p, ic.Root)
			os.andPullup(p, ic.Root)
		}
	}
}

// useConflict returns true if any register used on exit from succ has
// a different value in b vs succ.
// Port of use_conflict() from optimize.c.
func useConflict(b, succ *codegen.Block) bool {
	use := succ.OutUse
	if use == 0 {
		return false
	}
	for atom := 0; atom < codegen.NAtoms; atom++ {
		if AtomElem(use, atom) {
			if b.Val[atom] != succ.Val[atom] {
				return true
			}
		}
	}
	return false
}

// foldEdge determines if we can replace the successor of ep with a child
// of that successor's block. Returns the replacement target, or nil.
// Port of fold_edge() from optimize.c.
func foldEdge(child *codegen.Block, ep *codegen.Edge) *codegen.Block {
	code := ep.Code
	var sense bool
	if code < 0 {
		code = -code
		sense = false
	} else {
		sense = true
	}

	if child.S.Code != code {
		return nil
	}

	aval0 := child.Val[AAtom]
	oval0 := uint32(child.Oval)
	aval1 := ep.Pred.Val[AAtom]
	oval1 := uint32(ep.Pred.Oval)

	if aval0 != aval1 {
		return nil
	}

	if oval0 == oval1 {
		if sense {
			return codegen.JT(child)
		}
		return codegen.JF(child)
	}

	if sense && code == int(bpf.BPF_JMP|bpf.BPF_JEQ|bpf.BPF_K) {
		return codegen.JF(child)
	}

	return nil
}

// optJ optimizes a single edge's target using dominator information.
// Port of opt_j() from optimize.c.
func (os *OptState) optJ(ep *codegen.Edge) {
	if codegen.JT(ep.Succ) == nil {
		return
	}

	// Common successor elimination
	if codegen.JT(ep.Succ) == codegen.JF(ep.Succ) {
		if !useConflict(ep.Pred, codegen.JT(ep.Succ)) {
			os.NonBranchMovementPerformed = true
			os.Done = false
			ep.Succ = codegen.JT(ep.Succ)
		}
	}

	// Edge dominator threading
	for {
		changed := false
		for i := uint(0); i < os.EdgeWords; i++ {
			x := ep.Edom[i]
			for x != 0 {
				k := uint(bits.TrailingZeros32(x))
				x &^= 1 << k
				k += i * 32

				if k >= os.NEdges {
					continue
				}
				target := foldEdge(ep.Succ, os.Edges[k])
				if target != nil && !useConflict(ep.Pred, target) {
					os.Done = false
					ep.Succ = target
					if codegen.JT(target) != nil {
						changed = true
						break
					}
					return
				}
			}
			if changed {
				break
			}
		}
		if !changed {
			return
		}
	}
}

// orPullup and andPullup are predicate assertion optimizations.
// Stub implementations for now — will be filled in Task 5.
func (os *OptState) orPullup(b *codegen.Block, root *codegen.Block)  {}
func (os *OptState) andPullup(b *codegen.Block, root *codegen.Block) {}
