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

func TestAtomuse(t *testing.T) {
	tests := []struct {
		name string
		code int
		k    uint32
		want int
	}{
		{"NOP", codegen.NOP, 0, -1},
		{"RET K", int(bpf.BPF_RET | bpf.BPF_K), 0, -1},
		{"RET A", int(bpf.BPF_RET | bpf.BPF_A), 0, AAtom},
		{"LD ABS", int(bpf.BPF_LD | bpf.BPF_ABS | bpf.BPF_W), 0, -1},
		{"LD IND", int(bpf.BPF_LD | bpf.BPF_IND | bpf.BPF_W), 0, XAtom},
		{"LD MEM 3", int(bpf.BPF_LD | bpf.BPF_MEM), 3, 3},
		{"ST 5", int(bpf.BPF_ST), 5, AAtom},
		{"STX 5", int(bpf.BPF_STX), 5, XAtom},
		{"ALU ADD K", int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_K), 0, AAtom},
		{"ALU ADD X", int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_X), 0, AXAtom},
		{"JMP JEQ K", int(bpf.BPF_JMP | bpf.BPF_JEQ | bpf.BPF_K), 0, AAtom},
		{"JMP JEQ X", int(bpf.BPF_JMP | bpf.BPF_JEQ | bpf.BPF_X), 0, AXAtom},
		{"TAX", int(bpf.BPF_MISC | bpf.BPF_TAX), 0, AAtom},
		{"TXA", int(bpf.BPF_MISC | bpf.BPF_TXA), 0, XAtom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &codegen.Stmt{Code: tt.code, K: tt.k}
			got := Atomuse(s)
			if got != tt.want {
				t.Errorf("Atomuse(%s) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestAtomdef(t *testing.T) {
	tests := []struct {
		name string
		code int
		k    uint32
		want int
	}{
		{"NOP", codegen.NOP, 0, -1},
		{"LD", int(bpf.BPF_LD | bpf.BPF_ABS | bpf.BPF_W), 0, AAtom},
		{"LDX", int(bpf.BPF_LDX | bpf.BPF_IMM), 0, XAtom},
		{"ALU", int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_K), 0, AAtom},
		{"ST 5", int(bpf.BPF_ST), 5, 5},
		{"STX 3", int(bpf.BPF_STX), 3, 3},
		{"TAX", int(bpf.BPF_MISC | bpf.BPF_TAX), 0, XAtom},
		{"TXA", int(bpf.BPF_MISC | bpf.BPF_TXA), 0, AAtom},
		{"RET", int(bpf.BPF_RET | bpf.BPF_K), 0, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &codegen.Stmt{Code: tt.code, K: tt.k}
			got := Atomdef(s)
			if got != tt.want {
				t.Errorf("Atomdef(%s) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestValueNumbering(t *testing.T) {
	os := &OptState{}
	os.vmap = make([]vmapinfo, 1000)
	os.vnodes = make([]valnode, 1000)
	os.initVal()

	// First call should return value 1
	v1 := os.F(int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H), 12, 0)
	if v1 != 1 {
		t.Errorf("first F() = %d, want 1", v1)
	}

	// Same triple should return same value
	v2 := os.F(int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H), 12, 0)
	if v2 != v1 {
		t.Errorf("duplicate F() = %d, want %d", v2, v1)
	}

	// Different triple should return new value
	v3 := os.F(int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_B), 23, 0)
	if v3 == v1 {
		t.Errorf("different F() = %d, should differ from %d", v3, v1)
	}
}

func TestKConstant(t *testing.T) {
	os := &OptState{}
	os.vmap = make([]vmapinfo, 1000)
	os.vnodes = make([]valnode, 1000)
	os.initVal()

	v := os.K(42)
	if !os.IsConst(v) {
		t.Error("K(42) should be const")
	}
	if os.ConstVal(v) != 42 {
		t.Errorf("ConstVal = %d, want 42", os.ConstVal(v))
	}

	// Same constant should get same value number
	v2 := os.K(42)
	if v2 != v {
		t.Errorf("K(42) returned different values: %d vs %d", v, v2)
	}
}

func TestAtomMask(t *testing.T) {
	if AtomMask(0) != 1 {
		t.Errorf("AtomMask(0) = %d, want 1", AtomMask(0))
	}
	if AtomMask(3) != 8 {
		t.Errorf("AtomMask(3) = %d, want 8", AtomMask(3))
	}
	if !AtomElem(0xFF, 7) {
		t.Error("AtomElem(0xFF, 7) should be true")
	}
	if AtomElem(0x0F, 7) {
		t.Error("AtomElem(0x0F, 7) should be false")
	}
}

func TestSlength(t *testing.T) {
	s0 := codegen.NewStmt(int(bpf.BPF_LD|bpf.BPF_IMM), 10)
	s1 := codegen.NewStmt(codegen.NOP, 0)
	s2 := codegen.NewStmt(int(bpf.BPF_ST), 0)
	s0.Next = s1
	s1.Next = s2

	n := Slength(s0)
	if n != 2 {
		t.Errorf("Slength = %d, want 2 (NOP skipped)", n)
	}
}
