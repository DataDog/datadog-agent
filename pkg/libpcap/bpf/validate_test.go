// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

import "testing"

func TestValidateEmpty(t *testing.T) {
	if Validate(nil) {
		t.Error("empty program should be invalid")
	}
}

func TestValidateSimpleReturn(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_RET|BPF_K, 0),
	}
	if !Validate(insns) {
		t.Error("simple return should be valid")
	}
}

func TestValidateNoReturn(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_IMM, 0),
	}
	if Validate(insns) {
		t.Error("program not ending with RET should be invalid")
	}
}

func TestValidateJumpOutOfBounds(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_JMP|BPF_JA, 5),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if Validate(insns) {
		t.Error("jump past end should be invalid")
	}
}

func TestValidateConditionalJumpOutOfBounds(t *testing.T) {
	insns := []Instruction{
		Jump(BPF_JMP|BPF_JEQ|BPF_K, 0, 10, 0),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if Validate(insns) {
		t.Error("conditional jump past end should be invalid")
	}
}

func TestValidateMemOutOfBounds(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_ST, BPF_MEMWORDS),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if Validate(insns) {
		t.Error("store to M[16] should be invalid")
	}
}

func TestValidateMemValid(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_ST, BPF_MEMWORDS-1),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if !Validate(insns) {
		t.Error("store to M[15] should be valid")
	}
}

func TestValidateDivByZeroK(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_ALU|BPF_DIV|BPF_K, 0),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if Validate(insns) {
		t.Error("div by constant 0 should be invalid")
	}
}

func TestValidateModByZeroK(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_ALU|BPF_MOD|BPF_K, 0),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if Validate(insns) {
		t.Error("mod by constant 0 should be invalid")
	}
}

func TestValidateDivByX(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_ALU|BPF_DIV|BPF_X, 0),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if !Validate(insns) {
		t.Error("div by X should be valid (runtime check)")
	}
}

func TestValidateLoadMemOutOfBounds(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_MEM, BPF_MEMWORDS),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if Validate(insns) {
		t.Error("load from M[16] should be invalid")
	}
}

func TestValidateComplexProgram(t *testing.T) {
	insns := []Instruction{
		Stmt(BPF_LD|BPF_H|BPF_ABS, 12),
		Jump(BPF_JMP|BPF_JEQ|BPF_K, 0x0800, 0, 1),
		Stmt(BPF_RET|BPF_K, 65535),
		Stmt(BPF_RET|BPF_K, 0),
	}
	if !Validate(insns) {
		t.Error("valid ethertype filter should pass validation")
	}
}
