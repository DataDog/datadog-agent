// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcPathFlagNonexistentDir ensures --proc-path with a nonexistent
// directory causes ps -e to fail with a non-zero exit code.
func TestProcPathFlagNonexistentDir(t *testing.T) {
	code, _, stderr := runCLI(t,
		"--allow-all-commands",
		"--proc-path", "/nonexistent/proc/path",
		"-c", "ps -e",
	)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "ps:")
}

// writeFakeProcCLI creates a minimal fake proc filesystem and returns its path.
func writeFakeProcCLI(t *testing.T, pid int, name string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("cpu 0 0 0 0\nbtime 1000000000\n"), 0o644))
	pidDir := filepath.Join(dir, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(pidDir, 0o755))
	stat := fmt.Sprintf("%d (%s) S 0 %d %d 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 1 0 100\n", pid, name, pid, pid)
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "stat"), []byte(stat), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "status"), []byte("Name:\t"+name+"\nUid:\t1000 1000 1000 1000\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "cmdline"), []byte(name+"\x00"), 0o644))
	return dir
}

// TestProcPathFlagFakeProc ensures --proc-path with a valid fake proc tree
// causes ps -e to succeed and list processes from the fake tree.
func TestProcPathFlagFakeProc(t *testing.T) {
	procPath := writeFakeProcCLI(t, 1, "fakeinit")
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--proc-path", procPath,
		"-c", "ps -e",
	)
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "fakeinit")
}
