// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenShared(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")

	// Create a test file
	content := []byte("test content")
	require.NoError(t, os.WriteFile(testFile, content, 0644))

	// Open the file using OpenShared
	f, err := OpenShared(testFile)
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()

	// Read the content
	data := make([]byte, len(content))
	n, err := f.Read(data)
	require.NoError(t, err)
	assert.Equal(t, len(content), n)
	assert.Equal(t, content, data)
}

func TestOpenSharedNonExistent(t *testing.T) {
	// Try to open a non-existent file
	_, err := OpenShared("/nonexistent/path/to/file.txt")
	assert.Error(t, err)
}

func TestOpenSharedDirectory(t *testing.T) {
	dir := t.TempDir()

	// Opening a directory should work (returns directory listing behavior)
	f, err := OpenShared(dir)
	require.NoError(t, err)
	require.NotNil(t, f)
	f.Close()
}

func TestOpenSharedAllowsDelete(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "deletable.txt")

	// Create a test file
	require.NoError(t, os.WriteFile(testFile, []byte("data"), 0644))

	// Open the file
	f, err := OpenShared(testFile)
	require.NoError(t, err)
	defer f.Close()

	// On Unix, we should be able to delete a file while it's open
	err = os.Remove(testFile)
	require.NoError(t, err)

	// File should no longer exist
	assert.False(t, FileExists(testFile))
}
