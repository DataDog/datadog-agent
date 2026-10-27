// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnsureRecursiveDoesNotChmodThroughSymlink checks that a recursive Ensure
// does not follow symlinks when applying a mode.
func TestEnsureRecursiveDoesNotChmodThroughSymlink(t *testing.T) {
	tree := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "real"), []byte("x"), 0600))
	require.NoError(t, os.Symlink(target, filepath.Join(tree, "planted")))

	// Owner/Group empty so the test needs no privileges; only Mode applies.
	p := Permission{Path: ".", Mode: 0755, Recursive: true}
	require.NoError(t, p.Ensure(context.Background(), tree))

	real, err := os.Stat(filepath.Join(tree, "real"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), real.Mode().Perm(),
		"regular files inside the tree must still get the mode")

	outside, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), outside.Mode().Perm(),
		"chmod must not follow a planted symlink onto its target")
}

// TestEnsureRecursiveMissingFileIsIgnored checks a file missing mid-walk does
// not fail the pass.
func TestEnsureRecursiveMissingFileIsIgnored(t *testing.T) {
	tree := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tree, "real"), []byte("x"), 0600))

	p := Permission{Path: ".", Mode: 0750, Recursive: true}
	require.NoError(t, p.Ensure(context.Background(), tree))

	info, err := os.Stat(filepath.Join(tree, "real"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0750), info.Mode().Perm())
}
