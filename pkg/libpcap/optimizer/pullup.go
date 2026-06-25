// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// orPullupImpl implements the OR predicate assertion optimization.
// Port of or_pullup() from optimize.c.
func (os *OptState) orPullupImpl(b *codegen.Block, root *codegen.Block) {
	ep := b.InEdges
	if ep == nil {
		return
	}

	// All predecessors must load the same A value
	val := ep.Pred.Val[AAtom]
	for ep = ep.Next; ep != nil; ep = ep.Next {
		if val != ep.Pred.Val[AAtom] {
			return
		}
	}

	// Find whether first predecessor reaches b via true or false branch
	var diffp **codegen.Block
	if codegen.JT(b.InEdges.Pred) == b {
		diffp = jtPtr(b.InEdges.Pred)
	} else {
		diffp = jfPtr(b.InEdges.Pred)
	}

	// Follow false chain looking for a different A value
	atTop := true
	for {
		if *diffp == nil {
			return
		}
		if codegen.JT(*diffp) != codegen.JT(b) {
			return
		}
		if !setMember((*diffp).Dom, uint(b.ID)) {
			return
		}
		if (*diffp).Val[AAtom] != val {
			break
		}
		diffp = jfPtr(*diffp)
		atTop = false
	}

	// Search further down for a block with the original A value
	samep := jfPtr(*diffp)
	for {
		if *samep == nil {
			return
		}
		if codegen.JT(*samep) != codegen.JT(b) {
			return
		}
		if !setMember((*samep).Dom, uint(b.ID)) {
			return
		}
		if (*samep).Val[AAtom] == val {
			break
		}
		samep = jfPtr(*samep)
	}

	// Pull up the node
	pull := *samep
	*samep = codegen.JF(pull)
	codegen.SetJF(pull, *diffp)

	if atTop {
		for ep = b.InEdges; ep != nil; ep = ep.Next {
			if codegen.JT(ep.Pred) == b {
				codegen.SetJT(ep.Pred, pull)
			} else {
				codegen.SetJF(ep.Pred, pull)
			}
		}
	} else {
		*diffp = pull
	}

	os.Done = false
	FindDom(os, root)
}

// andPullupImpl implements the AND predicate assertion optimization.
// Port of and_pullup() from optimize.c.
func (os *OptState) andPullupImpl(b *codegen.Block, root *codegen.Block) {
	ep := b.InEdges
	if ep == nil {
		return
	}

	val := ep.Pred.Val[AAtom]
	for ep = ep.Next; ep != nil; ep = ep.Next {
		if val != ep.Pred.Val[AAtom] {
			return
		}
	}

	var diffp **codegen.Block
	if codegen.JT(b.InEdges.Pred) == b {
		diffp = jtPtr(b.InEdges.Pred)
	} else {
		diffp = jfPtr(b.InEdges.Pred)
	}

	// Follow TRUE chain (differs from or_pullup which follows false)
	atTop := true
	for {
		if *diffp == nil {
			return
		}
		if codegen.JF(*diffp) != codegen.JF(b) {
			return
		}
		if !setMember((*diffp).Dom, uint(b.ID)) {
			return
		}
		if (*diffp).Val[AAtom] != val {
			break
		}
		diffp = jtPtr(*diffp)
		atTop = false
	}

	samep := jtPtr(*diffp)
	for {
		if *samep == nil {
			return
		}
		if codegen.JF(*samep) != codegen.JF(b) {
			return
		}
		if !setMember((*samep).Dom, uint(b.ID)) {
			return
		}
		if (*samep).Val[AAtom] == val {
			break
		}
		samep = jtPtr(*samep)
	}

	pull := *samep
	*samep = codegen.JT(pull)
	codegen.SetJT(pull, *diffp)

	if atTop {
		for ep = b.InEdges; ep != nil; ep = ep.Next {
			if codegen.JT(ep.Pred) == b {
				codegen.SetJT(ep.Pred, pull)
			} else {
				codegen.SetJF(ep.Pred, pull)
			}
		}
	} else {
		*diffp = pull
	}

	os.Done = false
	FindDom(os, root)
}

// jtPtr returns a pointer to the JT field of a block's Et edge.
// This allows modifying the successor through a double pointer.
func jtPtr(b *codegen.Block) **codegen.Block {
	return &b.Et.Succ
}

// jfPtr returns a pointer to the JF field of a block's Ef edge.
func jfPtr(b *codegen.Block) **codegen.Block {
	return &b.Ef.Succ
}

// makeMarks marks all reachable blocks from p via DFS.
func makeMarks(ic *codegen.ICode, p *codegen.Block) {
	if p == nil || ic.IsMarked(p) {
		return
	}
	ic.MarkBlock(p)
	if bpf.Class(uint16(p.S.Code)) != bpf.BPF_RET {
		makeMarks(ic, codegen.JT(p))
		makeMarks(ic, codegen.JF(p))
	}
}

// markCode marks all reachable blocks in the CFG.
func markCode(ic *codegen.ICode) {
	ic.UnMarkAll()
	makeMarks(ic, ic.Root)
}

// eqSlist returns true if two statement lists are equivalent (ignoring NOPs).
func eqSlist(x, y *codegen.SList) bool {
	for {
		for x != nil && x.S.Code == codegen.NOP {
			x = x.Next
		}
		for y != nil && y.S.Code == codegen.NOP {
			y = y.Next
		}
		if x == nil {
			return y == nil
		}
		if y == nil {
			return false
		}
		if x.S.Code != y.S.Code || x.S.K != y.S.K {
			return false
		}
		x = x.Next
		y = y.Next
	}
}

// eqBlk returns true if two blocks are identical (same branch, same successors, same statements).
func eqBlk(b0, b1 *codegen.Block) bool {
	if b0.S.Code == b1.S.Code &&
		b0.S.K == b1.S.K &&
		codegen.JT(b0) == codegen.JT(b1) &&
		codegen.JF(b0) == codegen.JF(b1) {
		return eqSlist(b0.Stmts, b1.Stmts)
	}
	return false
}

// InternBlocks unifies identical blocks in the CFG.
// Port of intern_blocks() from optimize.c.
func (os *OptState) InternBlocks(ic *codegen.ICode) {
	for {
		done := true
		for i := uint(0); i < os.NBlocks; i++ {
			os.Blocks[i].Link = nil
		}

		markCode(ic)

		for i := os.NBlocks - 1; i > 0; {
			i--
			if !ic.IsMarked(os.Blocks[i]) {
				continue
			}
			for j := i + 1; j < os.NBlocks; j++ {
				if !ic.IsMarked(os.Blocks[j]) {
					continue
				}
				if eqBlk(os.Blocks[i], os.Blocks[j]) {
					if os.Blocks[j].Link != nil {
						os.Blocks[i].Link = os.Blocks[j].Link
					} else {
						os.Blocks[i].Link = os.Blocks[j]
					}
					break
				}
			}
		}

		for i := uint(0); i < os.NBlocks; i++ {
			p := os.Blocks[i]
			if codegen.JT(p) == nil {
				continue
			}
			if codegen.JT(p).Link != nil {
				done = false
				codegen.SetJT(p, codegen.JT(p).Link)
			}
			if codegen.JF(p).Link != nil {
				done = false
				codegen.SetJF(p, codegen.JF(p).Link)
			}
		}
		if done {
			return
		}
	}
}

// OptRoot simplifies the root block by skipping blocks with JT==JF
// and removing statements from return blocks.
// Port of opt_root() from optimize.c.
func OptRoot(b **codegen.Block) {
	s := (*b).Stmts
	(*b).Stmts = nil

	// Skip blocks where both branches go to the same place
	for bpf.Class(uint16((*b).S.Code)) == bpf.BPF_JMP && codegen.JT(*b) == codegen.JF(*b) {
		*b = codegen.JT(*b)
	}

	// Prepend saved statements
	tmp := (*b).Stmts
	if tmp != nil {
		codegen.Sappend(s, tmp)
	}
	(*b).Stmts = s

	// If root is a return, no statements needed
	if bpf.Class(uint16((*b).S.Code)) == bpf.BPF_RET {
		(*b).Stmts = nil
	}
}
