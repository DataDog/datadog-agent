// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

import "testing"

func TestInstructionSize(t *testing.T) {
	var insn Instruction
	_ = insn.Code
	_ = insn.Jt
	_ = insn.Jf
	_ = insn.K
}

func TestConstants(t *testing.T) {
	if BPF_LD != 0x00 {
		t.Errorf("BPF_LD = %#x, want 0x00", BPF_LD)
	}
	if BPF_LDX != 0x01 {
		t.Errorf("BPF_LDX = %#x, want 0x01", BPF_LDX)
	}
	if BPF_ST != 0x02 {
		t.Errorf("BPF_ST = %#x, want 0x02", BPF_ST)
	}
	if BPF_STX != 0x03 {
		t.Errorf("BPF_STX = %#x, want 0x03", BPF_STX)
	}
	if BPF_ALU != 0x04 {
		t.Errorf("BPF_ALU = %#x, want 0x04", BPF_ALU)
	}
	if BPF_JMP != 0x05 {
		t.Errorf("BPF_JMP = %#x, want 0x05", BPF_JMP)
	}
	if BPF_RET != 0x06 {
		t.Errorf("BPF_RET = %#x, want 0x06", BPF_RET)
	}
	if BPF_MISC != 0x07 {
		t.Errorf("BPF_MISC = %#x, want 0x07", BPF_MISC)
	}

	if Class(BPF_LD|BPF_W|BPF_ABS) != BPF_LD {
		t.Error("Class extraction failed")
	}
	if Size(BPF_LD|BPF_H|BPF_ABS) != BPF_H {
		t.Error("Size extraction failed")
	}
	if Mode(BPF_LD|BPF_W|BPF_ABS) != BPF_ABS {
		t.Error("Mode extraction failed")
	}
	if Op(BPF_ALU|BPF_ADD|BPF_K) != BPF_ADD {
		t.Error("Op extraction failed")
	}
	if Src(BPF_JMP|BPF_JEQ|BPF_X) != BPF_X {
		t.Error("Src extraction failed")
	}
}

func TestStmtJump(t *testing.T) {
	stmt := Stmt(BPF_RET|BPF_K, 0)
	if stmt.Code != BPF_RET|BPF_K || stmt.Jt != 0 || stmt.Jf != 0 || stmt.K != 0 {
		t.Errorf("Stmt = %+v", stmt)
	}

	jump := Jump(BPF_JMP|BPF_JEQ|BPF_K, 42, 1, 2)
	if jump.Code != BPF_JMP|BPF_JEQ|BPF_K || jump.Jt != 1 || jump.Jf != 2 || jump.K != 42 {
		t.Errorf("Jump = %+v", jump)
	}
}
