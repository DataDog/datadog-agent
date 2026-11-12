// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// countFiles returns the number of entries directly under dir.
func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	return len(entries)
}

// TestFileRemovalBudgetInitializedByRun verifies Run() installs the counter;
// without it the budget check is a no-op.
func TestFileRemovalBudgetInitializedByRun(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	assert.Nil(t, r.fileRemovalCount, "counter should be nil before Run")

	require.NoError(t, r.Run(context.Background(), parseScript(t, "true")))

	assert.NotNil(t, r.fileRemovalCount, "counter should be initialized after Run")
}

// TestFileRemovalBudgetSharedWithSubshells verifies that both ordinary and
// background (pipeline) subshells draw on the parent's budget, so
// `... | xargs -n1 rm` and `( rm ... )` cannot start a fresh allowance.
func TestFileRemovalBudgetSharedWithSubshells(t *testing.T) {
	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	require.NoError(t, r.Run(context.Background(), parseScript(t, "true")))
	require.NotNil(t, r.fileRemovalCount)

	assert.Same(t, r.fileRemovalCount, r.subshell(false).fileRemovalCount,
		"subshell must share the parent's fileRemovalCount pointer")
	assert.Same(t, r.fileRemovalCount, r.subshell(true).fileRemovalCount,
		"background subshell must share the parent's fileRemovalCount pointer")
}

// TestFileRemovalBudgetResetBetweenRuns verifies that the budget bounds one
// Run() call (a loop stops after MaxFileRemovalsPerRun files) and that a
// subsequent Run() on the same runner starts fresh — the budget is per run,
// not per runner lifetime.
func TestFileRemovalBudgetResetBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	total := builtins.MaxFileRemovalsPerRun + 3
	for i := range total {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), []byte("x"), 0o600))
	}

	r, _, errBuf := newRemediationRunner(t, dir)
	prog := parseScript(t, `for f in *.txt; do rm "$f"; done`)

	// The last three iterations hit the exhausted budget and fail, so the
	// script's final status is 1.
	err := r.Run(context.Background(), prog)
	var status ExitStatus
	require.ErrorAs(t, err, &status)
	assert.Equal(t, ExitStatus(1), status)
	assert.Equal(t, int64(builtins.MaxFileRemovalsPerRun), r.fileRemovalCount.Load(),
		"first run must stop at the budget")
	assert.Equal(t, 3, countFiles(t, dir), "files past the budget must survive")
	assert.Contains(t, errBuf.String(), "run-wide budget of 100 file removals is exhausted",
		"the error must name the budget so an agent can recover")

	errBuf.Reset()
	require.NoError(t, r.Run(context.Background(), prog))
	assert.Equal(t, int64(3), r.fileRemovalCount.Load(), "second run must start from a fresh budget")
	assert.Equal(t, 0, countFiles(t, dir))
	assert.Empty(t, errBuf.String())
}

// TestFileRemovalBudgetNotChargedForFailedRemovals verifies that removals that
// never happened (missing file) do not consume a legitimate operator's
// allowance.
func TestFileRemovalBudgetNotChargedForFailedRemovals(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.txt"), []byte("x"), 0o600))

	r, _, _ := newRemediationRunner(t, dir)
	script := `for i in 1 2 3 4 5 6 7 8 9; do rm missing.txt; done
rm real.txt`
	require.NoError(t, r.Run(context.Background(), parseScript(t, script)))

	assert.Equal(t, int64(1), r.fileRemovalCount.Load(),
		"only the successful removal should be charged")
	assert.Equal(t, 0, countFiles(t, dir))
}
