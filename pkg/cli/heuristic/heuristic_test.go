// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package heuristic

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateStateFile redirects the state file to a test-specific temp directory for the
// duration of the test, preventing races between parallel test runs.
func isolateStateFile(t *testing.T) {
	t.Helper()
	prev := statePathOverride
	statePathOverride = filepath.Join(t.TempDir(), "heuristic-state.json")
	t.Cleanup(func() { statePathOverride = prev })
}

// clearAgentEnvVars unsets all known agent env vars for the duration of a test.
func clearAgentEnvVars(t *testing.T) {
	t.Helper()
	for _, env := range []string{"CLAUDECODE", "CURSOR_AGENT", "AIDER_MODEL", "CLINE", "WINDSURF_AGENT", "COPILOT_AGENT"} {
		t.Setenv(env, "")
	}
}

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
	isolateStateFile(t)
	clearAgentEnvVars(t)

	now := time.Unix(1700000000, 0).UTC()

	// Baseline: no flags
	scoreNoFlags, _ := BuildScore("agent status", []string{}, now)

	// Reset state so both calls share the same baseline (no inter-command latency)
	statePathOverride = filepath.Join(t.TempDir(), "heuristic-state-2.json")

	// With machine-readable flag — same timestamp, fresh session
	scoreWithFlags, _ := BuildScore("agent status", []string{"-j"}, now)

	assert.Equal(t, 30, scoreWithFlags-scoreNoFlags, "machine-readable flag should contribute exactly 30 points")
}

// TestInterCommandLatencySignal verifies that a second diagnostic command within 500ms adds +40.
func TestInterCommandLatencySignal(t *testing.T) {
	isolateStateFile(t)
	clearAgentEnvVars(t)

	now := time.Unix(1700000000, 0).UTC()
	scoreFirst, _ := BuildScore("agent status", []string{}, now)

	// Second diagnostic command within 500ms: +40 pts
	scoreSecond, _ := BuildScore("agent health", []string{}, now.Add(300*time.Millisecond))

	assert.GreaterOrEqual(t, scoreSecond-scoreFirst, 40, "inter-command latency <500ms should add at least 40 points")
}

// TestSessionResetClearsLatencySignal verifies that after inactivity timeout the
// inter-command latency signal does not fire.
func TestSessionResetClearsLatencySignal(t *testing.T) {
	isolateStateFile(t)
	clearAgentEnvVars(t)

	now := time.Unix(1700000000, 0).UTC()
	scoreFirst, _ := BuildScore("agent status", []string{}, now)

	// Command after inactivity timeout: session resets, no inter-command latency
	scoreAfterReset, _ := BuildScore("agent health", []string{}, now.Add(11*time.Minute))

	// The delta should be < 40 (the latency signal points), same other signals apply
	assert.Less(t, scoreAfterReset-scoreFirst, 40, "after session reset, inter-command latency signal should not fire")
}

func TestBuildScoreReturnsLabel(t *testing.T) {
	isolateStateFile(t)
	clearAgentEnvVars(t)

	now := time.Unix(1700000000, 0).UTC()
	_, label := BuildScore("agent status", []string{}, now)
	require.NotEmpty(t, label)
	assert.Contains(t, []string{"llm", "human", "unknown", "self_reported"}, label)
}

func TestHasMachineFirstFlags(t *testing.T) {
	assert.True(t, hasMachineFirstFlags([]string{"status", "-j"}))
	assert.True(t, hasMachineFirstFlags([]string{"health", "--json"}))
	assert.True(t, hasMachineFirstFlags([]string{"status", "--pretty-json"}))
	assert.False(t, hasMachineFirstFlags([]string{"status", "--verbose"}))
}

// TestBuildScoreAgentFlag verifies that --agent flag yields "self_reported" with score 100.
func TestBuildScoreAgentFlag(t *testing.T) {
	isolateStateFile(t)

	now := time.Unix(1700000000, 0).UTC()
	score, label := BuildScore("agent status", []string{"status", "--agent"}, now)

	assert.Equal(t, 100, score)
	assert.Equal(t, "self_reported", label)
}

// TestBuildScoreAgentEnvVar verifies that a known agent env var yields "self_reported" with score 100.
func TestBuildScoreAgentEnvVar(t *testing.T) {
	isolateStateFile(t)
	t.Setenv("CLAUDECODE", "1")

	now := time.Unix(1700000000, 0).UTC()
	score, label := BuildScore("agent status", []string{"status"}, now)

	assert.Equal(t, 100, score)
	assert.Equal(t, "self_reported", label)
}

// TestBuildScoreNoAgentMode verifies that without flag or env var, heuristic runs normally.
func TestBuildScoreNoAgentMode(t *testing.T) {
	isolateStateFile(t)
	clearAgentEnvVars(t)

	now := time.Unix(1700000000, 0).UTC()
	_, label := BuildScore("agent status", []string{"status"}, now)

	assert.NotEqual(t, "self_reported", label, "without --agent flag or env var, label should not be self_reported")
}

// TestAgentFlagKeepsLatencyTracking verifies that self-reported calls still update the state file
// with correct timestamps so subsequent heuristic calls see accurate latency data.
func TestAgentFlagKeepsLatencyTracking(t *testing.T) {
	isolateStateFile(t)

	now := time.Unix(1700000000, 0).UTC()

	_, _ = BuildScore("agent status", []string{"status", "--agent"}, now)

	st, err := loadState()
	require.NoError(t, err, "state file should be readable after --agent call")
	require.NotNil(t, st)
	assert.Equal(t, now.UnixMilli(), st.LastCommandUnixMilli, "LastCommandUnixMilli should match invocation time")
	// "agent status" is a diagnostic command — LastDiagnosticUnixMilli must also be set
	assert.Equal(t, now.UnixMilli(), st.LastDiagnosticUnixMilli, "LastDiagnosticUnixMilli should be set for a diagnostic command")
}

// TestBuildScoreAgentFlagExplicitTrue verifies that --agent=true is treated as agent mode.
func TestBuildScoreAgentFlagExplicitTrue(t *testing.T) {
	isolateStateFile(t)

	now := time.Unix(1700000000, 0).UTC()

	for _, form := range []string{"--agent=true", "--agent=1", "--agent=True", "--agent=TRUE"} {
		score, label := BuildScore("agent status", []string{"status", form}, now)
		assert.Equal(t, 100, score, "form %q should yield score 100", form)
		assert.Equal(t, "self_reported", label, "form %q should yield self_reported", form)
	}
}

// TestBuildScoreAgentFlagExplicitFalse verifies that --agent=false does not trigger agent mode,
// even when agent env vars are set (explicit opt-out takes precedence).
func TestBuildScoreAgentFlagExplicitFalse(t *testing.T) {
	isolateStateFile(t)
	// Set an agent env var — --agent=false must override it
	t.Setenv("CLAUDECODE", "1")

	now := time.Unix(1700000000, 0).UTC()

	for _, form := range []string{"--agent=false", "--agent=0", "--agent=False", "--agent=FALSE"} {
		_, label := BuildScore("agent status", []string{"status", form}, now)
		assert.NotEqual(t, "self_reported", label, "form %q should not yield self_reported", form)
	}
}
