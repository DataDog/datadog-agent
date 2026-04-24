// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import "github.com/DataDog/datadog-agent/pkg/libpcap/bpf"

// JmpCode constructs a BPF_JMP|BPF_K opcode with the given condition.
// Port of the JMP(c) macro from gencode.c.
func JmpCode(c int) int {
	return c | int(bpf.BPF_JMP) | int(bpf.BPF_K)
}

// Backpatch resolves unresolved jump targets in a block list.
// Blocks are linked through their JT/JF fields based on the sense flag.
// Port of backpatch() from gencode.c.
func Backpatch(list *Block, target *Block) {
	for list != nil {
		var next *Block
		if !list.Sense {
			next = JT(list)
			SetJT(list, target)
		} else {
			next = JF(list)
			SetJF(list, target)
		}
		list = next
	}
}

// Merge concatenates two block lists for backpatching.
// Port of merge() from gencode.c.
func Merge(b0, b1 *Block) {
	// Find end of b0's list
	cur := b0
	for {
		if !cur.Sense {
			if JT(cur) == nil {
				SetJT(cur, b1)
				return
			}
			cur = JT(cur)
		} else {
			if JF(cur) == nil {
				SetJF(cur, b1)
				return
			}
			cur = JF(cur)
		}
	}
}

// GenAnd combines two blocks with logical AND.
// If b0 matches, continue to b1; if b0 doesn't match, the whole thing fails.
// Port of gen_and() from gencode.c.
func GenAnd(b0, b1 *Block) {
	if b0 == nil || b1 == nil {
		return
	}
	Backpatch(b0, b1.Head)
	b0.Sense = !b0.Sense
	b1.Sense = !b1.Sense
	Merge(b1, b0)
	b1.Sense = !b1.Sense
	b1.Head = b0.Head
}

// GenOr combines two blocks with logical OR.
// If b0 matches, the whole thing matches; if not, try b1.
// Port of gen_or() from gencode.c.
func GenOr(b0, b1 *Block) {
	if b0 == nil || b1 == nil {
		return
	}
	b0.Sense = !b0.Sense
	Backpatch(b0, b1.Head)
	b0.Sense = !b0.Sense
	Merge(b1, b0)
	b1.Head = b0.Head
}

// GenNot negates a block by flipping its sense.
// Port of gen_not() from gencode.c.
func GenNot(b *Block) {
	if b != nil {
		b.Sense = !b.Sense
	}
}

// GenRetBlk creates a return block that returns the given value.
// Port of gen_retblk_internal() from gencode.c.
func GenRetBlk(cs *CompilerState, v int) *Block {
	b := cs.NewBlock(int(bpf.BPF_RET|bpf.BPF_K), uint32(v))
	return b
}

// FinishParse finalizes the compilation of a filter expression.
// It backpatches the accept (return snaplen) and reject (return 0) blocks
// to the root of the CFG.
// Port of finish_parse() from gencode.c.
func FinishParse(cs *CompilerState, p *Block) error {
	if p == nil {
		// Empty filter — accept all
		cs.IC.Root = GenRetBlk(cs, cs.Snaplen)
		return cs.Err
	}

	// Backpatch true (match) to return snaplen
	Backpatch(p, GenRetBlk(cs, cs.Snaplen))
	// Backpatch false (no match) to return 0
	p.Sense = !p.Sense
	Backpatch(p, GenRetBlk(cs, 0))

	cs.IC.Root = p.Head
	return cs.Err
}
