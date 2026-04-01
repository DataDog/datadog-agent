// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

func TestEqSlist(t *testing.T) {
	s1 := codegen.NewStmt(0x28, 12) // ldh [12]
	s2 := codegen.NewStmt(0x28, 12)

	if !eqSlist(s1, s2) {
		t.Error("identical slists should be equal")
	}

	s3 := codegen.NewStmt(0x28, 14) // different offset
	if eqSlist(s1, s3) {
		t.Error("different slists should not be equal")
	}

	// NOP should be skipped
	nop := codegen.NewStmt(codegen.NOP, 0)
	nop.Next = s2
	if !eqSlist(s1, nop) {
		t.Error("NOP-prefixed slist should equal non-NOP slist")
	}
}

func TestEqBlk(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	accept := cs.NewBlock(0x06, 65535)
	reject := cs.NewBlock(0x06, 0)

	b1 := cs.NewBlock(0x15, 0x0800)
	codegen.SetJT(b1, accept)
	codegen.SetJF(b1, reject)
	b1.Stmts = codegen.NewStmt(0x28, 12)

	b2 := cs.NewBlock(0x15, 0x0800)
	codegen.SetJT(b2, accept)
	codegen.SetJF(b2, reject)
	b2.Stmts = codegen.NewStmt(0x28, 12)

	if !eqBlk(b1, b2) {
		t.Error("identical blocks should be equal")
	}

	b3 := cs.NewBlock(0x15, 0x86dd) // different K
	codegen.SetJT(b3, accept)
	codegen.SetJF(b3, reject)
	b3.Stmts = codegen.NewStmt(0x28, 12)

	if eqBlk(b1, b3) {
		t.Error("blocks with different K should not be equal")
	}
}

func TestInternBlocks(t *testing.T) {
	_, ic := buildTCPCFG()
	os := initOptState(ic)

	os.InternBlocks(ic)
	t.Log("InternBlocks completed without panic")
}

func TestOptRoot(t *testing.T) {
	_, ic := buildSimpleCFG()
	root := ic.Root

	// OptRoot should not crash
	OptRoot(&root)
	if root == nil {
		t.Fatal("root is nil after OptRoot")
	}
	ic.Root = root
}

func TestOptRootSkipIdenticalBranches(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	accept := cs.NewBlock(0x06, 65535)

	// Block where JT == JF (both go to accept)
	b := cs.NewBlock(0x15, 0x0800)
	codegen.SetJT(b, accept)
	codegen.SetJF(b, accept)
	b.Stmts = codegen.NewStmt(0x28, 12)

	root := b
	OptRoot(&root)

	// Root should have been simplified to the accept block
	if root != accept {
		t.Error("OptRoot should skip blocks where JT==JF")
	}
}

func TestMakeMarks(t *testing.T) {
	_, ic := buildSimpleCFG()
	markCode(ic)

	// Root should be marked
	if !ic.IsMarked(ic.Root) {
		t.Error("root should be marked")
	}
}

func TestPullupDoesNotCrashOnSimpleFilter(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := initOptState(ic)

	FindLevels(os, ic)
	FindDom(os, ic.Root)
	FindUD(os, ic.Root)
	FindInedges(os)

	for i := 1; i <= ic.Root.Level; i++ {
		for p := os.Levels[i]; p != nil; p = p.Link {
			os.orPullup(p, ic.Root)
			os.andPullup(p, ic.Root)
		}
	}
	t.Log("Pullup on simple filter completed without panic")
}

func TestPullupDoesNotCrashOnTCP(t *testing.T) {
	_, ic := buildTCPCFG()
	os := initOptState(ic)

	FindLevels(os, ic)
	FindDom(os, ic.Root)
	FindUD(os, ic.Root)
	FindInedges(os)

	for i := 1; i <= ic.Root.Level; i++ {
		for p := os.Levels[i]; p != nil; p = p.Link {
			os.orPullup(p, ic.Root)
			os.andPullup(p, ic.Root)
		}
	}
	t.Log("Pullup on TCP filter completed without panic")
}
