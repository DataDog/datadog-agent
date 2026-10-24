// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlobReadDirCountInitializedByRun verifies that Run() initializes the
// globReadDirCount counter (it must be non-nil for the limit check to work).
func TestGlobReadDirCountInitializedByRun(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	assert.Nil(t, r.globReadDirCount, "counter should be nil before Run")

	err = r.Run(context.Background(), parseScript(t, "true"))
	require.NoError(t, err)

	assert.NotNil(t, r.globReadDirCount, "counter should be initialized after Run")
}

// TestGlobReadDirCountResetBetweenRuns verifies that each Run() call creates
// a fresh counter, so a script that used many ReadDir calls in the first run
// does not affect the budget of the second run.
func TestGlobReadDirCountResetBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	for i := range 3 {
		f, err := os.Create(filepath.Join(dir, "f"+string(rune('a'+i))))
		require.NoError(t, err)
		f.Close()
	}

	r, err := New(allowAllCommandsOpt(), AllowedPaths([]string{dir}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	// First run: consume some ReadDir budget.
	prog := parseScript(t, "echo * * * * *")
	err = r.Run(context.Background(), prog)
	require.NoError(t, err)
	firstCount := r.globReadDirCount.Load()
	assert.Greater(t, firstCount, int64(0), "first run should have consumed ReadDir calls")

	// Second run: counter should be fresh (not accumulated).
	err = r.Run(context.Background(), prog)
	require.NoError(t, err)
	secondCount := r.globReadDirCount.Load()
	assert.Equal(t, firstCount, secondCount,
		"second run should have the same count as first (fresh counter, same script)")
}

// TestGlobReadDirCountSharedWithSubshell verifies that subshell() shares the
// same atomic counter pointer with the parent, so the limit is enforced
// across the entire Run() tree.
func TestGlobReadDirCountSharedWithSubshell(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	err = r.Run(context.Background(), parseScript(t, "true"))
	require.NoError(t, err)
	require.NotNil(t, r.globReadDirCount)

	sub := r.subshell(false)
	assert.Same(t, r.globReadDirCount, sub.globReadDirCount,
		"subshell must share the parent's globReadDirCount pointer")
}

// TestGlobReadDirCountSharedWithBackgroundSubshell verifies that background
// subshells (used for pipes) also share the same counter.
func TestGlobReadDirCountSharedWithBackgroundSubshell(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	err = r.Run(context.Background(), parseScript(t, "true"))
	require.NoError(t, err)
	require.NotNil(t, r.globReadDirCount)

	sub := r.subshell(true)
	assert.Same(t, r.globReadDirCount, sub.globReadDirCount,
		"background subshell must share the parent's globReadDirCount pointer")
}

// TestGlobReadDirCountNotIncrementedForQuotedStrings verifies that quoted
// strings (which don't trigger glob expansion) do not increment the counter.
func TestGlobReadDirCountNotIncrementedForQuotedStrings(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	f.Close()

	r, err := New(allowAllCommandsOpt(), AllowedPaths([]string{dir}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	// Quoted "*" should not trigger ReadDir.
	err = r.Run(context.Background(), parseScript(t, `echo "*" '*' "hello"`))
	require.NoError(t, err)

	assert.Equal(t, int64(0), r.globReadDirCount.Load(),
		"quoted strings should not trigger ReadDir calls")
}

// TestGlobReadDirCountIncrementsForUnquotedGlob verifies that unquoted globs
// do increment the counter.
func TestGlobReadDirCountIncrementsForUnquotedGlob(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	f.Close()

	r, err := New(allowAllCommandsOpt(), AllowedPaths([]string{dir}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	err = r.Run(context.Background(), parseScript(t, "echo * *.txt"))
	require.NoError(t, err)

	assert.Equal(t, int64(2), r.globReadDirCount.Load(),
		"two unquoted glob patterns should trigger 2 ReadDir calls")
}
