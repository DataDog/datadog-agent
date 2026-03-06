// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestListFiles_BasicListing(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("hello"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755))

	out, err := runListFiles(t, tmpDir)
	require.NoError(t, err)
	assert.Empty(t, out.Error)
	assert.Len(t, out.Entries, 2)

	byName := indexByName(out.Entries)
	assert.False(t, byName["file.txt"].IsDir)
	assert.Equal(t, int64(5), byName["file.txt"].Size)
	assert.True(t, byName["subdir"].IsDir)
	assert.Equal(t, filepath.Join(tmpDir, "file.txt"), byName["file.txt"].Path)
}

func TestListFiles_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	out, err := runListFiles(t, tmpDir)
	require.NoError(t, err)
	assert.Empty(t, out.Error)
	assert.Empty(t, out.Entries)
}

func TestListFiles_NonExistentPath(t *testing.T) {
	out, err := runListFiles(t, "/nonexistent/path/that/does/not/exist")
	require.NoError(t, err)
	assert.NotEmpty(t, out.Error)
	assert.Empty(t, out.Entries)
}

func TestListFiles_RelativePathRejected(t *testing.T) {
	handler := NewListFilesHandler()
	task := makeTask(t, map[string]interface{}{"path": "relative/path"})
	_, err := handler.Run(t.Context(), task, nil)
	assert.Error(t, err)
}

func TestListFiles_NonRecursive(t *testing.T) {
	tmpDir := t.TempDir()

	nested := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.MkdirAll(nested, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "deep.txt"), []byte("deep"), 0644))

	out, err := runListFiles(t, tmpDir)
	require.NoError(t, err)

	// Only "subdir" should appear, not "subdir/deep.txt"
	assert.Len(t, out.Entries, 1)
	assert.Equal(t, "subdir", out.Entries[0].Name)
}

func TestListFiles_ModTimeSet(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "f.log"), []byte("x"), 0644))

	out, err := runListFiles(t, tmpDir)
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	assert.NotEmpty(t, out.Entries[0].ModTime)
}

// --- Host path conversion tests (kept from prior suite) ---

func TestToHostPath(t *testing.T) {
	assert.Equal(t, "/host/var/log", toHostPath("/host", "/var/log"))
	assert.Equal(t, "/var/log", toHostPath("", "/var/log"))
}

func TestToOutputPath(t *testing.T) {
	assert.Equal(t, "/var/log", toOutputPath("/host", "/host/var/log"))
	assert.Equal(t, "/var/log", toOutputPath("", "/var/log"))
}

// --- helpers ---

func runListFiles(t *testing.T, path string) (*ListFilesOutputs, error) {
	t.Helper()
	handler := NewListFilesHandler()
	task := makeTask(t, map[string]interface{}{"path": path})
	result, err := handler.Run(t.Context(), task, nil)
	if err != nil {
		return nil, err
	}
	out, ok := result.(*ListFilesOutputs)
	require.True(t, ok)
	return out, nil
}

func makeTask(t *testing.T, inputs map[string]interface{}) *types.Task {
	t.Helper()
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: inputs}
	return task
}

func indexByName(entries []FileEntry) map[string]FileEntry {
	m := make(map[string]FileEntry, len(entries))
	for _, e := range entries {
		m[e.Name] = e
	}
	return m
}
