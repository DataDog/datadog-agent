// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package cd_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// TestCdReservedNameDoesNotHang ensures `cd` to a Windows reserved name
// (CON, NUL, etc.) fails fast — not by hanging on the device file. The
// sandbox.Stat call should reject these because they are not regular
// directories under the AllowedPaths root.
func TestCdReservedNameDoesNotHang(t *testing.T) {
	dir := canonicalTempDir(t)
	for _, name := range []string{"CON", "NUL", "PRN", "AUX", "COM1", "LPT1"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, stderr, code := testutil.RunScriptCtx(ctx, t, "cd "+name, dir,
				interp.AllowedPaths([]string{dir}))
			assert.Equal(t, 1, code, "cd %s must exit 1, not hang", name)
			assert.Contains(t, stderr, "cd:")
		})
	}
}

// TestCdCaseInsensitiveOnNTFS verifies that on Windows (NTFS, default
// case-insensitive), `cd Child` succeeds when the directory is named
// `child`.
func TestCdCaseInsensitiveOnNTFS(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "child"), 0o755))
	stdout, _, code := cdRun(t, "cd Child; pwd", dir)
	assert.Equal(t, 0, code)
	// Exact case echoed back depends on the platform; just verify pwd
	// resolved a non-empty absolute path containing "child".
	assert.True(t, strings.Contains(strings.ToLower(stdout), "child"))
}

// TestCdForwardSlashSeparator verifies that a shell-style forward-slash
// path argument is accepted on Windows. Win32 APIs accept both `/` and
// `\\` as separators; rshell's `cd` should not be more restrictive.
func TestCdForwardSlashSeparator(t *testing.T) {
	dir := canonicalTempDir(t)
	nested := filepath.Join(dir, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	stdout, stderr, code := cdRun(t, "cd a/b; pwd", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Contains(t, stdout, "a")
	assert.Contains(t, stdout, "b")
}
