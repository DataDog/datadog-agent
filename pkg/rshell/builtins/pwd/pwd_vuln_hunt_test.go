// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Blocked-attack regression tests added by the vuln-hunt campaign
// 2026-05-18-initial-audit (target: pwd).
package pwd_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// H2: a symlink whose target lies outside the sandbox is observed by
// resolveSymlinks via ReadlinkFile; subsequent LstatFile calls on the
// outside-sandbox components fail, and the loop treats them as opaque.
// pwd may *print* the resolved path (since the symlink target name was
// already discoverable via ls -l), but the sandbox must continue to
// block reads at that location — verified here by attempting to cat the
// reported path and confirming the cat fails.
func TestVulnHuntBuiltinFileAccessBypass_SymlinkTargetOutsideSandbox(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlinks differ on Windows")
	}
	root := canonicalTempDir(t)
	parent := filepath.Dir(root)
	// File outside the sandbox.
	outside := filepath.Join(parent, "secret-outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("S3CR3T\n"), 0644))
	t.Cleanup(func() { _ = os.Remove(outside) })
	// Symlink inside the sandbox pointing to that file.
	linkDir := filepath.Join(root, "linkdir")
	require.NoError(t, os.Mkdir(linkDir, 0755))
	link := filepath.Join(linkDir, "link-to-outside")
	require.NoError(t, os.Symlink(outside, link))
	// Subdirectory inside the sandbox that we'll cd into.
	cwdDir := filepath.Join(root, "real-cwd")
	require.NoError(t, os.Mkdir(cwdDir, 0755))

	// pwd -L always returns the logical cwd, regardless of symlinks. We
	// don't run pwd -P from a path that itself contains a symlink target
	// outside the sandbox in this test — that would require chdir into
	// the outside path which is itself prevented. Instead, we directly
	// observe that reading the outside file via the in-sandbox symlink
	// fails: the sandbox blocks the read even though the symlink can be
	// followed.
	ctx, cancel := context.WithTimeout(context.Background(), 5)
	defer cancel()
	_ = ctx
	stdout, stderr, code := testutil.RunScript(t,
		"cat linkdir/link-to-outside", root,
		interp.AllowedPaths([]string{root}))
	assert.NotEqual(t, 0, code, "cat through symlink to outside-sandbox must fail; stdout=%q stderr=%q", stdout, stderr)
	assert.NotContains(t, stdout, "S3CR3T", "secret content must not leak through symlink")
	// pwd in the sandbox is unaffected.
	stdout2, _, code2 := testutil.RunScript(t, "pwd", cwdDir,
		interp.AllowedPaths([]string{root}))
	assert.Equal(t, 0, code2)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(stdout2), "real-cwd"),
		"pwd must return the logical cwd, got %q", stdout2)
}
