// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

func TestGenRetBlk(t *testing.T) {
	cs := NewCompilerState(1, 65535, 0, nil)
	b := GenRetBlk(cs, 65535)
	if b.S.Code != int(bpf.BPF_RET|bpf.BPF_K) {
		t.Errorf("code = %#x, want BPF_RET|BPF_K", b.S.Code)
	}
	if b.S.K != 65535 {
		t.Errorf("k = %d, want 65535", b.S.K)
	}
}

func TestJmpCode(t *testing.T) {
	got := JmpCode(int(bpf.BPF_JEQ))
	want := int(bpf.BPF_JMP | bpf.BPF_JEQ | bpf.BPF_K)
	if got != want {
		t.Errorf("JmpCode(BPF_JEQ) = %#x, want %#x", got, want)
	}
}

func TestBackpatch(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	// Create a block with unresolved JT
	b := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 0x0800)
	b.Sense = false // unresolved link is in JT

	target := GenRetBlk(cs, 65535)

	Backpatch(b, target)

	if JT(b) != target {
		t.Error("Backpatch didn't set JT to target")
	}
}

func TestBackpatchSense(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	b := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 0x0800)
	b.Sense = true // unresolved link is in JF

	target := GenRetBlk(cs, 0)

	Backpatch(b, target)

	if JF(b) != target {
		t.Error("Backpatch with sense=true didn't set JF to target")
	}
}

func TestGenNot(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)
	b := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 0)
	b.Sense = false

	GenNot(b)
	if !b.Sense {
		t.Error("GenNot didn't flip sense")
	}

	GenNot(b)
	if b.Sense {
		t.Error("GenNot didn't flip sense back")
	}
}

func TestGenAndOr(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	// Create two simple condition blocks
	b0 := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 0x0800)
	b1 := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 6)

	// AND: b0 must match, then b1 must match
	GenAnd(b0, b1)

	// After AND, b1.Head should be b0.Head
	if b1.Head != b0.Head {
		t.Error("GenAnd didn't set b1.Head to b0.Head")
	}

	// The JT of b0 should point to b1 (if b0 matches, check b1)
	if JT(b0) != b1 {
		t.Error("GenAnd didn't backpatch b0's true branch to b1")
	}
}

func TestGenOrBlocks(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	b0 := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 0x0800)
	b1 := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 0x86dd)

	GenOr(b0, b1)

	// After OR, b1.Head should be b0.Head
	if b1.Head != b0.Head {
		t.Error("GenOr didn't set b1.Head to b0.Head")
	}
}

func TestFinishParseEmpty(t *testing.T) {
	cs := NewCompilerState(1, 65535, 0, nil)
	err := FinishParse(cs, nil)
	if err != nil {
		t.Errorf("FinishParse(nil) = %v, want nil", err)
	}
	if cs.IC.Root == nil {
		t.Error("FinishParse(nil) should set IC.Root to accept-all block")
	}
	if cs.IC.Root.S.Code != int(bpf.BPF_RET|bpf.BPF_K) {
		t.Error("FinishParse(nil) root should be a return block")
	}
	if cs.IC.Root.S.K != 65535 {
		t.Errorf("FinishParse(nil) root.K = %d, want 65535 (snaplen)", cs.IC.Root.S.K)
	}
}

func TestFinishParseWithBlock(t *testing.T) {
	cs := NewCompilerState(1, 65535, 0, nil)
	b := cs.NewBlock(JmpCode(int(bpf.BPF_JEQ)), 0x0800)
	err := FinishParse(cs, b)
	if err != nil {
		t.Errorf("FinishParse = %v, want nil", err)
	}
	if cs.IC.Root == nil {
		t.Fatal("IC.Root is nil")
	}
	// The root should be b's head
	if cs.IC.Root != b.Head {
		t.Error("IC.Root should be b.Head")
	}
	// JT should lead to accept (return snaplen)
	if JT(b) == nil || JT(b).S.K != 65535 {
		t.Error("true branch should return snaplen")
	}
	// JF should lead to reject (return 0)
	if JF(b) == nil || JF(b).S.K != 0 {
		t.Error("false branch should return 0")
	}
}
