// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package workloadselectionimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetFileReadableByEveryone_FileCanBeOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-policy.bin")

	// Create initial file
	require.NoError(t, os.WriteFile(path, []byte("initial content"), 0644))

	// Apply the ACL (this was previously making the file unwritable)
	require.NoError(t, setFileReadableByEveryone(path))

	// Verify the file can still be overwritten by the current user
	err := os.WriteFile(path, []byte("updated content"), 0644)
	assert.NoError(t, err, "file should be writable after setFileReadableByEveryone")

	// Verify the content was actually updated
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "updated content", string(content))
}
