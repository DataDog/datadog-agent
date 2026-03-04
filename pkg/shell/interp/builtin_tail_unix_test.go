// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package interp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTail_Symlink_Followed verifies that tail follows symlinks for read
// operations (by design — relies on OS permissions, not interpreter policy).
func TestTail_Symlink_Followed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("via symlink\n"), 0644))
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink(target, link))

	stdout, _, err := runTailScript(t, dir, `tail link.txt`)
	require.NoError(t, err)
	assert.Equal(t, "via symlink\n", stdout)
}

// TestTail_Directory_Error verifies that passing a directory as a file
// argument produces a non-empty stderr and exit code 1.
func TestTail_Directory_Error(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(sub, 0755))

	_, stderr, err := runTailScript(t, dir, `tail subdir`)
	require.NoError(t, err)
	assert.NotEmpty(t, stderr)
}
