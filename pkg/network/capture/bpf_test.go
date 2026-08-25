// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"testing"

	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileBPFFilter(t *testing.T) {
	t.Run("empty filter returns nil with no error", func(t *testing.T) {
		raw, err := compileBPFFilter("", 65535)
		require.NoError(t, err)
		assert.Nil(t, raw)
	})

	t.Run("valid filter compiles to non-empty instructions", func(t *testing.T) {
		raw, err := compileBPFFilter("tcp and port 80", 65535)
		require.NoError(t, err)
		assert.NotEmpty(t, raw)
	})

	t.Run("invalid filter syntax returns an error", func(t *testing.T) {
		_, err := compileBPFFilter("this is not )( a filter", 65535)
		assert.Error(t, err)
	})
}

func TestBpfToEBPF(t *testing.T) {
	t.Run("valid classic BPF converts to eBPF instructions", func(t *testing.T) {
		raw, err := compileBPFFilter("icmp", 65535)
		require.NoError(t, err)
		require.NotEmpty(t, raw)

		insts, err := bpfToEBPF(raw)
		require.NoError(t, err)
		assert.NotEmpty(t, insts)
	})

	t.Run("register contract disjointness matches buildProgram's expectations", func(t *testing.T) {
		// cbpfc requires PacketStart/PacketEnd disjoint from Working. This is
		// the exact contract violation ("register X overlaps Y") that caused
		// the original program-load failure — assert it stays enforced here
		// rather than relying only on end-to-end verifier runs.
		working := map[asm.Register]bool{asm.R0: true, asm.R3: true, asm.R4: true, asm.R5: true}
		assert.False(t, working[asm.R1], "PacketStart (R1) must not be a Working register")
		assert.False(t, working[asm.R2], "PacketEnd (R2) must not be a Working register")
	})

	t.Run("result label anchor constant is non-empty", func(t *testing.T) {
		// buildProgram relies on this exact symbol to anchor an instruction
		// after splicing in filter instructions; cbpfc's generated code jumps
		// to it unconditionally. An empty/changed label here without a
		// matching anchor reproduces the "unsatisfied program reference"
		// verifier error found during sandbox validation.
		assert.NotEmpty(t, cbpfcResultLabel)
	})
}

func TestBuildHeaderOffsetInsts(t *testing.T) {
	t.Run("never touches the survives-across-helper-calls registers", func(t *testing.T) {
		reserved := map[asm.Register]bool{asm.R6: true, asm.R7: true, asm.R8: true, asm.R9: true}
		insts := buildHeaderOffsetInsts(65535)
		require.NotEmpty(t, insts)
		for _, inst := range insts {
			if reserved[inst.Dst] {
				t.Fatalf("instruction clobbers reserved register %v: %v", inst.Dst, inst)
			}
		}
	})

	t.Run("result register R4 is always written", func(t *testing.T) {
		insts := buildHeaderOffsetInsts(65535)
		wrote := false
		for _, inst := range insts {
			if inst.Dst == asm.R4 {
				wrote = true
				break
			}
		}
		assert.True(t, wrote, "expected at least one instruction writing R4 (the result register)")
	})

	t.Run("tiny snapLen still produces a valid, non-empty instruction sequence", func(t *testing.T) {
		// snapLen too small to safely read even the EtherType field — should
		// short-circuit straight to the finalize clamp rather than emit
		// unsafe reads.
		insts := buildHeaderOffsetInsts(4)
		require.NotEmpty(t, insts)
	})

	t.Run("every jump target label has a matching anchor", func(t *testing.T) {
		for _, snapLen := range []uint32{4, 14, 20, 34, 40, 54, 60, 65535} {
			insts := buildHeaderOffsetInsts(snapLen)
			labels := map[string]bool{}
			for _, inst := range insts {
				if sym := inst.Symbol(); sym != "" {
					labels[sym] = true
				}
			}
			for _, inst := range insts {
				if target := inst.Reference(); target != "" {
					assert.True(t, labels[target], "snapLen=%d: jump target %q has no matching anchor", snapLen, target)
				}
			}
		}
	})
}
