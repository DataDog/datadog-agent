// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package interp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On Windows "/dev/null" names an ordinary file relative to the working
// directory, so Check must apply the input-redirect cat rule to it, as Run
// does; the native NUL spelling stays exempt.
func TestCheckInputRedirectUnixNullSpellingGatedOnWindows(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dev"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "dev", "null"), []byte("x\n"), 0o644))
	runner := checkRunner(t, AllowedCommands([]string{"rshell:tr"}), AllowedPaths([]string{root}))
	runner.Dir = root

	result := checkScript(t, runner, "tr a-z A-Z < /dev/null")
	assert.Equal(t, CheckDenied, result.Status, "%+v", result)
	assert.True(t, checkHasIssue(result, CheckCommandNotAllowed), "%+v", result)

	for _, spelling := range []string{"NUL", "nul"} {
		result = checkScript(t, runner, "tr a-z A-Z < "+spelling)
		assert.False(t, checkHasIssue(result, CheckCommandNotAllowed), "%s: %+v", spelling, result)
	}
}
