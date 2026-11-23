// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package wc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWcSymlinkToFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "target.txt", "hello\n")
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link.txt")))
	stdout, _, code := cmdRun(t, "wc link.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1 1 6 link.txt\n", stdout)
}

func TestWcDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink("nonexistent", filepath.Join(dir, "dangle.txt")))
	stdout, stderr, code := cmdRun(t, "wc dangle.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "wc:")
}
