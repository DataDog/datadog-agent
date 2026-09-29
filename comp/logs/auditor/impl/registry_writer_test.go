// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package auditorimpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicRegistryWriter(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")
	registryDirPath := tmpDir
	registryTmpFile := "registry.json.tmp"
	testData := []byte(`{"test": "data"}`)
	require.NoError(t, os.WriteFile(registryPath, []byte(`{"old": "data"}`), 0644))

	// Create atomic registry writer
	writer := NewAtomicRegistryWriter()

	// Test writing registry
	err := writer.WriteRegistry(registryPath, registryDirPath, registryTmpFile, testData)
	require.NoError(t, err)

	// Verify file exists and has correct content
	content, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	assert.Equal(t, testData, content)
}

func TestAtomicRegistryWriterCleansUpTemporaryFileWhenReplacementFails(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")
	require.NoError(t, os.Mkdir(registryPath, 0755))

	writer := NewAtomicRegistryWriter()
	err := writer.WriteRegistry(registryPath, tmpDir, "registry.json.tmp", []byte(`{"test": "data"}`))
	require.Error(t, err)

	temporaryFiles, err := filepath.Glob(filepath.Join(tmpDir, "registry.json.tmp*"))
	require.NoError(t, err)
	assert.Empty(t, temporaryFiles)
}

func TestAtomicRegistryWriterSupportsLongPaths(t *testing.T) {
	registryDirPath := t.TempDir()
	for len(registryDirPath) < 260 {
		registryDirPath = filepath.Join(registryDirPath, strings.Repeat("a", 32))
		require.NoError(t, os.Mkdir(registryDirPath, 0755))
	}
	registryPath := filepath.Join(registryDirPath, "registry.json")
	testData := []byte(`{"test": "data"}`)
	require.Greater(t, len(registryPath), 260)
	require.NoError(t, os.WriteFile(registryPath, []byte(`{"old": "data"}`), 0644))

	writer := NewAtomicRegistryWriter()
	require.NoError(t, writer.WriteRegistry(registryPath, registryDirPath, "registry.json.tmp", testData))

	content, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	assert.Equal(t, testData, content)
}

func TestNonAtomicRegistryWriter(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")
	registryDirPath := tmpDir
	registryTmpFile := "registry.json.tmp"
	testData := []byte(`{"test": "data"}`)

	// Create non-atomic registry writer
	writer := NewNonAtomicRegistryWriter()

	err := writer.WriteRegistry(registryPath, registryDirPath, registryTmpFile, testData)
	require.NoError(t, err)

	// Verify file exists and has correct content
	content, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	assert.Equal(t, testData, content)
}
