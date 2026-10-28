// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2" // not v3 due to lenient duplicate mapping-key handling
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

func TestOperationApply_MalformedYAMLNamesFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "application_monitoring.yaml")
	// A root-level mapping followed by a root-level sequence item is invalid YAML
	// and makes go-yaml return "did not find expected key" with only a line number.
	malformed := "apm_configuration_default:\n  DD_LOGS_INJECTION: true\n\n- selectors:\n  - origin: language\n"
	err := os.WriteFile(filePath, []byte(malformed), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationMergePatch,
		FilePath:          "/application_monitoring.yaml",
		Patch:             []byte(`{"config_id": "abc"}`),
	}

	err = op.apply(context.Background(), root)
	assert.Error(t, err)
	// The error must name the offending file so customers can pinpoint it...
	assert.ErrorContains(t, err, `"/application_monitoring.yaml"`)
	// ...while preserving the underlying parser error (line number + reason).
	assert.ErrorContains(t, err, "did not find expected key")
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

func TestOperationApply_SNMPCredentialsFile(t *testing.T) {
	tmpDir := t.TempDir()

	root, err := os.OpenRoot(tmpDir)
	require.NoError(t, err)
	defer root.Close()

	// The credentials directory does not exist yet: writing the first file under it
	// relies on the operation creating it.
	op := &FileOperation{
		FileOperationType: FileOperationMergePatch,
		FilePath:          "/conf.d/snmp.d/credentials/snmp_credentials.yaml",
		Patch:             []byte(`{"foo": "bar"}`),
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	written, err := os.ReadFile(filepath.Join(tmpDir, "conf.d", "snmp.d", "credentials", "snmp_credentials.yaml"))
	require.NoError(t, err)
	var writtenMap map[string]any
	err = yaml.Unmarshal(written, &writtenMap)
	assert.NoError(t, err)
	assert.Equal(t, "bar", writtenMap["foo"])
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

// otelConfigSeed is a minimal but realistic DDOT collector config (a subset of
// cmd/otel-agent/dist/otel-config.yaml) used to exercise config experiments against the deeply
// nested /otel-config.yaml file.
const otelConfigSeed = `receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
exporters:
  debug:
    verbosity: detailed
processors:
  infraattributes:
    cardinality: 2
service:
  pipelines:
    traces:
      receivers:
      - otlp
      processors:
      - infraattributes
      exporters:
      - debug
`

func TestConfig_OTelConfigStartPromote(t *testing.T) {
	stableDir := t.TempDir()
	experimentDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(stableDir, "otel-config.yaml"), []byte(otelConfigSeed), 0640))

	dirs := &Directories{StablePath: stableDir, ExperimentPath: experimentDir}

	err := dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "otel-exp-001",
		FileOperations: []FileOperation{
			// Deep-merge a nested collector value without clobbering sibling keys.
			{FileOperationType: FileOperationMergePatch, FilePath: "/otel-config.yaml", Patch: []byte(`{"processors":{"infraattributes":{"cardinality":1}}}`)},
			// jq transform on the same nested config.
			{FileOperationType: FileOperationJQ, FilePath: "/otel-config.yaml", Transform: `.exporters.debug.verbosity = "normal"`},
		},
	})
	assert.NoError(t, err)

	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)

	// After promote the stable config reflects the merge-patch + jq transform with sibling keys preserved.
	// (Asserted on the stable dir so the test is OS-agnostic: nix applies the ops in the experiment
	// dir and swaps on promote, Windows applies them in place to the stable dir.)
	stableContent, err := os.ReadFile(filepath.Join(stableDir, "otel-config.yaml"))
	assert.NoError(t, err)
	assertOTelExperimentConfig(t, string(stableContent))
}

func TestConfig_OTelConfigStartStop(t *testing.T) {
	stableDir := t.TempDir()
	experimentDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(stableDir, "otel-config.yaml"), []byte(otelConfigSeed), 0640))
	original, err := os.ReadFile(filepath.Join(stableDir, "otel-config.yaml"))
	assert.NoError(t, err)

	dirs := &Directories{StablePath: stableDir, ExperimentPath: experimentDir}

	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "otel-exp-002",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/otel-config.yaml", Patch: []byte(`{"processors":{"infraattributes":{"cardinality":1}}}`)},
		},
	})
	assert.NoError(t, err)

	// Rolling back restores the stable config to its original content (OS-agnostic).
	err = dirs.RemoveExperiment(context.Background())
	assert.NoError(t, err)

	stableContent, err := os.ReadFile(filepath.Join(stableDir, "otel-config.yaml"))
	assert.NoError(t, err)
	assert.Equal(t, string(original), string(stableContent))
}

// assertOTelExperimentConfig checks that both the merge-patch (cardinality 2 -> 1) and the jq
// transform (verbosity detailed -> normal) were applied while the untouched sibling sections
// (receivers, service pipelines) survived the deep merge.
func assertOTelExperimentConfig(t *testing.T, content string) {
	t.Helper()
	assert.Contains(t, content, "cardinality: 1")
	assert.NotContains(t, content, "cardinality: 2")
	assert.Contains(t, content, "verbosity: normal")
	assert.NotContains(t, content, "verbosity: detailed")
	assert.Contains(t, content, "otlp:")
	assert.Contains(t, content, "pipelines:")
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

	t.Run("replace secrets in jq arguments", func(t *testing.T) {
		ops := Operations{
			DeploymentID: "test-config",
			FileOperations: []FileOperation{
				{
					FileOperationType: FileOperationJQ,
					Transform:         `.api_key = $api_key`,
					Arguments:         json.RawMessage(`{"api_key": "SEC[apikey]"}`),
				},
			},
		}

		err := ReplaceSecrets(&ops, map[string]string{
			"apikey": "my-api-key",
		})

		assert.NoError(t, err)
		// The transform text is untouched; only the argument value is substituted.
		assert.Equal(t, `.api_key = $api_key`, ops.FileOperations[0].Transform)
		assert.JSONEq(t, `{"api_key": "my-api-key"}`, string(ops.FileOperations[0].Arguments))
	})

	t.Run("replace secrets nested in jq arguments", func(t *testing.T) {
		ops := Operations{
			DeploymentID: "test-config",
			FileOperations: []FileOperation{
				{
					FileOperationType: FileOperationJQ,
					Transform:         `.auth = $auth`,
					// Placeholder nested inside an object argument.
					Arguments: json.RawMessage(`{"auth": {"api_key": "SEC[apikey]", "tokens": ["SEC[apikey]"]}}`),
				},
			},
		}

		err := ReplaceSecrets(&ops, map[string]string{
			"apikey": "my-api-key",
		})

		assert.NoError(t, err)
		assert.JSONEq(t, `{"auth": {"api_key": "my-api-key", "tokens": ["my-api-key"]}}`, string(ops.FileOperations[0].Arguments))
	})

	t.Run("unreplaced secret in jq arguments returns error", func(t *testing.T) {
		ops := Operations{
			DeploymentID: "test-config",
			FileOperations: []FileOperation{
				{
					FileOperationType: FileOperationJQ,
					Transform:         `.api_key = $api_key`,
					Arguments:         json.RawMessage(`{"api_key": "SEC[apikey]"}`),
				},
			},
		}

		err := ReplaceSecrets(&ops, map[string]string{
			"wrong-key": "some-value",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "secrets are not fully replaced")
	})

	t.Run("secret embedded directly in transform is not substituted and errors", func(t *testing.T) {
		ops := Operations{
			DeploymentID: "test-config",
			FileOperations: []FileOperation{
				{
					FileOperationType: FileOperationJQ,
					Transform:         `.api_key = "SEC[apikey]"`,
				},
			},
		}

		// Even though the secret is provided, secrets are only substituted into arguments,
		// so a SEC[...] left in the transform text is rejected.
		err := ReplaceSecrets(&ops, map[string]string{
			"apikey": "my-api-key",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "secrets are not fully replaced")
	})
}

func TestOperationApply_JQ(t *testing.T) {
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

	// JQ: set foo to baz
	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.foo = "baz"`,
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "baz", updatedMap["foo"])

	if runtime.GOOS != "windows" {
		stat, err := os.Stat(filePath)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0640), stat.Mode().Perm())
	}
}

func TestOperationApply_JQWithArguments(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	orig := map[string]any{"tags": []any{"team:fleet"}}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// The transform is a static program; the values come from arguments as $vars.
	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.api_key = $api_key | .tags += [$env_tag]`,
		Arguments:         json.RawMessage(`{"api_key": "abcd1234", "env_tag": "env:prod"}`),
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "abcd1234", updatedMap["api_key"])
	assert.Equal(t, []any{"team:fleet", "env:prod"}, updatedMap["tags"])
}

func TestOperationApply_JQWithTypedArguments(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	err := os.WriteFile(filePath, []byte("foo: bar\n"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// Arguments are typed JSON values, not just strings: a number, a bool, and an object.
	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.workers = $workers | .logs_enabled = $enabled | .apm_config = $apm`,
		Arguments:         json.RawMessage(`{"workers": 4, "enabled": true, "apm": {"enabled": false}}`),
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, 4, updatedMap["workers"])
	assert.Equal(t, true, updatedMap["logs_enabled"])
	assert.Equal(t, map[any]any{"enabled": false}, updatedMap["apm_config"])
}

func TestOperationApply_JQUndeclaredVariable(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	err := os.WriteFile(filePath, []byte("foo: bar\n"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// $missing is referenced but not provided as an argument: compile error.
	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.foo = $missing`,
	}

	err = op.apply(context.Background(), root)
	assert.Error(t, err)
}

func TestOperationApply_JQConditionalTransform(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	orig := map[string]any{
		"logs_enabled": true,
		"tags":         []any{"env:prod", "team:fleet"},
	}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// Conditional restructuring that JSON patch/merge-patch cannot express:
	// uppercase every tag, and add a flag derived from another field.
	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.tags |= map(ascii_upcase) | .logs_config = {"enabled": .logs_enabled}`,
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, []any{"ENV:PROD", "TEAM:FLEET"}, updatedMap["tags"])
	assert.Equal(t, map[any]any{"enabled": true}, updatedMap["logs_config"])
}

func TestOperationApply_JQMultipleOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	orig := map[string]any{"items": []any{"a", "b"}}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// A transform yielding more than one output is written as a multi-document YAML stream.
	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.items[] | {value: .}`,
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	decoder := yaml.NewDecoder(strings.NewReader(string(updated)))
	var docs []map[string]any
	for {
		var doc map[string]any
		if err := decoder.Decode(&doc); err != nil {
			break
		}
		docs = append(docs, doc)
	}
	assert.Equal(t, []map[string]any{{"value": "a"}, {"value": "b"}}, docs)
}

func TestOperationApply_JQWithTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	// yaml.v2 decodes this value into a time.Time, which gojq cannot process unless normalized.
	err := os.WriteFile(filePath, []byte("created_at: 2021-01-02T15:04:05Z\nfoo: bar\n"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.foo = "baz"`,
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "baz", updatedMap["foo"])
	assert.Equal(t, "2021-01-02T15:04:05Z", updatedMap["created_at"]) // no sub-seconds: RFC3339 and RFC3339Nano are identical
}

func TestOperationApply_JQWithSubSecondTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	// yaml.v2 parses this to a time.Time with nanosecond precision.
	err := os.WriteFile(filePath, []byte("created_at: 2021-01-02T15:04:05.123456789Z\nfoo: bar\n"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.foo = "baz"`,
	}

	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)
	assert.Equal(t, "baz", updatedMap["foo"])
	// Sub-second precision must be preserved (RFC3339Nano, not RFC3339).
	assert.Equal(t, "2021-01-02T15:04:05.123456789Z", updatedMap["created_at"])
}

func TestOperationApply_JQInvalidTransform(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	err := os.WriteFile(filePath, []byte("foo: bar\n"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.foo |`, // syntax error
	}

	err = op.apply(context.Background(), root)
	assert.Error(t, err)
}

func TestOperationApply_JQRuntimeError(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	err := os.WriteFile(filePath, []byte("foo: bar\n"), 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	// .foo is a string, indexing it like an object is a runtime error.
	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         `.foo.bar`,
	}

	err = op.apply(context.Background(), root)
	assert.Error(t, err)
}

// applyJQToTags writes a realistic datadog.yaml whose `tags` field is set to the
// given tags, applies the jq transform, and returns the resulting tags slice. It also
// asserts that the other (unrelated) config fields are left untouched.
func applyJQToTags(t *testing.T, tags []any, transform string) []any {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")

	orig := map[string]any{
		"api_key":      "0123456789abcdef0123456789abcdef",
		"site":         "datadoghq.com",
		"hostname":     "my-host",
		"log_level":    "info",
		"tags":         tags,
		"logs_enabled": true,
		"logs_config": map[string]any{
			"container_collect_all": true,
		},
		"apm_config": map[string]any{
			"enabled": true,
		},
	}
	origBytes, err := yaml.Marshal(orig)
	assert.NoError(t, err)
	err = os.WriteFile(filePath, origBytes, 0644)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationJQ,
		FilePath:          "/datadog.yaml",
		Transform:         transform,
	}
	err = op.apply(context.Background(), root)
	assert.NoError(t, err)

	updated, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	var updatedMap map[string]any
	err = yaml.Unmarshal(updated, &updatedMap)
	assert.NoError(t, err)

	// Every field other than `tags` must be preserved exactly.
	assert.Equal(t, "0123456789abcdef0123456789abcdef", updatedMap["api_key"])
	assert.Equal(t, "datadoghq.com", updatedMap["site"])
	assert.Equal(t, "my-host", updatedMap["hostname"])
	assert.Equal(t, "info", updatedMap["log_level"])
	assert.Equal(t, true, updatedMap["logs_enabled"])
	assert.Equal(t, map[any]any{"container_collect_all": true}, updatedMap["logs_config"])
	assert.Equal(t, map[any]any{"enabled": true}, updatedMap["apm_config"])

	got, _ := updatedMap["tags"].([]any)
	return got
}

func TestOperationApply_JQAddTagIfMissing(t *testing.T) {
	// Idempotent add: only append the tag when it is not already present.
	const transform = `if (.tags | index("env:prod")) == null then .tags += ["env:prod"] else . end`

	t.Run("tag missing is added", func(t *testing.T) {
		got := applyJQToTags(t, []any{"team:fleet"}, transform)
		assert.Equal(t, []any{"team:fleet", "env:prod"}, got)
	})

	t.Run("tag already present is not duplicated", func(t *testing.T) {
		got := applyJQToTags(t, []any{"env:prod", "team:fleet"}, transform)
		assert.Equal(t, []any{"env:prod", "team:fleet"}, got)
	})
}

func TestOperationApply_JQReplaceTag(t *testing.T) {
	const transform = `.tags |= map(if . == "env:staging" then "env:prod" else . end)`
	got := applyJQToTags(t, []any{"env:staging", "team:fleet"}, transform)
	assert.Equal(t, []any{"env:prod", "team:fleet"}, got)
}

func TestOperationApply_JQDeleteTag(t *testing.T) {
	const transform = `.tags -= ["env:staging"]`
	got := applyJQToTags(t, []any{"env:staging", "team:fleet"}, transform)
	assert.Equal(t, []any{"team:fleet"}, got)
}

func TestFillRestoration(t *testing.T) {
	stableDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte(`api_key: my_apikey
snmp_traps:
  community_strings:
    - abc
    - def
`), 0644))

	operations := Operations{
		FileOperations: []FileOperation{
			{
				FileOperationType: FileOperationMergePatch,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(`["untouched"]`),
			},
			{
				FileOperationType: FileOperationRestore,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(`["api_key", "snmp_traps.community_strings.0", "does_not_exist", "a/b~c"]`),
			},
		},
	}

	err := fillRestoration(&operations, stableDir)
	require.NoError(t, err)

	// Non-restore operations are left untouched
	assert.JSONEq(t, `["untouched"]`, string(operations.FileOperations[0].Patch))
	assert.Equal(t, FileOperationRestore, operations.FileOperations[1].FileOperationType)
	assert.Equal(t, "/datadog.yaml", operations.FileOperations[1].FilePath)
	assert.JSONEq(t, `[
		{"op": "remove", "path": "/api_key", "value": null},
		{"op": "add", "path": "/api_key", "value": "my_apikey"},
		{"op": "remove", "path": "/snmp_traps/community_strings/0", "value": null},
		{"op": "add", "path": "/snmp_traps/community_strings/0", "value": "abc"},
		{"op": "remove", "path": "/does_not_exist", "value": null},
		{"op": "remove", "path": "/a~1b~0c", "value": null}
	]`, string(operations.FileOperations[1].Patch))
}

// TestRestore fills restore operations from a stable config and applies them to a modified
// experiment config, checking the settings end up restored to their stable values.
func TestRestore(t *testing.T) {
	tests := []struct {
		name       string
		filePath   string
		stable     string
		experiment string
		settings   string
		expected   string
	}{
		{
			name:       "core config",
			filePath:   "/datadog.yaml",
			stable:     "api_key: my_apikey\nsite: datadoghq.com\nlogs_config:\n  container_collect_all: true\n",
			experiment: "api_key: new_apikey\nsite: datadoghq.eu\nlogs_config:\n  container_collect_all: false\n",
			settings:   `["api_key", "logs_config.container_collect_all"]`,
			expected:   "api_key: my_apikey\nsite: datadoghq.eu\nlogs_config:\n  container_collect_all: true\n",
		},
		{
			name:       "core config with list index",
			filePath:   "/datadog.yaml",
			stable:     "api_key: my_apikey\nsnmp_traps:\n  community_strings: [abc, def]\n",
			experiment: "api_key: new_apikey\nsnmp_traps:\n  community_strings: [xyz, uvw]\n",
			settings:   `["api_key", "snmp_traps.community_strings.0"]`,
			expected:   "api_key: my_apikey\nsnmp_traps:\n  community_strings: [abc, uvw]\n",
		},
		{
			name:       "core config with nested list index",
			filePath:   "/datadog.yaml",
			stable:     "logs_config:\n  processing_rules:\n    - type: exclude_at_match\n      name: exclude_healthcheck\n",
			experiment: "logs_config:\n  processing_rules:\n    - type: exclude_at_match\n      name: renamed\n",
			settings:   `["logs_config.processing_rules.0.name"]`,
			expected:   "logs_config:\n  processing_rules:\n    - type: exclude_at_match\n      name: exclude_healthcheck\n",
		},
		{
			name:       "list index appended to shorter list",
			filePath:   "/datadog.yaml",
			stable:     "snmp_traps:\n  community_strings: [abc, def]\n",
			experiment: "snmp_traps:\n  community_strings: [abc]\n",
			settings:   `["snmp_traps.community_strings.1"]`,
			expected:   "snmp_traps:\n  community_strings: [abc, def]\n",
		},
		{
			name:       "whole section",
			filePath:   "/datadog.yaml",
			stable:     "snmp_traps:\n  enabled: true\n  community_strings: [abc, def]\n",
			experiment: "snmp_traps:\n  enabled: false\n  port: 9162\n",
			settings:   `["snmp_traps"]`,
			expected:   "snmp_traps:\n  enabled: true\n  community_strings: [abc, def]\n",
		},
		{
			name:       "setting missing from experiment is added with its parents",
			filePath:   "/datadog.yaml",
			stable:     "api_key: my_apikey\nlogs_config:\n  processing_rules:\n    - name: rule\n",
			experiment: "site: datadoghq.eu\n",
			settings:   `["api_key", "logs_config.processing_rules"]`,
			expected:   "api_key: my_apikey\nsite: datadoghq.eu\nlogs_config:\n  processing_rules:\n    - name: rule\n",
		},
		{
			name:       "setting missing from stable is removed",
			filePath:   "/datadog.yaml",
			stable:     "api_key: my_apikey\n",
			experiment: "api_key: my_apikey\nlogs_enabled: true\n",
			settings:   `["logs_enabled", "logs_config.container_collect_all"]`,
			expected:   "api_key: my_apikey\n",
		},
		{
			name:       "null setting",
			filePath:   "/datadog.yaml",
			stable:     "proxy: null\n",
			experiment: "proxy:\n  http: http://proxy\n",
			settings:   `["proxy"]`,
			expected:   "proxy: null\n",
		},
		{
			name:       "system-probe config",
			filePath:   "/system-probe.yaml",
			stable:     "network_config:\n  enabled: true\nservice_monitoring_config:\n  enabled: false\n",
			experiment: "network_config:\n  enabled: false\nservice_monitoring_config:\n  enabled: true\n",
			settings:   `["network_config.enabled", "service_monitoring_config.enabled"]`,
			expected:   "network_config:\n  enabled: true\nservice_monitoring_config:\n  enabled: false\n",
		},
		{
			name:       "integration config",
			filePath:   "/conf.d/redisdb.d/conf.yaml",
			stable:     "init_config: {}\ninstances:\n  - host: localhost\n    port: 6379\n    tags: [env:prod]\n",
			experiment: "init_config: {}\ninstances:\n  - host: localhost\n    port: 6380\n    tags: [env:staging]\n",
			settings:   `["instances.0.port", "instances.0.tags.0"]`,
			expected:   "init_config: {}\ninstances:\n  - host: localhost\n    port: 6379\n    tags: [env:prod]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stableDir := t.TempDir()
			experimentDir := t.TempDir()
			filePath := strings.TrimPrefix(tt.filePath, "/")
			for dir, content := range map[string]string{stableDir: tt.stable, experimentDir: tt.experiment} {
				fullPath := filepath.Join(dir, filePath)
				require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
				require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
			}
			operations := Operations{
				FileOperations: []FileOperation{
					{FileOperationType: FileOperationRestore, FilePath: tt.filePath, Patch: []byte(tt.settings)},
				},
			}

			require.NoError(t, fillRestoration(&operations, stableDir))
			require.NoError(t, operations.Apply(context.Background(), experimentDir))

			restored, err := os.ReadFile(filepath.Join(experimentDir, filePath))
			require.NoError(t, err)
			var restoredMap, expectedMap map[string]any
			require.NoError(t, yaml.Unmarshal(restored, &restoredMap))
			require.NoError(t, yaml.Unmarshal([]byte(tt.expected), &expectedMap))
			assert.Equal(t, expectedMap, restoredMap)
		})
	}
}

func TestFillRestoration_MissingFile(t *testing.T) {
	operations := Operations{
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationRestore, FilePath: "/security-agent.yaml", Patch: []byte(`["runtime_security_config.enabled"]`)},
		},
	}

	err := fillRestoration(&operations, t.TempDir())
	require.NoError(t, err)

	assert.JSONEq(t, `[{"op": "remove", "path": "/runtime_security_config/enabled", "value": null}]`, string(operations.FileOperations[0].Patch))
}

func TestFillRestoration_MultipleOperations(t *testing.T) {
	stableDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: my_apikey\nsite: datadoghq.eu\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "system-probe.yaml"), []byte("network_config:\n  enabled: true\n"), 0644))

	operations := Operations{
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationRestore, FilePath: "/datadog.yaml", Patch: []byte(`["api_key"]`)},
			{FileOperationType: FileOperationRestore, FilePath: "/system-probe.yaml", Patch: []byte(`["network_config.enabled"]`)},
			{FileOperationType: FileOperationRestore, FilePath: "/datadog.yaml", Patch: []byte(`["site"]`)},
		},
	}

	err := fillRestoration(&operations, stableDir)
	require.NoError(t, err)

	assert.JSONEq(t, `[{"op": "remove", "path": "/api_key", "value": null}, {"op": "add", "path": "/api_key", "value": "my_apikey"}]`, string(operations.FileOperations[0].Patch))
	assert.JSONEq(t, `[{"op": "remove", "path": "/network_config/enabled", "value": null}, {"op": "add", "path": "/network_config/enabled", "value": true}]`, string(operations.FileOperations[1].Patch))
	assert.JSONEq(t, `[{"op": "remove", "path": "/site", "value": null}, {"op": "add", "path": "/site", "value": "datadoghq.eu"}]`, string(operations.FileOperations[2].Patch))
}

func TestFillRestoration_Errors(t *testing.T) {
	stableDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: [unclosed\n"), 0644))

	tests := []struct {
		name     string
		filePath string
		settings string
	}{
		{name: "disallowed file", filePath: "/not-allowed.yaml", settings: `["api_key"]`},
		{name: "patch is not an array", filePath: "/system-probe.yaml", settings: `{"api_key": "x"}`},
		{name: "malformed stable file", filePath: "/datadog.yaml", settings: `["api_key"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operations := Operations{
				FileOperations: []FileOperation{
					{FileOperationType: FileOperationRestore, FilePath: tt.filePath, Patch: []byte(tt.settings)},
				},
			}
			assert.Error(t, fillRestoration(&operations, stableDir))
		})
	}
}

func TestOperationApply_Restore(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte("api_key: new_apikey\nsite: datadoghq.com\n"), 0644))

	root, err := os.OpenRoot(tmpDir)
	require.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationRestore,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(`[{"op": "remove", "path": "/api_key"}, {"op": "add", "path": "/api_key", "value": "my_apikey"}, {"op": "remove", "path": "/missing/setting"}]`),
	}
	require.NoError(t, op.apply(context.Background(), root))

	updated, err := os.ReadFile(filePath)
	require.NoError(t, err)
	var updatedMap map[string]any
	require.NoError(t, yaml.Unmarshal(updated, &updatedMap))
	assert.Equal(t, map[string]any{"api_key": "my_apikey", "site": "datadoghq.com"}, updatedMap)
}
