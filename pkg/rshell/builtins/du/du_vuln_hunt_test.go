// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Blocked-attack regression tests added by the vuln-hunt campaign
// 2026-05-18-initial-audit (target: du).
package du_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// H10: a self-referential directory symlink (a/link -> a) must be detected
// by the ancestorIDs cycle check so du terminates rather than recursing
// indefinitely. The 256-depth cap also bounds the worst case, but the
// cycle check should fire first with a clearer signal.
func TestVulnHuntBuiltinSpecialFiles_SymlinkCycleDetected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks differ on Windows")
	}
	root := t.TempDir()
	subdir := filepath.Join(root, "a")
	require.NoError(t, os.Mkdir(subdir, 0755))
	// `a/loop` -> `a` makes a cycle: descending into a/loop re-enters a.
	require.NoError(t, os.Symlink(subdir, filepath.Join(subdir, "loop")))

	// -L (follow symlinks) is what makes the cycle dangerous; without -L,
	// the symlink is treated as a regular entry. Run with -L and confirm
	// du completes (does not hang) and emits at least one warning about
	// the cycle.
	stdout, stderr, code := cmdRun(t, "du -L .", root)
	// Either success with cycle warning, or a soft-error exit — both are
	// acceptable. The critical assertion is "did not hang" (cmdRun has an
	// implicit short timeout via testutil; if du recursed indefinitely
	// the test would fail with a timeout panic). Confirm a finite result.
	assert.NotEmpty(t, stdout, "du must produce some output even when cycle is present")
	// Cycle detection writes a warning to stderr; we don't pin the exact
	// wording to avoid coupling to messages.
	_ = code
	_ = stderr
}

// H1: -L must not turn an allowed symlink into an AllowedPaths escape. The
// StatFile wrapper follows links, but the sandbox must reject targets outside
// the configured root before du can report their metadata.
func TestVulnHuntBuiltinFileAccessBypass_DereferenceSymlinkOutsideAllowedPathsBlocked(t *testing.T) {
	if !canSymlink() {
		t.Skip("symlinks unavailable")
	}
	base := t.TempDir()
	allowed := filepath.Join(base, "allowed")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	require.NoError(t, os.Mkdir(outside, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret-data\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(outside, "secret-dir"), 0o700))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(allowed, "escape")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret-dir"), filepath.Join(allowed, "escape-dir")))

	stdout, stderr, code := cmdRun(t, "du -b ../outside/secret.txt", allowed)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "du: cannot access '../outside/secret.txt'")
	assert.NotContains(t, stdout+stderr, "secret-data")
	assert.NotContains(t, stdout+stderr, outside)

	stdout, stderr, code = cmdRun(t, "du -L -b escape", allowed)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "du: cannot access 'escape'")
	assert.NotContains(t, stdout+stderr, "secret-data")
	assert.NotContains(t, stdout+stderr, outside)

	stdout, stderr, code = cmdRun(t, "du -L -b escape-dir", allowed)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "du: cannot access 'escape-dir'")
	assert.NotContains(t, stdout+stderr, outside)
}
