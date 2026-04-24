// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

func newTestOptState() *OptState {
	os := &OptState{}
	os.vmap = make([]vmapinfo, 1000)
	os.vnodes = make([]valnode, 1000)
	os.initVal()
	return os
}

func TestOptStmtLoadImm(t *testing.T) {
	os := newTestOptState()
	val := make([]uint32, codegen.NAtoms)

	s := codegen.Stmt{Code: int(bpf.BPF_LD | bpf.BPF_IMM), K: 42}
	os.OptStmt(&s, val, true)

	if val[AAtom] == ValUnknown {
		t.Error("A should have a value after LD IMM")
	}
	if !os.IsConst(val[AAtom]) {
		t.Error("A should be const after LD #42")
	}
	if os.ConstVal(val[AAtom]) != 42 {
		t.Errorf("A const = %d, want 42", os.ConstVal(val[AAtom]))
	}
}

func TestOptStmtConstantFold(t *testing.T) {
	os := newTestOptState()
	val := make([]uint32, codegen.NAtoms)

	// Load 10 into A
	s1 := codegen.Stmt{Code: int(bpf.BPF_LD | bpf.BPF_IMM), K: 10}
	os.OptStmt(&s1, val, true)

	// Add 5 to A — should constant fold to LD #15
	s2 := codegen.Stmt{Code: int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_K), K: 5}
	os.OptStmt(&s2, val, true)

	if s2.Code != int(bpf.BPF_LD|bpf.BPF_IMM) {
		t.Errorf("expected constant fold to LD IMM, got code=%#x", s2.Code)
	}
	if s2.K != 15 {
		t.Errorf("expected folded value 15, got %d", s2.K)
	}
}

func TestOptStmtAddZero(t *testing.T) {
	os := newTestOptState()
	val := make([]uint32, codegen.NAtoms)

	// Load something into A
	s1 := codegen.Stmt{Code: int(bpf.BPF_LD | bpf.BPF_ABS | bpf.BPF_H), K: 12}
	os.OptStmt(&s1, val, true)

	// Add 0 to A — should be eliminated (NOP)
	s2 := codegen.Stmt{Code: int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_K), K: 0}
	os.OptStmt(&s2, val, true)

	if s2.Code != codegen.NOP {
		t.Errorf("add #0 should be NOP, got code=%#x", s2.Code)
	}
}

func TestOptStmtMulZero(t *testing.T) {
	os := newTestOptState()
	val := make([]uint32, codegen.NAtoms)

	// Load something into A
	s1 := codegen.Stmt{Code: int(bpf.BPF_LD | bpf.BPF_ABS | bpf.BPF_H), K: 12}
	os.OptStmt(&s1, val, true)

	// Mul 0 → should become LD #0
	s2 := codegen.Stmt{Code: int(bpf.BPF_ALU | bpf.BPF_MUL | bpf.BPF_K), K: 0}
	os.OptStmt(&s2, val, true)

	if s2.Code != int(bpf.BPF_LD|bpf.BPF_IMM) || s2.K != 0 {
		t.Errorf("mul #0 should be LD #0, got code=%#x k=%d", s2.Code, s2.K)
	}
}

func TestOptStmtIndirectToAbsolute(t *testing.T) {
	os := newTestOptState()
	val := make([]uint32, codegen.NAtoms)

	// Load constant 14 into X
	s1 := codegen.Stmt{Code: int(bpf.BPF_LDX | bpf.BPF_IMM), K: 14}
	os.OptStmt(&s1, val, true)

	// Indirect load with X → should become absolute load with offset+14
	s2 := codegen.Stmt{Code: int(bpf.BPF_LD | bpf.BPF_IND | bpf.BPF_H), K: 0}
	os.OptStmt(&s2, val, true)

	if bpf.Mode(uint16(s2.Code)) != bpf.BPF_ABS {
		t.Errorf("indirect with const X should become absolute, got mode=%#x", bpf.Mode(uint16(s2.Code)))
	}
	if s2.K != 14 {
		t.Errorf("absolute offset should be 14, got %d", s2.K)
	}
}

func TestOptPeepStTax(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	// Create a block with: st M[0]; ldx M[0]
	b := cs.NewBlock(codegen.JmpCode(int(bpf.BPF_JEQ)), 0)
	s1 := codegen.NewStmt(int(bpf.BPF_ST), 0)
	s2 := codegen.NewStmt(int(bpf.BPF_LDX|bpf.BPF_MEM), 0)
	s1.Next = s2
	b.Stmts = s1

	os := newTestOptState()
	os.OptPeep(b)

	// s2 should have been changed to TAX
	if s2.S.Code != int(bpf.BPF_MISC|bpf.BPF_TAX) {
		t.Errorf("st M[0]; ldx M[0] should become st M[0]; tax, got code=%#x", s2.S.Code)
	}
}

func TestOptPeepLdTax(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	// Create a block with: ld #k; tax
	b := cs.NewBlock(codegen.JmpCode(int(bpf.BPF_JEQ)), 0)
	s1 := codegen.NewStmt(int(bpf.BPF_LD|bpf.BPF_IMM), 42)
	s2 := codegen.NewStmt(int(bpf.BPF_MISC|bpf.BPF_TAX), 0)
	s1.Next = s2
	b.Stmts = s1

	os := newTestOptState()
	os.OptPeep(b)

	// Should become: ldx #k; txa
	if s1.S.Code != int(bpf.BPF_LDX|bpf.BPF_IMM) {
		t.Errorf("ld #k should become ldx #k, got %#x", s1.S.Code)
	}
	if s2.S.Code != int(bpf.BPF_MISC|bpf.BPF_TXA) {
		t.Errorf("tax should become txa, got %#x", s2.S.Code)
	}
}

func TestOptDeadstores(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	// Create block: ld #1; ld #2; ret A
	// The first ld #1 is dead (overwritten by ld #2)
	b := cs.NewBlock(int(bpf.BPF_RET|bpf.BPF_A), 0)
	s1 := codegen.NewStmt(int(bpf.BPF_LD|bpf.BPF_IMM), 1)
	s2 := codegen.NewStmt(int(bpf.BPF_LD|bpf.BPF_IMM), 2)
	s1.Next = s2
	b.Stmts = s1
	b.OutUse = AtomMask(AAtom) // A is used on exit

	os := newTestOptState()
	os.OptDeadstores(b)

	if s1.S.Code != codegen.NOP {
		t.Errorf("first ld #1 should be NOP (dead), got %#x", s1.S.Code)
	}
	if s2.S.Code == codegen.NOP {
		t.Error("second ld #2 should NOT be NOP")
	}
}

func TestFoldOp(t *testing.T) {
	os := newTestOptState()
	v10 := os.K(10)
	v3 := os.K(3)

	s := codegen.Stmt{Code: int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_K)}
	os.foldOp(&s, v10, v3)

	if s.Code != int(bpf.BPF_LD|bpf.BPF_IMM) {
		t.Errorf("foldOp should produce LD IMM, got %#x", s.Code)
	}
	if s.K != 13 {
		t.Errorf("10+3 = %d, want 13", s.K)
	}
}
