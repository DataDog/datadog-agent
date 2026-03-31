// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

func initOptState(ic *codegen.ICode) *OptState {
	os := &OptState{}
	OptInit(os, ic)
	FindLevels(os, ic)
	return os
}

func TestOptBlksIP(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := initOptState(ic)

	// Run block optimization (structural pass)
	os.OptBlks(ic, false)
	if os.Err != nil {
		t.Fatalf("OptBlks error: %v", os.Err)
	}
}

func TestOptBlksTCP(t *testing.T) {
	_, ic := buildTCPCFG()
	os := initOptState(ic)

	os.OptBlks(ic, false)
	if os.Err != nil {
		t.Fatalf("OptBlks error: %v", os.Err)
	}
}

func TestOptBlksStmtsIP(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := initOptState(ic)

	// Run statement-level optimization
	FindUD(os, ic.Root)
	os.OptBlks(ic, true)
	if os.Err != nil {
		t.Fatalf("OptBlks(stmts) error: %v", os.Err)
	}
}

func TestOptBlksStmtsTCP(t *testing.T) {
	_, ic := buildTCPCFG()
	os := initOptState(ic)

	FindUD(os, ic.Root)
	os.OptBlks(ic, true)
	if os.Err != nil {
		t.Fatalf("OptBlks(stmts) error: %v", os.Err)
	}
}

func TestUseConflict(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	b1 := cs.NewBlock(codegen.JmpCode(0x10), 0) // JEQ
	b2 := cs.NewBlock(codegen.JmpCode(0x10), 0)

	// Same values → no conflict
	b1.Val[AAtom] = 5
	b2.Val[AAtom] = 5
	b2.OutUse = AtomMask(AAtom)
	if useConflict(b1, b2) {
		t.Error("expected no conflict when values match")
	}

	// Different values → conflict
	b1.Val[AAtom] = 5
	b2.Val[AAtom] = 7
	if !useConflict(b1, b2) {
		t.Error("expected conflict when values differ")
	}

	// No use → no conflict regardless of values
	b2.OutUse = 0
	if useConflict(b1, b2) {
		t.Error("expected no conflict when out_use is 0")
	}
}

func TestFoldEdge(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	child := cs.NewBlock(codegen.JmpCode(int(0x10)), 42) // JEQ #42
	jt := cs.NewBlock(int(0x06), 65535)                   // ret accept
	jf := cs.NewBlock(int(0x06), 0)                       // ret reject
	codegen.SetJT(child, jt)
	codegen.SetJF(child, jf)

	parent := cs.NewBlock(codegen.JmpCode(int(0x10)), 42)
	parent.Val[AAtom] = 5
	child.Val[AAtom] = 5
	child.Oval = 10

	ep := &codegen.Edge{Code: parent.S.Code, Pred: parent}
	parent.Oval = 10 // same oval

	// Same code, same A, same oval, sense=true → return JT(child)
	target := foldEdge(child, ep)
	if target != jt {
		t.Error("expected JT(child) for matching edge")
	}

	// Negative code (false branch) → return JF(child)
	ep.Code = -parent.S.Code
	target = foldEdge(child, ep)
	if target != jf {
		t.Error("expected JF(child) for negative edge")
	}
}
