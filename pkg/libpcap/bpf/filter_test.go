// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

import (
	"encoding/binary"
	"testing"
)

func TestFilterReturnK(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_RET|BPF_K, 65535),
	}
	got := Filter(insns, nil, 0)
	if got != 65535 {
		t.Errorf("Filter = %d, want 65535", got)
	}
}

func TestFilterReturnZero(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_RET|BPF_K, 0),
	}
	got := Filter(insns, nil, 0)
	if got != 0 {
		t.Errorf("Filter = %d, want 0", got)
	}
}

func TestFilterNil(t *testing.T) {
	got := Filter(nil, []byte{1, 2, 3}, 3)
	if got != ^uint32(0) {
		t.Errorf("Filter(nil) = %d, want %d", got, ^uint32(0))
	}
}

func TestFilterLoadAbsWord(t *testing.T) {
	pkt := make([]byte, 4)
	binary.BigEndian.PutUint32(pkt, 0xDEADBEEF)
	insns := []Instruction{
		Stmt(BPF_LD|BPF_W|BPF_ABS, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, pkt, uint32(len(pkt)))
	if got != 0xDEADBEEF {
		t.Errorf("Filter = %#x, want 0xDEADBEEF", got)
	}
}

func TestFilterLoadAbsHalf(t *testing.T) {
	pkt := []byte{0x00, 0x00, 0x12, 0x34}
	insns := []Instruction{
		Stmt(BPF_LD|BPF_H|BPF_ABS, 2),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, pkt, uint32(len(pkt)))
	if got != 0x1234 {
		t.Errorf("Filter = %#x, want 0x1234", got)
	}
}

func TestFilterLoadAbsByte(t *testing.T) {
	pkt := []byte{0x00, 0xAB}
	insns := []Instruction{
		Stmt(BPF_LD|BPF_B|BPF_ABS, 1),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, pkt, uint32(len(pkt)))
	if got != 0xAB {
		t.Errorf("Filter = %#x, want 0xAB", got)
	}
}

func TestFilterOutOfBounds(t *testing.T) {
	pkt := []byte{0x01}
	insns := []Instruction{
		Stmt(BPF_LD|BPF_W|BPF_ABS, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, pkt, uint32(len(pkt)))
	if got != 0 {
		t.Errorf("Filter = %d, want 0 (out of bounds)", got)
	}
}

func TestFilterJumpEq(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_B|BPF_ABS, 0),
		Jump(BPF_JMP|BPF_JEQ|BPF_K, 0x42, 0, 1),
		Stmt(BPF_RET|BPF_K, 1),
		Stmt(BPF_RET|BPF_K, 0),
	}

	got := Filter(insns, []byte{0x42}, 1)
	if got != 1 {
		t.Errorf("Filter(match) = %d, want 1", got)
	}

	got = Filter(insns, []byte{0x00}, 1)
	if got != 0 {
		t.Errorf("Filter(no match) = %d, want 0", got)
	}
}

func TestFilterALU(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_IMM, 10),
		Stmt(BPF_ALU|BPF_ADD|BPF_K, 5),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, nil, 0)
	if got != 15 {
		t.Errorf("Filter = %d, want 15", got)
	}
}

func TestFilterDivByZeroX(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_IMM, 42),
		Stmt(BPF_LDX|BPF_IMM, 0),
		Stmt(BPF_ALU|BPF_DIV|BPF_X, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, nil, 0)
	if got != 0 {
		t.Errorf("Filter = %d, want 0 (div by zero)", got)
	}
}

func TestFilterMemory(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_IMM, 99),
		Stmt(BPF_ST, 0),
		Stmt(BPF_LDX|BPF_MEM, 0),
		Stmt(BPF_MISC|BPF_TXA, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, nil, 0)
	if got != 99 {
		t.Errorf("Filter = %d, want 99", got)
	}
}

func TestFilterShiftCap(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_IMM, 1),
		Stmt(BPF_LDX|BPF_IMM, 32),
		Stmt(BPF_ALU|BPF_LSH|BPF_X, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, nil, 0)
	if got != 0 {
		t.Errorf("Filter = %d, want 0 (shift >= 32)", got)
	}
}

func TestFilterLoadIndirect(t *testing.T) {
	pkt := []byte{0x00, 0x00, 0xAB, 0xCD}
	insns := []Instruction{
		Stmt(BPF_LDX|BPF_IMM, 2),
		Stmt(BPF_LD|BPF_H|BPF_IND, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, pkt, uint32(len(pkt)))
	if got != 0xABCD {
		t.Errorf("Filter = %#x, want 0xABCD", got)
	}
}

func TestFilterMSH(t *testing.T) {
	pkt := []byte{0x45} // low nibble=5, so X = 5*4 = 20
	insns := []Instruction{
		Stmt(BPF_LDX|BPF_MSH|BPF_B, 0),
		Stmt(BPF_MISC|BPF_TXA, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, pkt, uint32(len(pkt)))
	if got != 20 {
		t.Errorf("Filter = %d, want 20", got)
	}
}

func TestFilterLoadLen(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_W|BPF_LEN, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, []byte{1, 2, 3}, 100)
	if got != 100 {
		t.Errorf("Filter = %d, want 100 (wirelen)", got)
	}
}

func TestFilterNeg(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_IMM, 1),
		Stmt(BPF_ALU|BPF_NEG, 0),
		Stmt(BPF_RET|BPF_A, 0),
	}
	got := Filter(insns, nil, 0)
	if got != 0xFFFFFFFF {
		t.Errorf("Filter = %#x, want 0xFFFFFFFF", got)
	}
}
