// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// buildSimpleCFG creates a simple CFG for testing: "ip" filter equivalent.
// root: jeq #0x800 → accept / reject
func buildSimpleCFG() (*codegen.CompilerState, *codegen.ICode) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	b := codegen.GenLinktype(cs, codegen.EthertypeIP)
	codegen.FinishParse(cs, b)

	return cs, &cs.IC
}

// buildTCPCFG creates a TCP filter CFG (more complex).
func buildTCPCFG() (*codegen.CompilerState, *codegen.ICode) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	b := codegen.GenProtoAbbrev(cs, codegen.QTCP)
	codegen.FinishParse(cs, b)

	return cs, &cs.IC
}

func TestOptInit(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := &OptState{}
	err := OptInit(os, ic)
	if err != nil {
		t.Fatalf("OptInit = %v", err)
	}
	if os.NBlocks == 0 {
		t.Fatal("NBlocks = 0")
	}
	t.Logf("IP filter: %d blocks, %d edges", os.NBlocks, os.NEdges)

	// Verify blocks are populated
	for i := uint(0); i < os.NBlocks; i++ {
		if os.Blocks[i] == nil {
			t.Errorf("Blocks[%d] is nil", i)
		}
	}
}

func TestOptInitTCP(t *testing.T) {
	_, ic := buildTCPCFG()
	os := &OptState{}
	err := OptInit(os, ic)
	if err != nil {
		t.Fatalf("OptInit = %v", err)
	}
	t.Logf("TCP filter: %d blocks, %d edges", os.NBlocks, os.NEdges)
	if os.NBlocks < 5 {
		t.Errorf("TCP should have >= 5 blocks, got %d", os.NBlocks)
	}
}

func TestFindLevels(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := &OptState{}
	OptInit(os, ic)

	FindLevels(os, ic)

	// Root should have the highest level
	root := ic.Root
	if root.Level == 0 {
		t.Error("root level should be > 0")
	}
	t.Logf("root level = %d", root.Level)

	// Return blocks should have level 0
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if codegen.JT(b) == nil {
			if b.Level != 0 {
				t.Errorf("leaf block %d level = %d, want 0", b.ID, b.Level)
			}
		}
	}

	// Level list should be populated
	if os.Levels[0] == nil {
		t.Error("Levels[0] is nil, should have leaf blocks")
	}
}

func TestFindDom(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := &OptState{}
	OptInit(os, ic)
	FindLevels(os, ic)

	FindDom(os, ic.Root)

	// Root should dominate itself
	if !setMember(ic.Root.Dom, uint(ic.Root.ID)) {
		t.Error("root should dominate itself")
	}

	// Root should dominate all blocks
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if !setMember(b.Dom, uint(ic.Root.ID)) {
			t.Errorf("root should dominate block %d", b.ID)
		}
	}
}

func TestFindClosure(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := &OptState{}
	OptInit(os, ic)
	FindLevels(os, ic)

	FindClosure(os, ic.Root)

	// Every block should have itself in its closure
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if !setMember(b.Closure, uint(b.ID)) {
			t.Errorf("block %d should be in its own closure", b.ID)
		}
	}

	// Root's closure should contain all blocks (it can reach everything)
	for i := uint(0); i < os.NBlocks; i++ {
		b := os.Blocks[i]
		if !setMember(b.Closure, uint(ic.Root.ID)) {
			t.Errorf("block %d closure should contain root", b.ID)
		}
	}
}

func TestFindUD(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := &OptState{}
	OptInit(os, ic)
	FindLevels(os, ic)

	FindUD(os, ic.Root)

	// The root block should have some in_use (it loads ethertype)
	root := ic.Root
	t.Logf("root: def=%#x kill=%#x in_use=%#x out_use=%#x",
		root.Def, root.Kill, root.InUse, root.OutUse)
}

func TestFindEdom(t *testing.T) {
	_, ic := buildSimpleCFG()
	os := &OptState{}
	OptInit(os, ic)
	FindLevels(os, ic)

	FindEdom(os, ic.Root)

	// Root edges should dominate themselves
	rootEt := &ic.Root.Et
	if !setMember(rootEt.Edom, uint(rootEt.ID)) {
		t.Error("root et should dominate itself")
	}
}

func TestComputeLocalUD(t *testing.T) {
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	// Create a block that loads ethertype: ldh [12]; jeq #0x800
	b := codegen.GenLinktype(cs, codegen.EthertypeIP)
	if b == nil {
		t.Fatal("GenLinktype returned nil")
	}

	ComputeLocalUD(b)

	// The block loads into A (LD), so A should be in def
	if !AtomElem(b.Def, AAtom) {
		t.Error("A should be in def (LD loads into A)")
	}
	// A is defined by LD before JEQ uses it, so A is in kill (not in_use).
	// in_use tracks values read from PREDECESSORS, not within the block.
	if !AtomElem(b.Kill, AAtom) {
		t.Error("A should be in kill (LD defines A before JEQ uses it)")
	}
}
