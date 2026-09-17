// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/bpf"
)

func buildAllowlistFilter(allowed []int) []bpf.Instruction {
	insts := []bpf.Instruction{
		bpf.LoadAbsolute{Off: offsetArch, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: auditArchX86_64, SkipTrue: 1},
		bpf.RetConstant{Val: 0}, // KILL
		bpf.LoadAbsolute{Off: offsetNr, Size: 4},
	}
	for _, nr := range allowed {
		insts = append(insts,
			bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(nr), SkipTrue: 0, SkipFalse: 1},
			bpf.RetConstant{Val: seccompRetAllow},
		)
	}
	insts = append(insts, bpf.RetConstant{Val: seccompRetErrno})
	return insts
}

func TestEmulateFilters_Actions(t *testing.T) {
	filter := buildAllowlistFilter([]int{0, 1, 3}) // read, write, close

	actions := emulateFilters([][]bpf.Instruction{filter}, auditArchX86_64)

	assert.Equal(t, ActionAllow, actions[0]) // read
	assert.Equal(t, ActionAllow, actions[1]) // write
	assert.Equal(t, ActionErrno, actions[2]) // open → denied
	assert.Equal(t, ActionAllow, actions[3]) // close
}

func TestEmulateFilters_Empty(t *testing.T) {
	result := emulateFilters(nil, auditArchX86_64)
	assert.Nil(t, result)
}

func TestEmulateFilters_StackedFilters(t *testing.T) {
	f1 := buildAllowlistFilter([]int{0, 1, 2, 3})
	f2 := buildAllowlistFilter([]int{1, 3, 5})

	actions := emulateFilters([][]bpf.Instruction{f1, f2}, auditArchX86_64)

	assert.Equal(t, ActionErrno, actions[0]) // read: denied by f2
	assert.Equal(t, ActionAllow, actions[1]) // write: allowed by both
	assert.Equal(t, ActionErrno, actions[2]) // open: denied by f2
	assert.Equal(t, ActionAllow, actions[3]) // close: allowed by both
	assert.Equal(t, ActionErrno, actions[5]) // fstat: denied by f1
}

func TestEmulateFilters_WrongArch(t *testing.T) {
	filter := buildAllowlistFilter([]int{0, 1, 2})

	actions := emulateFilters([][]bpf.Instruction{filter}, auditArchAArch64)

	for nr := 0; nr < 10; nr++ {
		assert.Equal(t, ActionKillThread, actions[nr], "NR %d should be killed on wrong arch", nr)
	}
}

func TestVerdictToAction(t *testing.T) {
	assert.Equal(t, ActionAllow, verdictToAction(seccompRetAllow))
	assert.Equal(t, ActionErrno, verdictToAction(seccompRetErrno))
	assert.Equal(t, ActionKillProcess, verdictToAction(seccompRetKillProcess))
	assert.Equal(t, ActionKillThread, verdictToAction(seccompRetKillThread))
	assert.Equal(t, ActionTrap, verdictToAction(seccompRetTrap))
	assert.Equal(t, ActionLog, verdictToAction(seccompRetLog))
	assert.Equal(t, ActionTrace, verdictToAction(seccompRetTrace))
}

func TestBuildSeccompData(t *testing.T) {
	data := buildSeccompData(42, auditArchX86_64)
	require.Len(t, data, seccompDataSize)
}

func TestArchToAudit(t *testing.T) {
	assert.Equal(t, uint32(auditArchX86_64), archToAudit("amd64"))
	assert.Equal(t, uint32(auditArchAArch64), archToAudit("arm64"))
	assert.Equal(t, uint32(auditArchX86_64), archToAudit(""))
}

// --- Arg condition analysis tests ---

// buildArgEqualityFilter creates a filter that checks:
//
//	if nr == syscallNR:
//	    if args[argIdx] == allowedVal: ALLOW
//	    else: ERRNO
//	else: ALLOW (default)
func buildArgEqualityFilter(syscallNR int, argIdx int, allowedVal uint32) []bpf.Instruction {
	argOff := uint32(offsetArgs + argIdx*8) // low 32 bits of args[N]
	return []bpf.Instruction{
		bpf.LoadAbsolute{Off: offsetArch, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: auditArchX86_64, SkipTrue: 1},
		bpf.RetConstant{Val: seccompRetKillThread},
		bpf.LoadAbsolute{Off: offsetNr, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(syscallNR), SkipTrue: 0, SkipFalse: 3},
		// NR matched — check arg
		bpf.LoadAbsolute{Off: argOff, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: allowedVal, SkipTrue: 0, SkipFalse: 1},
		bpf.RetConstant{Val: seccompRetAllow},
		bpf.RetConstant{Val: seccompRetErrno},
		// Default: allow other syscalls
		bpf.RetConstant{Val: seccompRetAllow},
	}
}

func TestAnalyzeArgConditions_EqualityCheck(t *testing.T) {
	// socket (NR 41): allow only if args[0] == 2 (AF_INET)
	filter := buildArgEqualityFilter(41, 0, 2)
	result := analyzeArgConditions(filter)

	require.Contains(t, result, 41)
	conds := result[41]
	require.Len(t, conds, 1)
	assert.Equal(t, 0, conds[0].Index)
	assert.Equal(t, "==", conds[0].Op)
	assert.Equal(t, uint32(2), conds[0].Value)
	assert.Equal(t, ActionAllow, conds[0].Action)
}

// buildArgBitmaskFilter creates a filter that checks:
//
//	if nr == syscallNR:
//	    if args[argIdx] & mask != 0: ERRNO (deny if bits set)
//	    else: ALLOW
//	else: ALLOW
func buildArgBitmaskFilter(syscallNR int, argIdx int, mask uint32) []bpf.Instruction {
	argOff := uint32(offsetArgs + argIdx*8)
	return []bpf.Instruction{
		bpf.LoadAbsolute{Off: offsetArch, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: auditArchX86_64, SkipTrue: 1},
		bpf.RetConstant{Val: seccompRetKillThread},
		bpf.LoadAbsolute{Off: offsetNr, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(syscallNR), SkipTrue: 0, SkipFalse: 4},
		// NR matched — check bitmask on arg
		bpf.LoadAbsolute{Off: argOff, Size: 4},
		bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: mask},
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 0, SkipTrue: 0, SkipFalse: 1},
		bpf.RetConstant{Val: seccompRetErrno},
		bpf.RetConstant{Val: seccompRetAllow},
		// Default
		bpf.RetConstant{Val: seccompRetAllow},
	}
}

func TestAnalyzeArgConditions_BitmaskCheck(t *testing.T) {
	// clone (NR 56): deny if args[0] & 0x7E020000 != 0 (CLONE_NEW* flags)
	filter := buildArgBitmaskFilter(56, 0, 0x7E020000)
	result := analyzeArgConditions(filter)

	require.Contains(t, result, 56)
	conds := result[56]
	require.Len(t, conds, 1)
	assert.Equal(t, 0, conds[0].Index)
	assert.Equal(t, "&", conds[0].Op)
	assert.Equal(t, uint32(0x7E020000), conds[0].Value)
	assert.Equal(t, ActionErrno, conds[0].Action)
}

// buildMultiArgValFilter creates a filter that allows a syscall only for
// multiple specific values of one argument.
func buildMultiArgValFilter(syscallNR int, argIdx int, allowedVals []uint32) []bpf.Instruction {
	argOff := uint32(offsetArgs + argIdx*8)
	insts := []bpf.Instruction{
		bpf.LoadAbsolute{Off: offsetArch, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: auditArchX86_64, SkipTrue: 1},
		bpf.RetConstant{Val: seccompRetKillThread},
		bpf.LoadAbsolute{Off: offsetNr, Size: 4},
	}
	// Skip to default allow if NR doesn't match: skip over arg checks + final deny
	skipLen := uint8(len(allowedVals)*2 + 2) // load + N*(jump+ret) + deny ret
	insts = append(insts, bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(syscallNR), SkipTrue: 0, SkipFalse: skipLen})
	insts = append(insts, bpf.LoadAbsolute{Off: argOff, Size: 4})
	for _, val := range allowedVals {
		insts = append(insts,
			bpf.JumpIf{Cond: bpf.JumpEqual, Val: val, SkipTrue: 0, SkipFalse: 1},
			bpf.RetConstant{Val: seccompRetAllow},
		)
	}
	insts = append(insts, bpf.RetConstant{Val: seccompRetErrno}) // deny
	insts = append(insts, bpf.RetConstant{Val: seccompRetAllow}) // default allow
	return insts
}

func TestAnalyzeArgConditions_MultipleValues(t *testing.T) {
	// socket (NR 41): allow only AF_INET(2), AF_INET6(10), AF_UNIX(1)
	filter := buildMultiArgValFilter(41, 0, []uint32{2, 10, 1})
	result := analyzeArgConditions(filter)

	require.Contains(t, result, 41)
	conds := result[41]
	require.Len(t, conds, 3)

	values := make(map[uint32]bool)
	for _, c := range conds {
		assert.Equal(t, 0, c.Index)
		assert.Equal(t, "==", c.Op)
		assert.Equal(t, ActionAllow, c.Action)
		values[c.Value] = true
	}
	assert.True(t, values[2], "AF_INET")
	assert.True(t, values[10], "AF_INET6")
	assert.True(t, values[1], "AF_UNIX")
}

func TestAnalyzeArgConditions_NoArgChecks(t *testing.T) {
	// Simple allowlist filter has no arg conditions.
	filter := buildAllowlistFilter([]int{0, 1, 2})
	result := analyzeArgConditions(filter)
	assert.Empty(t, result)
}

func TestAnalyzeArgConditions_BitsSetDirect(t *testing.T) {
	// Filter using JumpBitsSet directly (no ALUOpAnd):
	// if nr == 56: if args[0] & 0x10000 set: ERRNO; else: ALLOW
	filter := []bpf.Instruction{
		bpf.LoadAbsolute{Off: offsetArch, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: auditArchX86_64, SkipTrue: 1},
		bpf.RetConstant{Val: seccompRetKillThread},
		bpf.LoadAbsolute{Off: offsetNr, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 56, SkipTrue: 0, SkipFalse: 3},
		bpf.LoadAbsolute{Off: offsetArgs, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x10000, SkipTrue: 0, SkipFalse: 1},
		bpf.RetConstant{Val: seccompRetErrno},
		bpf.RetConstant{Val: seccompRetAllow},
		bpf.RetConstant{Val: seccompRetAllow},
	}
	result := analyzeArgConditions(filter)

	require.Contains(t, result, 56)
	conds := result[56]
	require.Len(t, conds, 1)
	assert.Equal(t, "&", conds[0].Op)
	assert.Equal(t, uint32(0x10000), conds[0].Value)
	assert.Equal(t, ActionErrno, conds[0].Action)
}
