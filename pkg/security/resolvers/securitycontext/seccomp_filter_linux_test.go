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
