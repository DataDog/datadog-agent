// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import "testing"

func TestNewCompilerState(t *testing.T) {
	cs := NewCompilerState(1, 262144, 0, nil)
	if cs.Linktype != 1 {
		t.Errorf("Linktype = %d, want 1", cs.Linktype)
	}
	if cs.Snaplen != 262144 {
		t.Errorf("Snaplen = %d, want 262144", cs.Snaplen)
	}
}

func TestNewBlock(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)
	b := cs.NewBlock(0x15, 42) // BPF_JMP|BPF_JEQ|BPF_K
	if b.ID != 0 {
		t.Errorf("first block ID = %d, want 0", b.ID)
	}
	if b.S.Code != 0x15 || b.S.K != 42 {
		t.Errorf("block stmt = {%#x, %d}, want {0x15, 42}", b.S.Code, b.S.K)
	}

	b2 := cs.NewBlock(0x06, 0) // BPF_RET|BPF_K
	if b2.ID != 1 {
		t.Errorf("second block ID = %d, want 1", b2.ID)
	}
}

func TestAllocFreeReg(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)
	r0 := cs.AllocReg()
	if r0 != 0 {
		t.Errorf("first reg = %d, want 0", r0)
	}
	r1 := cs.AllocReg()
	if r1 != 1 {
		t.Errorf("second reg = %d, want 1", r1)
	}

	cs.FreeReg(r0)
	r2 := cs.AllocReg()
	if r2 != 0 {
		t.Errorf("after free, next reg = %d, want 0", r2)
	}
}

func TestSappend(t *testing.T) {
	s0 := NewStmt(0x00, 10) // BPF_LD|BPF_IMM
	s1 := NewStmt(0x04, 5)  // BPF_ALU|BPF_ADD|BPF_K

	Sappend(s0, s1)

	if s0.Next != s1 {
		t.Error("Sappend didn't link s1 after s0")
	}
}

func TestQualConstants(t *testing.T) {
	if QHost != 1 || QNet != 2 || QPort != 3 {
		t.Error("address qualifiers wrong")
	}
	if QSrc != 1 || QDst != 2 || QOr != 3 || QAnd != 4 {
		t.Error("direction qualifiers wrong")
	}
	if QIP != 2 || QTCP != 6 || QUDP != 7 {
		t.Error("protocol qualifiers wrong")
	}
}

func TestJTJF(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)
	b := cs.NewBlock(0x15, 0)
	bt := cs.NewBlock(0x06, 1)
	bf := cs.NewBlock(0x06, 0)

	SetJT(b, bt)
	SetJF(b, bf)

	if JT(b) != bt {
		t.Error("JT didn't return true successor")
	}
	if JF(b) != bf {
		t.Error("JF didn't return false successor")
	}
}
