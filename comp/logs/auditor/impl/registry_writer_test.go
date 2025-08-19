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
