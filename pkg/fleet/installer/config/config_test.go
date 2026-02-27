// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v2"
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

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	// Check file content
	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "baz", updatedMap["foo"])

	// Check file permissions (skip on Windows as POSIX permissions don't apply)
	if runtime.GOOS != "windows" {
		stat, err := os.Stat(filePath)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0640), stat.Mode().Perm())
	}
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

func TestEnsureDir(t *testing.T) {
	t.Run("simple directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		root, err := os.OpenRoot(tmpDir)
		assert.NoError(t, err)
		defer root.Close()

		err = ensureDir(root, filepath.Join("subdir", "file.txt"))
		assert.NoError(t, err)

		// Verify directory was created
		_, err = os.Stat(filepath.Join(tmpDir, "subdir"))
		assert.NoError(t, err)
	})

	t.Run("nested directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		root, err := os.OpenRoot(tmpDir)
		assert.NoError(t, err)
		defer root.Close()

		err = ensureDir(root, "/"+filepath.Join("level1", "level2", "level3", "file.txt"))
		assert.NoError(t, err)

		// Verify all directories were created
		_, err = os.Stat(filepath.Join(tmpDir, "level1", "level2", "level3"))
		assert.NoError(t, err)
	})

	t.Run("directory already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		existingDir := filepath.Join(tmpDir, "existing")
		err := os.MkdirAll(existingDir, 0755)
		assert.NoError(t, err)

		root, err := os.OpenRoot(tmpDir)
		assert.NoError(t, err)
		defer root.Close()

		// Should not error when directory already exists
		err = ensureDir(root, "/"+filepath.Join("existing", "file.txt"))
		assert.NoError(t, err)
	})

	t.Run("file in current directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		root, err := os.OpenRoot(tmpDir)
		assert.NoError(t, err)
		defer root.Close()

		// No directory to create, should return immediately
		err = ensureDir(root, "file.txt")
		assert.NoError(t, err)
	})

	t.Run("partially existing path", func(t *testing.T) {
		tmpDir := t.TempDir()
		partialDir := filepath.Join(tmpDir, "existing")
		err := os.MkdirAll(partialDir, 0755)
		assert.NoError(t, err)

		root, err := os.OpenRoot(tmpDir)
		assert.NoError(t, err)
		defer root.Close()

		// Create new subdirectories under existing one
		err = ensureDir(root, filepath.Join("existing", "new1", "new2", "file.txt"))
		assert.NoError(t, err)

		// Verify all directories exist
		_, err = os.Stat(filepath.Join(tmpDir, "existing", "new1", "new2"))
		assert.NoError(t, err)
	})

	t.Run("path traversal", func(t *testing.T) {
		tmpDir := t.TempDir()
		root, err := os.OpenRoot(tmpDir)
		assert.NoError(t, err)
		defer root.Close()

		err = ensureDir(root, "/"+filepath.Join("..", "existing", "file.txt"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path escape")
	})
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

	op, err := buildOperationsFromLegacyConfigFile(filepath.Join(managedDir, "datadog.yaml"), tmpDir, legacyPathPrefix)
	assert.NoError(t, err)

	// Check merge patch operation
	assert.Equal(t, FileOperationMergePatch, op.FileOperationType)
	assert.Equal(t, "/datadog.yaml", op.FilePath)
	assert.Equal(t, string(legacyConfig), string(op.Patch))
}

func TestBuildOperationsFromLegacyInstaller(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, filepath.Join("managed", "datadog-agent", "aaa-bbb-ccc"))
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)
	err = os.Symlink(managedDir, filepath.Join(tmpDir, legacyPathPrefix))
	assert.NoError(t, err)

	// Create legacy config files
	err = os.WriteFile(filepath.Join(managedDir, "datadog.yaml"), []byte("foo: legacy\n"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(managedDir, "security-agent.yaml"), []byte("enabled: true\n"), 0644)
	assert.NoError(t, err)

	ops := buildOperationsFromLegacyInstaller(tmpDir)

	// Should have 3 operations: 2 merge patches + 1 delete all
	assert.Len(t, ops, 3)

	// Check that we have operations for both files
	filePaths := make(map[string]bool)
	for _, op := range ops {
		filePaths[strings.TrimPrefix(strings.TrimPrefix(op.FilePath, "/"), "\\")] = true
	}
	assert.True(t, filePaths["datadog.yaml"])
	assert.True(t, filePaths["security-agent.yaml"])
	assert.Equal(t, FileOperationDeleteAll, ops[0].FileOperationType)
	assert.Equal(t, "/managed", ops[0].FilePath)
}

func TestBuildOperationsFromLegacyConfigFileKeepApplicationMonitoring(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, filepath.Join("managed", "datadog-agent", "aaa-bbb-ccc"))
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)
	err = os.Symlink(managedDir, filepath.Join(tmpDir, legacyPathPrefix))
	assert.NoError(t, err)

	legacyConfig := []byte("{\"bar\":123,\"foo\":\"legacy_value\"}")
	err = os.WriteFile(filepath.Join(managedDir, "application_monitoring.yaml"), legacyConfig, 0644)
	assert.NoError(t, err)

	ops := buildOperationsFromLegacyInstaller(tmpDir)
	assert.Len(t, ops, 2)

	assert.Equal(t, FileOperationDeleteAll, ops[0].FileOperationType)
	assert.Equal(t, "/managed", ops[0].FilePath)
	assert.Equal(t, FileOperationMergePatch, ops[1].FileOperationType)
	assert.Equal(t, "/"+filepath.Join("managed", "datadog-agent", "stable", "application_monitoring.yaml"), ops[1].FilePath)
	assert.Equal(t, string(legacyConfig), string(ops[1].Patch))
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
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

	err = op.apply(context.Background(), root)
	assert.Error(t, err)
}

func TestConfig_SimpleStartPromote(t *testing.T) {
	stableDir := t.TempDir()     // This acts as the 'Stable' config directory
	experimentDir := t.TempDir() // This acts as the 'Experiment' config directory

	// Place a simple base config in the stable directory
	baseConfigPath := filepath.Join(stableDir, "datadog.yaml")
	baseContent := []byte("log_level: info\n")
	assert.NoError(t, os.WriteFile(baseConfigPath, baseContent, 0644))

	dirs := &Directories{
		StablePath:     stableDir,
		ExperimentPath: experimentDir,
	}

	// Start experiment: create a new config in experiment
	err := dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "exp-001",
		FileOperations: []FileOperation{
			{
				FileOperationType: FileOperationMergePatch,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(`{"log_level": "debug"}`),
			},
			{
				FileOperationType: FileOperationMergePatch,
				FilePath:          "/conf.d/mycheck.d/config.yaml",
				Patch:             []byte(`{"integration_setting": true}`),
			},
		},
	})
	assert.NoError(t, err)

	// Promote experiment
	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)

	// After promote: stable has the new content
	finalContent, err := os.ReadFile(filepath.Join(stableDir, "datadog.yaml"))
	assert.NoError(t, err)
	assert.Equal(t, string(finalContent), "log_level: debug\n")
	finalConfDContent, err := os.ReadFile(filepath.Join(stableDir, "conf.d", "mycheck.d", "config.yaml"))
	assert.NoError(t, err)
	assert.Equal(t, string(finalConfDContent), "integration_setting: true\n")
}

func TestConfig_SimpleStartStop(t *testing.T) {
	stableDir := t.TempDir()
	experimentDir := t.TempDir()

	// Place a simple base config in the stable directory
	baseConfigPath := filepath.Join(stableDir, "datadog.yaml")
	baseContent := []byte("log_level: warn\n")
	assert.NoError(t, os.WriteFile(baseConfigPath, baseContent, 0644))

	dirs := &Directories{
		StablePath:     stableDir,
		ExperimentPath: experimentDir,
	}

	// Start experiment: patch config
	err := dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "exp-002",
		FileOperations: []FileOperation{
			{
				FileOperationType: FileOperationMergePatch,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(`{"log_level": "debug"}`),
			},
			{
				FileOperationType: FileOperationMergePatch,
				FilePath:          "/conf.d/mycheck.d/config.yaml",
				Patch:             []byte(`{"integration_setting": true}`),
			},
		},
	})
	assert.NoError(t, err)

	// Stop experiment (rollback to stable)
	err = dirs.RemoveExperiment(context.Background())
	assert.NoError(t, err)

	// Stable should remain unchanged
	finalContent, err := os.ReadFile(filepath.Join(stableDir, "datadog.yaml"))
	assert.NoError(t, err)
	assert.Equal(t, string(baseContent), string(finalContent))
	_, err = os.ReadFile(filepath.Join(stableDir, "conf.d", "mycheck.d", "config.yaml"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestOperationApply_MultipleAPIKeysWithDuplicateKeys(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")

	// Craft YAML content with two api_key entries (duplicate), which is allowed in yaml.v2 loader,
	// but not in yaml.v3 (v3 will only keep the last one by default, but v2 merges them as a map).
	// For this test, we intentionally want to verify parsing and overwriting with yaml.v2.

	// Note: When there are duplicate keys, yaml.v2 will use the last occurrence by default.
	// For the purpose of this test, we check that the resulting config can be updated and parsed safely.

	originalYAML := []byte(`
api_key: "KEY_1"
some_other: value
api_key: "KEY_2"
`)
	assert.NoError(t, os.WriteFile(filePath, originalYAML, 0644))

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// Patch: Replace api_key to a new value (should replace the effective value, regardless of duplicates in original)
	patchJSON := `[{"op": "replace", "path": "/api_key", "value": "NEW_KEY"}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	// Check file content, should now have api_key: NEW_KEY
	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)

	var updatedMap map[string]interface{}
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)

	// The map should take only the last value (or now, our patched one)
	assert.Equal(t, "NEW_KEY", updatedMap["api_key"])
	assert.Equal(t, "value", updatedMap["some_other"])
}

func TestOperationApply_ApplicationMonitoringPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "application_monitoring.yaml")
	orig := map[string]any{"enabled": true}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0600)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// Patch the file
	patchJSON := `[{"op": "replace", "path": "/enabled", "value": false}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/application_monitoring.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	// Check file permissions - should be world-readable (0644)
	// Skip on Windows as POSIX permissions don't apply
	if runtime.GOOS != "windows" {
		stat, err := os.Stat(filePath)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), stat.Mode().Perm(), "application_monitoring.yaml should be world-readable (0644)")
	}
}

func TestOperationApply_NestedMaps(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")

	// Create a YAML file with nested maps
	orig := map[string]any{
		"foo": "bar",
		"nested": map[string]any{
			"key": "value",
		},
	}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// Patch: update nested value
	patchJSON := `[{"op": "replace", "path": "/nested/key", "value": "newvalue"}]`
	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(patchJSON),
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	// Check file content
	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)

	nested := updatedMap["nested"].(map[interface{}]interface{})
	assert.Equal(t, "newvalue", nested["key"])
	assert.Equal(t, "bar", updatedMap["foo"])
}

func TestReplaceSecrets(t *testing.T) {
	t.Run("successfully replace secrets", func(t *testing.T) {
		ops := Operations{
			DeploymentID: "test-config",
			FileOperations: []FileOperation{
				{
					Patch: []byte(`api_key: SEC[apikey]`),
				},
				{
					Patch: []byte(`app_key: SEC[appkey]`),
				},
			},
		}

		err := ReplaceSecrets(&ops, map[string]string{
			"apikey": "my-api-key",
			"appkey": "my-app-key",
		})

		assert.NoError(t, err)
		assert.Equal(t, "api_key: my-api-key", string(ops.FileOperations[0].Patch))
		assert.Equal(t, "app_key: my-app-key", string(ops.FileOperations[1].Patch))
	})

	t.Run("no secrets to replace", func(t *testing.T) {
		ops := Operations{
			DeploymentID: "test-config",
			FileOperations: []FileOperation{
				{
					Patch: []byte(`{"log_level": "debug"}`),
				},
			},
		}

		err := ReplaceSecrets(&ops, map[string]string{})

		assert.NoError(t, err)
		assert.Equal(t, `{"log_level": "debug"}`, string(ops.FileOperations[0].Patch))
	})

	t.Run("unreplaced secret returns error", func(t *testing.T) {
		ops := Operations{
			DeploymentID: "test-config",
			FileOperations: []FileOperation{
				{
					Patch: []byte(`api_key: SEC[apikey]`),
				},
			},
		}

		err := ReplaceSecrets(&ops, map[string]string{
			"wrong-key": "some-value",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "secrets are not fully replaced")
	})
}
