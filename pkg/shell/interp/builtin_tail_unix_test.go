// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package interp

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTail_SymlinkToRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := writeTempFile(t, dir, "target.txt", "link content\n")
	link := filepath.Join(dir, "link.txt")
	os.Symlink(target, link)

	stdout, _, ec := runTail(t, fmt.Sprintf("tail %s", link))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "link content\n", stdout)
}

func TestTail_DanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling.txt")
	os.Symlink(filepath.Join(dir, "nonexistent"), link)

	_, stderr, ec := runTail(t, fmt.Sprintf("tail %s", link))
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "tail:")
}

func TestTail_UnixSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer l.Close()

	_, stderr, ec := runTail(t, fmt.Sprintf("tail %s", sock))
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "not a regular file")
}

func TestTail_DevNull(t *testing.T) {
	// /dev/null is a character device, not a regular file — should be rejected.
	_, stderr, ec := runTail(t, "tail /dev/null")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "not a regular file")
}
