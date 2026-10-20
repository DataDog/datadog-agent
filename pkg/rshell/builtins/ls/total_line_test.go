// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ls_test

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func TestLsLongTotalLineUses1KBlocks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Stat_t.Blocks not available on Windows")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/a.txt", []byte("hello"), 0o644))

	stdout, stderr, code := testutil.RunScript(t, "ls -l", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.True(t, len(lines) >= 1, "expected at least 1 line of output")
	assert.True(t, strings.HasPrefix(lines[0], "total "), "expected total line, got: %s", lines[0])
}

func TestLsLongTotalLineSuppressedOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/a.txt", []byte("hello"), 0o644))

	stdout, stderr, code := testutil.RunScript(t, "ls -l", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.True(t, len(lines) >= 1, "expected at least 1 line of output")
	assert.False(t, strings.HasPrefix(lines[0], "total "),
		"total line should be suppressed on Windows, got: %s", lines[0])
}

func TestLsLongNlinkIsValid(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/a.txt", []byte("hello"), 0o644))

	stdout, stderr, code := testutil.RunScript(t, "ls -l a.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)

	// Long format: mode nlink owner group size date name
	// Nlink must be either a positive integer or "?" (unknown).
	// It must never be "0" — that's impossible for an existing file.
	fields := strings.Fields(stdout)
	require.True(t, len(fields) >= 2, "expected at least 2 fields, got: %s", stdout)
	nlink := fields[1]
	assert.NotEqual(t, "0", nlink, "nlink should not be 0 for an existing file")

	if nlink != "?" {
		nlinkVal := 0
		n, err := fmt.Sscanf(nlink, "%d", &nlinkVal)
		require.NoError(t, err, "nlink should be a valid integer")
		require.Equal(t, 1, n, "expected exactly one parsed value")
		assert.GreaterOrEqual(t, nlinkVal, 1, "nlink should be >= 1")
	}
}
