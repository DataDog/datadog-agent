// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package heuristic

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scoreLabel is the bucketing logic extracted for unit-testing label boundaries.
func scoreLabel(score int) string {
	switch {
	case score >= 65:
		return "llm"
	case score < 25:
		return "human"
	default:
		return "unknown"
	}
}

func TestScoreLabelBoundaries(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{0, "human"},
		{24, "human"},
		{25, "unknown"},
		{64, "unknown"},
		{65, "llm"},
		{100, "llm"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, scoreLabel(tc.score), "score=%d", tc.score)
	}
}

// TestMachineFirstFlagsAddScore verifies that machine-readable flags contribute +30 to the
// score relative to a baseline call with the same other signals.
func TestMachineFirstFlagsAddScore(t *testing.T) {
	_ = os.Remove(statePath())
	t.Cleanup(func() { _ = os.Remove(statePath()) })

	now := time.Unix(1700000000, 0).UTC()

	// Baseline: no flags
	scoreNoFlags, _ := BuildScore("agent status", []string{}, now)

	// Reset state so both calls share the same baseline (no inter-command latency)
	require.NoError(t, os.Remove(statePath()))

	// With machine-readable flag — same timestamp, fresh session
	scoreWithFlags, _ := BuildScore("agent status", []string{"-j"}, now)

	assert.Equal(t, 30, scoreWithFlags-scoreNoFlags, "machine-readable flag should contribute exactly 30 points")
}

// TestInterCommandLatencySignal verifies that a second diagnostic command within 500ms adds +40.
func TestInterCommandLatencySignal(t *testing.T) {
	_ = os.Remove(statePath())
	t.Cleanup(func() { _ = os.Remove(statePath()) })

	now := time.Unix(1700000000, 0).UTC()
	scoreFirst, _ := BuildScore("agent status", []string{}, now)

	// Second diagnostic command within 500ms: +40 pts
	scoreSecond, _ := BuildScore("agent health", []string{}, now.Add(300*time.Millisecond))

	assert.GreaterOrEqual(t, scoreSecond-scoreFirst, 40, "inter-command latency <500ms should add at least 40 points")
}

// TestSessionResetClearsLatencySignal verifies that after inactivity timeout the
// inter-command latency signal does not fire.
func TestSessionResetClearsLatencySignal(t *testing.T) {
	_ = os.Remove(statePath())
	t.Cleanup(func() { _ = os.Remove(statePath()) })

	now := time.Unix(1700000000, 0).UTC()
	scoreFirst, _ := BuildScore("agent status", []string{}, now)

	// Command after inactivity timeout: session resets, no inter-command latency
	scoreAfterReset, _ := BuildScore("agent health", []string{}, now.Add(11*time.Minute))

	// The delta should be < 40 (the latency signal points), same other signals apply
	assert.Less(t, scoreAfterReset-scoreFirst, 40, "after session reset, inter-command latency signal should not fire")
}

func TestBuildScoreReturnsLabel(t *testing.T) {
	_ = os.Remove(statePath())
	t.Cleanup(func() { _ = os.Remove(statePath()) })

	now := time.Unix(1700000000, 0).UTC()
	_, label := BuildScore("agent status", []string{}, now)
	require.NotEmpty(t, label)
	assert.Contains(t, []string{"llm", "human", "unknown"}, label)
}

func TestHasMachineFirstFlags(t *testing.T) {
	assert.True(t, hasMachineFirstFlags([]string{"status", "-j"}))
	assert.True(t, hasMachineFirstFlags([]string{"health", "--json"}))
	assert.True(t, hasMachineFirstFlags([]string{"status", "--pretty-json"}))
	assert.False(t, hasMachineFirstFlags([]string{"status", "--verbose"}))
}
