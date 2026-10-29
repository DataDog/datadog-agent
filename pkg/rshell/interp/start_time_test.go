// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

func parseScript(t *testing.T, src string) *syntax.File {
	t.Helper()
	prog, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	require.NoError(t, err)
	return prog
}

// TestStartTimeZeroBeforeRun verifies that startTime is not set until Run is
// called, so callers cannot accidentally observe a stale time from a previous
// run. Uses New() directly (not newResetRunner) to check the initial zero-value
// state before any Run or Reset call.
func TestStartTimeZeroBeforeRun(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	assert.True(t, r.startTime.IsZero(), "startTime should be zero before Run")
}

// TestStartTimeSetByRun verifies that Run captures the current time into
// startTime before executing any builtins.
func TestStartTimeSetByRun(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	before := time.Now()
	err = r.Run(context.Background(), parseScript(t, "true"))
	after := time.Now()
	require.NoError(t, err)

	assert.False(t, r.startTime.IsZero(), "startTime should be set after Run")
	assert.True(t, !r.startTime.Before(before), "startTime should be >= time before Run")
	assert.True(t, !r.startTime.After(after), "startTime should be <= time after Run")
}

// TestStartTimeUpdatesOnSubsequentRun verifies that each Run call captures a
// fresh timestamp, so commands in different runs do not share a stale time.
func TestStartTimeUpdatesOnSubsequentRun(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	prog := parseScript(t, "true")

	err = r.Run(context.Background(), prog)
	require.NoError(t, err)
	first := r.startTime

	// Sleep to ensure the wall clock advances between runs. Windows default
	// timer resolution is ~15 ms, so 50 ms provides a safe margin.
	time.Sleep(50 * time.Millisecond)

	err = r.Run(context.Background(), prog)
	require.NoError(t, err)
	second := r.startTime

	assert.True(t, second.After(first), "startTime should advance between Run calls")
}

// TestStartTimePropagatedToSubshell verifies that a child runner created by
// subshell() inherits the parent's startTime so builtins in subshells and
// pipelines use the correct time reference.
func TestStartTimePropagatedToSubshell(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	err = r.Run(context.Background(), parseScript(t, "true"))
	require.NoError(t, err)
	require.False(t, r.startTime.IsZero())

	sub := r.subshell(false)
	assert.Equal(t, r.startTime, sub.startTime,
		"subshell must inherit parent startTime")
}

// TestStartTimeResetToZeroByReset verifies that Reset clears startTime so that
// a runner that has been reset but not yet re-run does not expose the previous
// run's timestamp.
func TestStartTimeResetToZeroByReset(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	err = r.Run(context.Background(), parseScript(t, "true"))
	require.NoError(t, err)
	require.False(t, r.startTime.IsZero(), "startTime should be set after Run")

	r.Reset()
	assert.True(t, r.startTime.IsZero(), "startTime should be cleared by Reset")
}
