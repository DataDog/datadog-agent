// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package auditorimpl

import (
	"os"
	"path/filepath"
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

	// Create atomic registry writer
	writer := NewAtomicRegistryWriter()

	// Test writing registry
	err := writer.WriteRegistry(registryPath, registryDirPath, registryTmpFile, testData)
	require.NoError(t, err)

	// Verify file exists and has correct content
	content, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	assert.Equal(t, testData, content)

	// Verify file permissions
	info, err := os.Stat(registryPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
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

	// Test writing registry
	err := writer.WriteRegistry(registryPath, registryDirPath, registryTmpFile, testData)
	require.NoError(t, err)

	// Verify file exists and has correct content
	content, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	assert.Equal(t, testData, content)

	// Verify file permissions
	info, err := os.Stat(registryPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestRegistryWriterErrorCases(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	registryDirPath := tmpDir
	registryTmpFile := "registry.json.tmp"
	testData := []byte(`{"test": "data"}`)

	// Test with non-existent directory
	writer := NewAtomicRegistryWriter()
	err := writer.WriteRegistry("/nonexistent/path/registry.json", registryDirPath, registryTmpFile, testData)
	assert.Error(t, err)

	// Test with read-only directory
	readOnlyDir := t.TempDir()
	err = os.Chmod(readOnlyDir, 0444)
	require.NoError(t, err)
	err = writer.WriteRegistry(filepath.Join(readOnlyDir, "registry.json"), readOnlyDir, registryTmpFile, testData)
	assert.Error(t, err)
}
