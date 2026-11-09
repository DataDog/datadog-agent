// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package tests_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPentestInputRedirect_UnixNullSpellingGatedOnWindows verifies that on
// Windows the Unix spelling "/dev/null" is not exempt from the cat
// requirement: the sandbox resolves it relative to the working directory,
// so it can name an ordinary file.
func TestPentestInputRedirect_UnixNullSpellingGatedOnWindows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dev"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dev", "null"), []byte("TOP SECRET\n"), 0o644))

	stdout, stderr, code := runBypass(t,
		`tr A-Z a-z < /dev/null; echo "rc=$?"`,
		dir,
		[]string{"rshell:tr", "rshell:echo"},
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "rc=1\n", stdout, "must not leak file contents")
	assert.Contains(t, stderr, "< /dev/null: file read not permitted (cat not in allowed commands)")
	assert.NotContains(t, strings.ToLower(stdout), "top secret")
}

// TestPentestInputRedirect_NativeNulNotGatedOnCat verifies that the native
// Windows null device spelling stays exempt from the cat requirement.
func TestPentestInputRedirect_NativeNulNotGatedOnCat(t *testing.T) {
	for _, spelling := range []string{"NUL", "nul"} {
		_, stderr, _ := runBypass(t,
			"tr A-Z a-z < "+spelling,
			t.TempDir(),
			[]string{"rshell:tr"},
		)
		assert.NotContains(t, stderr, "cat not in allowed commands", spelling)
	}
}
