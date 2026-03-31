// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package optimizer implements BPF program optimization.
// It is a port of libpcap's optimize.c.
package optimizer

import (
	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// Register atom indices for use-def analysis.
const (
	AAtom  = bpf.BPF_MEMWORDS     // Accumulator register (index 16)
	XAtom  = bpf.BPF_MEMWORDS + 1 // Index register (index 17)
	AXAtom = codegen.NAtoms        // Both A and X (index 18, sentinel for use-def)
)

// Modulus for value numbering hash table.
const hashModulus = 213

// ValUnknown indicates an unknown value number.
const ValUnknown = 0

// valnode represents an entry in the value numbering hash table.
// Each unique {opcode, operands} triple gets a unique value number.
type valnode struct {
	code   int
	v0, v1 uint32
	val    uint32 // assigned value number (never 0)
	next   *valnode
}

// vmapinfo maps a value number to an optional constant.
type vmapinfo struct {
	isConst  bool
	constVal uint32
}

// OptState holds all optimizer state.
// Port of opt_state_t from optimize.c.
type OptState struct {
	Done                       bool
	NonBranchMovementPerformed bool

	NBlocks uint
	Blocks  []*codegen.Block
	NEdges  uint
	Edges   []*codegen.Edge

	// Bit-vector sizing
	NodeWords uint
	EdgeWords uint

	// Level-ordered block lists (levels[i] = linked list at level i via Block.Link)
	Levels []*codegen.Block

	// Value numbering
	hashtbl  [hashModulus]*valnode
	curval   uint32
	maxval   uint32
	vmap     []vmapinfo
	vnodes   []valnode
	nextNode int

	// Error
	Err error
}

// initVal resets the value numbering hash table for a new optimization pass.
func (os *OptState) initVal() {
	os.curval = 0
	os.nextNode = 0
	for i := range os.hashtbl {
		os.hashtbl[i] = nil
	}
}

// F looks up or assigns a value number for the triple (code, v0, v1).
// Port of F() from optimize.c.
func (os *OptState) F(code int, v0, v1 uint32) uint32 {
	hash := (uint(code) ^ (uint(v0) << 4) ^ (uint(v1) << 8)) % hashModulus

	for p := os.hashtbl[hash]; p != nil; p = p.next {
		if p.code == code && p.v0 == v0 && p.v1 == v1 {
			return p.val
		}
	}

	// Not found — allocate new value number
	os.curval++
	val := os.curval

	if os.nextNode >= len(os.vnodes) {
		// Shouldn't happen if properly sized
		return val
	}

	p := &os.vnodes[os.nextNode]
	os.nextNode++
	p.code = code
	p.v0 = v0
	p.v1 = v1
	p.val = val
	p.next = os.hashtbl[hash]
	os.hashtbl[hash] = p

	// If this is a load immediate, record the constant
	if int(val) < len(os.vmap) {
		if code == int(bpf.BPF_LD|bpf.BPF_IMM|bpf.BPF_W) {
			os.vmap[val] = vmapinfo{isConst: true, constVal: v0}
		} else {
			os.vmap[val] = vmapinfo{}
		}
	}

	return val
}

// K returns the value number for a constant.
func (os *OptState) K(v uint32) uint32 {
	return os.F(int(bpf.BPF_LD|bpf.BPF_IMM|bpf.BPF_W), v, 0)
}

// IsConst returns whether a value number represents a known constant.
func (os *OptState) IsConst(v uint32) bool {
	if int(v) < len(os.vmap) {
		return os.vmap[v].isConst
	}
	return false
}

// ConstVal returns the constant value for a value number.
func (os *OptState) ConstVal(v uint32) uint32 {
	if int(v) < len(os.vmap) {
		return os.vmap[v].constVal
	}
	return 0
}

// vstore stores a value number, optionally eliminating redundant stores.
func (os *OptState) vstore(s *codegen.Stmt, valp *uint32, newval uint32, alter bool) {
	if alter && newval != ValUnknown && os.IsConst(newval) && *valp == newval {
		s.Code = codegen.NOP
	} else {
		*valp = newval
	}
}
