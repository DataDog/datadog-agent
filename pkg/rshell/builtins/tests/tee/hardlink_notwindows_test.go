// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Hard-link write-target rejection is unix-only: Windows cannot report a
// link count from an open handle without a new syscall surface (see
// allowedpaths/hardlink_windows.go and the hard-link entry in AGENTS.md), so
// the guard degrades to not-enforced there. This mirrors
// builtins/tests/truncate/hardlink_unix_test.go.
package tee_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTeeRejectsHardLinkedWriteTarget is the regression test for the
// path-based-containment gap: AllowedPaths resolves paths, but a hard link
// inside a :rw root is a second name for an inode that may also be named
// elsewhere. Writing to it would mutate the shared content while every path
// check passes.
func TestTeeRejectsHardLinkedWriteTarget(t *testing.T) {
	dir := t.TempDir()
	orig := writeFile(t, dir, "orig.txt", "original content")
	linked := filepath.Join(dir, "linked.txt")
	if err := os.Link(orig, linked); err != nil {
		t.Skipf("hard links unsupported on this filesystem: %v", err)
	}

	_, stderr, code := teeRunStdin(t, "tee linked.txt", dir, "new\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "hard links are not supported")
	// The original file must be untouched — the write must never reach
	// the shared inode.
	assert.Equal(t, "original content", readFile(t, orig))
}
