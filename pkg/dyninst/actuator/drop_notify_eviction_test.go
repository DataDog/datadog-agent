// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
)

// evictLoggingProgram is a LoadedProgram that records every
// EvictBufferOlderThan call and exposes a settable DropNotifyLostAt
// value.
type evictLoggingProgram struct {
	fakeLoadedProgram
	lostAt    uint64
	evictions []uint64
}

func (p *evictLoggingProgram) DropNotifyLostAt() uint64 { return p.lostAt }
func (p *evictLoggingProgram) EvictBufferOlderThan(cutoffKtimeNs uint64) {
	p.evictions = append(p.evictions, cutoffKtimeNs)
}

func setupEvictionTest(t *testing.T, grace time.Duration) (*state, *program, *evictLoggingProgram) {
	t.Helper()
	sm := newState(Config{
		BufferEvictionConfig: BufferEvictionConfig{GraceWindow: grace},
	})
	lp := &evictLoggingProgram{}
	prog := &program{
		state: programStateLoaded,
		id:    ir.ProgramID(1),
		loaded: &loadedProgram{
			programID: 1,
			loaded:    lp,
		},
	}
	sm.programs[prog.id] = prog
	return sm, prog, lp
}

func TestEvictionRule_NoLossEver(t *testing.T) {
	sm, prog, lp := setupEvictionTest(t, 10*time.Second)
	// DropNotifyLostAt stays at 0. Call many times; no eviction ever.
	origNow := nowKtimeNs
	t.Cleanup(func() { nowKtimeNs = origNow })
	nowKtimeNs = func() uint64 { return 100_000_000_000 }
	for i := 0; i < 10; i++ {
		evaluateDropNotifyEviction(sm, prog)
	}
	assert.Empty(t, lp.evictions)
	assert.Zero(t, prog.lastAppliedLost)
}

func TestEvictionRule_FreshLossHeldByGraceWindow(t *testing.T) {
	sm, prog, lp := setupEvictionTest(t, 10*time.Second)
	origNow := nowKtimeNs
	t.Cleanup(func() { nowKtimeNs = origNow })

	// BPF reported a loss 1 second ago. Grace window is 10s — don't
	// evict yet.
	lp.lostAt = 99_000_000_000
	nowKtimeNs = func() uint64 { return 100_000_000_000 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Empty(t, lp.evictions)
	assert.Zero(t, prog.lastAppliedLost)

	// 10 seconds later: the loss is now exactly graceWindow old.
	// The rule requires lostAt < now - graceWindow (strict), so this
	// poll should fire.
	nowKtimeNs = func() uint64 { return 99_000_000_000 + 10_000_000_001 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Equal(t, []uint64{99_000_000_000}, lp.evictions)
	assert.Equal(t, uint64(99_000_000_000), prog.lastAppliedLost)
}

func TestEvictionRule_RepeatedLoss(t *testing.T) {
	sm, prog, lp := setupEvictionTest(t, 1*time.Second)
	origNow := nowKtimeNs
	t.Cleanup(func() { nowKtimeNs = origNow })

	// First loss at T=100s, poll at T=102s (1s past grace).
	lp.lostAt = 100_000_000_000
	nowKtimeNs = func() uint64 { return 102_000_000_000 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Equal(t, []uint64{100_000_000_000}, lp.evictions)

	// Another loss at T=103s, poll at T=105s.
	lp.lostAt = 103_000_000_000
	nowKtimeNs = func() uint64 { return 105_000_000_000 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Equal(t, []uint64{100_000_000_000, 103_000_000_000}, lp.evictions)
	assert.Equal(t, uint64(103_000_000_000), prog.lastAppliedLost)
}

func TestEvictionRule_StaleLostValueNoRepeat(t *testing.T) {
	sm, prog, lp := setupEvictionTest(t, 1*time.Second)
	origNow := nowKtimeNs
	t.Cleanup(func() { nowKtimeNs = origNow })

	// Loss at T=100, we see it and evict.
	lp.lostAt = 100_000_000_000
	nowKtimeNs = func() uint64 { return 102_000_000_000 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Len(t, lp.evictions, 1)

	// Next poll: same lostAt, should NOT re-fire.
	nowKtimeNs = func() uint64 { return 103_000_000_000 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Len(t, lp.evictions, 1)
}

func TestEvictionRule_GraceWindowDisabled(t *testing.T) {
	sm, prog, lp := setupEvictionTest(t, 0)
	origNow := nowKtimeNs
	t.Cleanup(func() { nowKtimeNs = origNow })

	// Even with a clear fault, zero grace window disables the rule.
	lp.lostAt = 100_000_000_000
	nowKtimeNs = func() uint64 { return 200_000_000_000 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Empty(t, lp.evictions)
}

func TestEvictionRule_FutureLostTimestampIgnored(t *testing.T) {
	// Degenerate: BPF reports a lostAt strictly greater than now (clock
	// skew, or BPF sampled slightly after our now-read). Rule's
	// "lostAt > now - graceWindow" branch must keep us waiting, not
	// evict with a future cutoff.
	sm, prog, lp := setupEvictionTest(t, 1*time.Second)
	origNow := nowKtimeNs
	t.Cleanup(func() { nowKtimeNs = origNow })

	lp.lostAt = 200_000_000_000
	nowKtimeNs = func() uint64 { return 100_000_000_000 }
	evaluateDropNotifyEviction(sm, prog)
	assert.Empty(t, lp.evictions)
}

// Verify the fakeLoadedProgram stub satisfies the new interface methods
// even for tests that don't customize DropNotifyLostAt.
func TestFakeLoadedProgram_SatisfiesInterface(_ *testing.T) {
	var _ LoadedProgram = &fakeLoadedProgram{}
	_ = loader.RuntimeStats{} // silence unused import if embedded alone
}
