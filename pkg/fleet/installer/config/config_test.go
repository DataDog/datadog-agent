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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
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

	err = op.apply(root, tmpDir)
	assert.Error(t, err)
}

func writeConfigV2(t *testing.T, v2Dir string) {
	managedDir := filepath.Join(v2Dir, "managed", "datadog-agent")
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(filepath.Join(managedDir, "v2"), 0755)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(filepath.Join(managedDir, "v2", "datadog.yaml"), []byte("log_level: debug\n"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(managedDir, "v2", "application_monitoring.yaml"), []byte("enabled: true\n"), 0644))
	assert.NoError(t, os.MkdirAll(filepath.Join(managedDir, "v2", "conf.d", "mycheck.d"), 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(managedDir, "v2", "conf.d", "mycheck.d", "config.yaml"), []byte("foo: bar\n"), 0644))

	// Create the stable and experiment symlinks
	err = os.Symlink(filepath.Join(managedDir, "v2"), filepath.Join(managedDir, "stable"))
	assert.NoError(t, err)
	err = os.Symlink(filepath.Join(managedDir, "v2"), filepath.Join(managedDir, "experiment"))
	assert.NoError(t, err)
}

func assertConfigV2(t *testing.T, v2Dir *string) {
	// /managed/datadog-agent/stable -> /etc/datadog-agent/managed/datadog-agent/v2
	// /managed/datadog-agent/experiment -> /etc/datadog-agent/managed/datadog-agent/v2
	// /managed/datadog-agent/v2/
	//     datadog.yaml
	//     application_monitoring.yaml
	//     conf.d/mycheck.d/config.yaml
	managedDir := filepath.Join(*v2Dir, "managed", "datadog-agent")
	info, err := os.Lstat(filepath.Join(managedDir, "v2"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeDir != 0)
	info, err = os.Stat(filepath.Join(managedDir, "v2", "datadog.yaml"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink == 0)
	info, err = os.Lstat(filepath.Join(managedDir, "v2", "application_monitoring.yaml"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink == 0)
	info, err = os.Lstat(filepath.Join(managedDir, "v2", "conf.d", "mycheck.d", "config.yaml"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink == 0)

	info, err = os.Lstat(filepath.Join(managedDir, "stable"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0)
	info, err = os.Lstat(filepath.Join(managedDir, "experiment"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0)

	// v2Dir/conf.d/mychecks.d/config.yaml does not exists
	_, err = os.Lstat(filepath.Join(*v2Dir, "conf.d", "mycheck.d", "config.yaml"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func assertConfigV3(t *testing.T, v3Dir *string) {
	// Check the content of the v3 directory
	// /managed/datadog-agent/stable
	//     application_monitoring.yaml
	// No more symlinks
	managedDir := filepath.Join(*v3Dir, "managed", "datadog-agent")
	_, err := os.Stat(filepath.Join(managedDir, "experiment"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(managedDir, "v2"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	stableInfo, err := os.Lstat(filepath.Join(managedDir, "stable"))
	assert.NoError(t, err)
	assert.True(t, stableInfo.Mode()&os.ModeSymlink == 0)

	// Check the files are not here anymore, except application_monitoring.yaml
	_, err = os.Lstat(filepath.Join(managedDir, "stable", "datadog.yaml"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
	info, err := os.Lstat(filepath.Join(managedDir, "stable", "application_monitoring.yaml"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink == 0)
	_, err = os.Lstat(filepath.Join(managedDir, "stable", "conf.d", "mycheck.d", "config.yaml"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	// v3Dir/conf.d/mychecks.d/config.yaml exists
	_, err = os.Lstat(filepath.Join(*v3Dir, "conf.d", "mycheck.d", "config.yaml"))
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink == 0)
}

func assertDeploymentID(t *testing.T, dirs *Directories, stableDeploymentID string, experimentDeploymentID string) {
	state, err := dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, stableDeploymentID, state.StableDeploymentID)
	assert.Equal(t, experimentDeploymentID, state.ExperimentDeploymentID)
}

func TestConfigV2ToV3(t *testing.T) {
	stableTmpDir := t.TempDir()
	managedDir := filepath.Join(stableTmpDir, "managed", "datadog-agent")
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)

	// Create a v2 tree
	writeConfigV2(t, stableTmpDir)
	assertConfigV2(t, &stableTmpDir) // Make sure it's correct

	// Convert v2 to v3
	newDir := t.TempDir()
	dirs := &Directories{
		StablePath:     stableTmpDir,
		ExperimentPath: newDir,
	}

	assertDeploymentID(t, dirs, "", "")

	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-456",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)

	experimentDir := &newDir
	stableDir := &stableTmpDir
	if runtime.GOOS == "windows" {
		experimentDir = &stableTmpDir
		stableDir = &newDir
	}

	assertDeploymentID(t, dirs, "", "experiment-456")

	assertConfigV2(t, stableDir) // Make sure nothing changed
	assertConfigV3(t, experimentDir)

	// Promote
	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV3(t, stableDir) // Make sure it changed

	assertDeploymentID(t, dirs, "experiment-456", "")
}

func TestConfigV2Rollback(t *testing.T) {
	stableTmpDir := t.TempDir()
	managedDir := filepath.Join(stableTmpDir, "managed", "datadog-agent")
	err := os.MkdirAll(managedDir, 0755)
	assert.NoError(t, err)

	// Create a v2 tree
	writeConfigV2(t, stableTmpDir)
	assertConfigV2(t, &stableTmpDir) // Make sure it's correct

	// Convert v2 to v3
	newDir := t.TempDir()
	dirs := &Directories{
		StablePath:     stableTmpDir,
		ExperimentPath: newDir,
	}

	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-456",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)

	experimentDir := &newDir
	stableDir := &stableTmpDir
	if runtime.GOOS == "windows" {
		experimentDir = &stableTmpDir
		stableDir = &newDir
	}

	assertConfigV2(t, stableDir) // Make sure nothing changed
	assertConfigV3(t, experimentDir)

	assertDeploymentID(t, dirs, "", "experiment-456")

	// Rollback
	err = dirs.RemoveExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV2(t, stableDir) // Make sure it's still v2

	// Write again
	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-789",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)
	assertConfigV2(t, stableDir) // Make sure it's still v2
	assertConfigV3(t, experimentDir)

	// Promote
	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV3(t, stableDir) // Make sure it changed

	assertDeploymentID(t, dirs, "experiment-789", "")
}
