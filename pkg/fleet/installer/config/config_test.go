// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"os"
	"path/filepath"
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

func TestDirectories_GetState(t *testing.T) {
	tmpDir := t.TempDir()
	stablePath := filepath.Join(tmpDir, "stable")
	experimentPath := filepath.Join(tmpDir, "experiment")
	
	err := os.MkdirAll(stablePath, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(experimentPath, 0755)
	assert.NoError(t, err)

	dirs := &Directories{
		StablePath:     stablePath,
		ExperimentPath: experimentPath,
	}

	// Test with no deployment IDs
	state, err := dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "", state.StableDeploymentID)
	assert.Equal(t, "", state.ExperimentDeploymentID)

	// Test with stable deployment ID only
	err = os.WriteFile(filepath.Join(stablePath, deploymentIDFile), []byte("stable-123"), 0644)
	assert.NoError(t, err)

	state, err = dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "stable-123", state.StableDeploymentID)
	assert.Equal(t, "", state.ExperimentDeploymentID)

	// Test with both deployment IDs
	err = os.WriteFile(filepath.Join(experimentPath, deploymentIDFile), []byte("experiment-456"), 0644)
	assert.NoError(t, err)

	state, err = dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "stable-123", state.StableDeploymentID)
	assert.Equal(t, "experiment-456", state.ExperimentDeploymentID)

	// Test with symlinked experiment (should clear experiment deployment ID)
	err = os.Remove(filepath.Join(experimentPath, deploymentIDFile))
	assert.NoError(t, err)
	err = os.Symlink(filepath.Join(stablePath, deploymentIDFile), filepath.Join(experimentPath, deploymentIDFile))
	assert.NoError(t, err)

	state, err = dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "stable-123", state.StableDeploymentID)
	assert.Equal(t, "", state.ExperimentDeploymentID)
}

func TestDirectories_WriteExperiment(t *testing.T) {
	tmpDir := t.TempDir()
	stablePath := filepath.Join(tmpDir, "stable")
	experimentPath := filepath.Join(tmpDir, "experiment")
	
	err := os.MkdirAll(stablePath, 0755)
	assert.NoError(t, err)

	// Create initial stable config
	err = os.WriteFile(filepath.Join(stablePath, "datadog.yaml"), []byte("foo: bar\n"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(stablePath, deploymentIDFile), []byte("stable-123"), 0644)
	assert.NoError(t, err)

	dirs := &Directories{
		StablePath:     stablePath,
		ExperimentPath: experimentPath,
	}

	// Create operations to modify the config
	patchJSON := `[{"op": "replace", "path": "/foo", "value": "baz"}]`
	operations := Operations{
		DeploymentID: "experiment-456",
		FileOperations: []FileOperation{
			{
				FileOperationType: FileOperationPatch,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(patchJSON),
			},
		},
	}

	err = dirs.WriteExperiment(context.Background(), operations)
	assert.NoError(t, err)

	// Check that experiment directory was created
	_, err = os.Stat(experimentPath)
	assert.NoError(t, err)

	// Check that the config was modified
	experimentConfig, err := os.ReadFile(filepath.Join(experimentPath, "datadog.yaml"))
	assert.NoError(t, err)
	var config map[string]any
	err = yaml.Unmarshal(experimentConfig, &config)
	assert.NoError(t, err)
	assert.Equal(t, "baz", config["foo"])

	// Check that deployment ID was written
	deploymentID, err := os.ReadFile(filepath.Join(experimentPath, deploymentIDFile))
	assert.NoError(t, err)
	assert.Equal(t, "experiment-456", string(deploymentID))
}

func TestDeleteConfigNameAllowed(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{"datadog.yaml", "/managed/stable/datadog.yaml", true},
		{"security-agent.yaml", "/managed/stable/security-agent.yaml", true},
		{"system-probe.yaml", "/managed/stable/system-probe.yaml", true},
		{"application_monitoring.yaml", "/managed/stable/application_monitoring.yaml", true},
		{"not in managed/stable", "/datadog.yaml", false},
		{"disallowed file", "/managed/stable/notallowed.yaml", false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deleteConfigNameAllowed(tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildOperationsFromLegacyConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, "managed", "stable")
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)

	// Create a legacy config file
	legacyConfig := []byte("foo: legacy_value\nbar: 123\n")
	err = os.WriteFile(filepath.Join(managedDir, "datadog.yaml"), legacyConfig, 0644)
	assert.NoError(t, err)

	ops, err := buildOperationsFromLegacyConfigFile(tmpDir, "/datadog.yaml")
	assert.NoError(t, err)
	assert.Len(t, ops, 2)

	// Check merge patch operation
	assert.Equal(t, FileOperationMergePatch, ops[0].FileOperationType)
	assert.Equal(t, "/datadog.yaml", ops[0].FilePath)
	assert.Equal(t, legacyConfig, []byte(ops[0].Patch))

	// Check delete operation
	assert.Equal(t, FileOperationDelete, ops[1].FileOperationType)
	assert.Equal(t, "/managed/stable/datadog.yaml", ops[1].FilePath)
}

func TestBuildOperationsFromLegacyInstaller(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, "managed", "stable")
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
		filePaths[op.FilePath] = true
	}
	assert.True(t, filePaths["/datadog.yaml"])
	assert.True(t, filePaths["/security-agent.yaml"])
	assert.True(t, filePaths["/managed/stable/datadog.yaml"])
	assert.True(t, filePaths["/managed/stable/security-agent.yaml"])
}
