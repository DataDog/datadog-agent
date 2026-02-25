// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetExecutable tests the GetExecutable function
func TestGetExecutable(t *testing.T) {
	// This test verifies that GetExecutable returns a valid path
	// It should either return the result of os.Executable() or fall back to argv[0]
	executable, err := GetExecutable()
	require.NoError(t, err)
	assert.NotEmpty(t, executable)

	// Verify it's an absolute path
	assert.True(t, filepath.IsAbs(executable), "executable path should be absolute")

	// Verify the file exists
	_, err = os.Stat(executable)
	assert.NoError(t, err, "executable file should exist")
}

// TestFromArgv0WithValidPath tests fromArgv0 with a valid argv[0]
func TestFromArgv0WithValidPath(t *testing.T) {
	// Save original os.Args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test with absolute path to current test binary
	executable, err := os.Executable()
	require.NoError(t, err)

	os.Args = []string{executable}
	result, err := fromArgv0()
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.True(t, filepath.IsAbs(result))
}

// TestFromArgv0WithRelativePath tests fromArgv0 with a relative path
func TestFromArgv0WithRelativePath(t *testing.T) {
	// Save original os.Args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test with relative path that can be found in PATH
	os.Args = []string{"go"} // 'go' binary should be in PATH
	result, err := fromArgv0()
	// This might succeed or fail depending on the system
	// If it succeeds, result should be an absolute path
	if err == nil {
		assert.True(t, filepath.IsAbs(result))
	}
}

// TestFromArgv0WithEmptyArgs tests fromArgv0 with empty os.Args
func TestFromArgv0WithEmptyArgs(t *testing.T) {
	// Save original os.Args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test with empty os.Args
	os.Args = []string{}
	_, err := fromArgv0()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty argv[0]")
}

// TestFromArgv0WithEmptyString tests fromArgv0 with empty string in argv[0]
func TestFromArgv0WithEmptyString(t *testing.T) {
	// Save original os.Args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test with empty string in argv[0]
	os.Args = []string{""}
	_, err := fromArgv0()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty argv[0]")
}

// TestFromArgv0WithNonexistentPath tests fromArgv0 with a nonexistent path
func TestFromArgv0WithNonexistentPath(t *testing.T) {
	// Save original os.Args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test with a path that doesn't exist
	os.Args = []string{"nonexistent-binary-that-should-not-exist-12345"}
	_, err := fromArgv0()
	assert.Error(t, err)
}
