// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestOperationApply_Patch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	orig := map[string]any{"foo": "bar"}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// Patch: change foo to baz
	patchJSON := `[{"op": "replace", "path": "/foo", "value": "baz"}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(root)
	assert.NoError(t, err)

	// Check file content
	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "baz", updatedMap["foo"])
}

func TestOperationApply_MergePatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	orig := map[string]any{"foo": "bar", "bar": "baz"}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// MergePatch: remove bar, change foo to qux
	mergePatch := `{"foo": "qux", "bar": null}`
	op := &FileOperation{
		FileOperationType: FileOperationMergePatch,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(mergePatch),
	}

	err = op.apply(root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "qux", updatedMap["foo"])
	_, exists := updatedMap["bar"]
	assert.False(t, exists)
}

func TestOperationApply_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	err := os.WriteFile(filePath, []byte("foo: bar"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationDelete,
		FilePath:          "/datadog.yaml",
	}

	err = op.apply(root)
	assert.NoError(t, err)
	_, err = os.Stat(filePath)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestOperationApply_EmptyYAMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	err := os.WriteFile(filePath, []byte(""), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	patchJSON := `[{"op": "add", "path": "/foo", "value": "bar"}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(root)
	assert.NoError(t, err)

	// Check that the file now contains the patched value
	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "bar", updatedMap["foo"])
}

func TestOperationApply_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Do not create the file

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	patchJSON := `[{"op": "add", "path": "/foo", "value": "bar"}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(root)
	assert.NoError(t, err)

	filePath := filepath.Join(tmpDir, "datadog.yaml")
	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "bar", updatedMap["foo"])
}

func TestOperationApply_DisallowedFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notallowed.yaml")
	err := os.WriteFile(filePath, []byte("foo: bar"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	patchJSON := `[{"op": "replace", "path": "/foo", "value": "baz"}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/notallowed.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestOperationApply_NestedConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "conf.d", "mycheck.d")
	err := os.MkdirAll(nestedDir, 0755)
	assert.NoError(t, err)

	filePath := filepath.Join(nestedDir, "config.yaml")
	// Create an initial config file
	initialContent := []byte("foo: oldval\nbar: 1\n")
	err = os.WriteFile(filePath, initialContent, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	patchJSON := `[{"op": "replace", "path": "/foo", "value": "newval"}, {"op": "add", "path": "/baz", "value": 42}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/conf.d/mycheck.d/config.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "newval", updatedMap["foo"])
	assert.Equal(t, 1, updatedMap["bar"])
	assert.Equal(t, 42, updatedMap["baz"])
}

func TestBuildOperationsFromLegacyConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, legacyPathPrefix)
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)

	// Create a legacy config file
	legacyConfig := []byte("{\"bar\":123,\"foo\":\"legacy_value\"}")
	err = os.WriteFile(filepath.Join(managedDir, "datadog.yaml"), legacyConfig, 0644)
	assert.NoError(t, err)

	ops, err := buildOperationsFromLegacyConfigFile(filepath.Join(managedDir, "datadog.yaml"), tmpDir, legacyPathPrefix)
	assert.NoError(t, err)
	assert.Len(t, ops, 2)

	// Check merge patch operation
	assert.Equal(t, FileOperationMergePatch, ops[0].FileOperationType)
	assert.Equal(t, "/datadog.yaml", ops[0].FilePath)
	assert.Equal(t, string(legacyConfig), string(ops[0].Patch))

	// Check delete operation
	assert.Equal(t, FileOperationDelete, ops[1].FileOperationType)
	assert.Equal(t, filepath.Join(legacyPathPrefix, "datadog.yaml"), strings.TrimPrefix(strings.TrimPrefix(ops[1].FilePath, "/"), "\\"))
}

func TestBuildOperationsFromLegacyInstaller(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, legacyPathPrefix)
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)

	// Create legacy config files
	err = os.WriteFile(filepath.Join(managedDir, "datadog.yaml"), []byte("foo: legacy\n"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(managedDir, "security-agent.yaml"), []byte("enabled: true\n"), 0644)
	assert.NoError(t, err)

	ops := buildOperationsFromLegacyInstaller(tmpDir)

	// Should have 4 operations: 2 merge patches + 2 deletes
	assert.Len(t, ops, 4)

	// Check that we have operations for both files
	filePaths := make(map[string]bool)
	for _, op := range ops {
		filePaths[strings.TrimPrefix(strings.TrimPrefix(op.FilePath, "/"), "\\")] = true
	}
	assert.True(t, filePaths["datadog.yaml"])
	assert.True(t, filePaths["security-agent.yaml"])
	assert.True(t, filePaths[filepath.Join("managed", "datadog-agent", "stable", "datadog.yaml")])
	assert.True(t, filePaths[filepath.Join("managed", "datadog-agent", "stable", "security-agent.yaml")])
}

func TestBuildOperationsFromLegacyConfigFileKeepApplicationMonitoring(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, legacyPathPrefix)
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)

	// Create a legacy config file
	legacyConfig := []byte("{\"bar\":123,\"foo\":\"legacy_value\"}")
	err = os.WriteFile(filepath.Join(managedDir, "application_monitoring.yaml"), legacyConfig, 0644)
	assert.NoError(t, err)

	ops := buildOperationsFromLegacyInstaller(tmpDir)
	assert.Len(t, ops, 0)
}

func TestOperationApply_Copy(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFilePath := filepath.Join(tmpDir, "datadog.yaml")
	destFilePath := filepath.Join(tmpDir, "security-agent.yaml")

	// Create source file
	sourceContent := []byte("foo: bar\nbaz: qux\n")
	err := os.WriteFile(sourceFilePath, sourceContent, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationCopy,
		FilePath:          "/datadog.yaml",
		DestinationPath:   "/security-agent.yaml",
	}

	err = op.apply(root)
	assert.NoError(t, err)

	// Check that source file still exists
	_, err = os.Stat(sourceFilePath)
	assert.NoError(t, err)

	// Check that destination file was created with correct content
	destContent, err := os.ReadFile(destFilePath)
	assert.NoError(t, err)
	assert.Equal(t, sourceContent, destContent)
}

func TestOperationApply_Move(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFilePath := filepath.Join(tmpDir, "datadog.yaml")
	destFilePath := filepath.Join(tmpDir, "otel-config.yaml")

	// Create source file
	sourceContent := []byte("foo: bar\nbaz: qux\n")
	err := os.WriteFile(sourceFilePath, sourceContent, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationMove,
		FilePath:          "/datadog.yaml",
		DestinationPath:   "/otel-config.yaml",
	}

	err = op.apply(root)
	assert.NoError(t, err)

	// Check that source file no longer exists
	_, err = os.Stat(sourceFilePath)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	// Check that destination file was created with correct content
	destContent, err := os.ReadFile(destFilePath)
	assert.NoError(t, err)
	assert.Equal(t, sourceContent, destContent)
}

func TestOperationApply_CopyWithNestedDestination(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFilePath := filepath.Join(tmpDir, "datadog.yaml")
	destDir := filepath.Join(tmpDir, "conf.d", "mycheck.d")
	destFilePath := filepath.Join(destDir, "config.yaml")

	// Create source file
	sourceContent := []byte("foo: bar\nbaz: qux\n")
	err := os.WriteFile(sourceFilePath, sourceContent, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationCopy,
		FilePath:          "/datadog.yaml",
		DestinationPath:   "/conf.d/mycheck.d/config.yaml",
	}

	err = op.apply(root)
	assert.NoError(t, err)

	// Check that nested directories were created
	_, err = os.Stat(destDir)
	assert.NoError(t, err)

	// Check that destination file was created with correct content
	destContent, err := os.ReadFile(destFilePath)
	assert.NoError(t, err)
	assert.Equal(t, sourceContent, destContent)
}

func TestOperationApply_MoveWithNestedDestination(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFilePath := filepath.Join(tmpDir, "system-probe.yaml")
	destDir := filepath.Join(tmpDir, "conf.d", "mycheck.d")
	destFilePath := filepath.Join(destDir, "config.yaml")

	// Create source file
	sourceContent := []byte("foo: bar\nbaz: qux\n")
	err := os.WriteFile(sourceFilePath, sourceContent, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationMove,
		FilePath:          "/system-probe.yaml",
		DestinationPath:   "/conf.d/mycheck.d/config.yaml",
	}

	err = op.apply(root)
	assert.NoError(t, err)

	// Check that nested directories were created
	_, err = os.Stat(destDir)
	assert.NoError(t, err)

	// Check that source file no longer exists
	_, err = os.Stat(sourceFilePath)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	// Check that destination file was created with correct content
	destContent, err := os.ReadFile(destFilePath)
	assert.NoError(t, err)
	assert.Equal(t, sourceContent, destContent)
}

func TestOperationApply_CopyMissingSource(t *testing.T) {
	tmpDir := t.TempDir()

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationCopy,
		FilePath:          "/datadog.yaml",
		DestinationPath:   "/security-agent.yaml",
	}

	err = op.apply(root)
	assert.Error(t, err)
}

func TestOperationApply_MoveMissingSource(t *testing.T) {
	tmpDir := t.TempDir()

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationMove,
		FilePath:          "/datadog.yaml",
		DestinationPath:   "/otel-config.yaml",
	}

	err = op.apply(root)
	assert.Error(t, err)
}
