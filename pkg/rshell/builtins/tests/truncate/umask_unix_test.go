// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package truncate_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTruncateUmask verifies that a newly created file has permissions
// derived from 0666 & ~umask, not a fixed mode.
func TestTruncateUmask(t *testing.T) {
	dir := t.TempDir()

	// Force a predictable umask of 022 → expected perm 0644.
	old := syscall.Umask(022)
	defer syscall.Umask(old)

	_, _, code := truncateRun(t, "truncate -s 10 newfile.txt", dir)
	require.Equal(t, 0, code)

	info, err := os.Stat(filepath.Join(dir, "newfile.txt"))
	require.NoError(t, err)
	// Mask to the 9 permission bits only.
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0644), perm,
		"expected 0644 (0666 & ~022), got %04o", perm)
}
