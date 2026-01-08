// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func assertConfigV2(t *testing.T, v2Dir string) {
	// /managed/datadog-agent/stable -> /etc/datadog-agent/managed/datadog-agent/v2
	// /managed/datadog-agent/experiment -> /etc/datadog-agent/managed/datadog-agent/v2
	// /managed/datadog-agent/v2/
	//     datadog.yaml
	//     application_monitoring.yaml
	//     conf.d/mycheck.d/config.yaml
	managedDir := filepath.Join(v2Dir, "managed", "datadog-agent")
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
	_, err = os.Lstat(filepath.Join(v2Dir, "conf.d", "mycheck.d", "config.yaml"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func assertConfigV3(t *testing.T, v3Dir string) {
	// Check the content of the v3 directory
	// /managed/datadog-agent/stable
	//     application_monitoring.yaml
	// No more symlinks
	managedDir := filepath.Join(v3Dir, "managed", "datadog-agent")
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
	_, err = os.Lstat(filepath.Join(v3Dir, "conf.d", "mycheck.d", "config.yaml"))
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
	stableDirPath := t.TempDir()
	stableManagedDirPath := filepath.Join(stableDirPath, "managed", "datadog-agent")
	err := os.MkdirAll(stableManagedDirPath, 0755)
	assert.NoError(t, err)

	// Create a v2 tree
	writeConfigV2(t, stableDirPath)
	assertConfigV2(t, stableDirPath) // Make sure it's correct

	// Convert v2 to v3
	experimentDirPath := t.TempDir()
	dirs := &Directories{
		StablePath:     stableDirPath,
		ExperimentPath: experimentDirPath,
	}

	assertDeploymentID(t, dirs, "", "")

	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-456",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)

	assertDeploymentID(t, dirs, "", "experiment-456")

	assertConfigV2(t, stableDirPath) // Make sure nothing changed
	assertConfigV3(t, experimentDirPath)

	// Promote
	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV3(t, stableDirPath) // Make sure it changed

	assertDeploymentID(t, dirs, "experiment-456", "")
}

func TestConfigV2Rollback(t *testing.T) {
	stableDirPath := t.TempDir()
	stableManagedDirPath := filepath.Join(stableDirPath, "managed", "datadog-agent")
	err := os.MkdirAll(stableManagedDirPath, 0755)
	assert.NoError(t, err)

	// Create a v2 tree
	writeConfigV2(t, stableDirPath)
	assertConfigV2(t, stableDirPath) // Make sure it's correct

	// Convert v2 to v3
	experimentDirPath := t.TempDir()
	dirs := &Directories{
		StablePath:     stableDirPath,
		ExperimentPath: experimentDirPath,
	}

	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-456",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)

	assertConfigV2(t, stableDirPath) // Make sure nothing changed
	assertConfigV3(t, experimentDirPath)

	assertDeploymentID(t, dirs, "", "experiment-456")

	// Rollback
	err = dirs.RemoveExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV2(t, stableDirPath) // Make sure it's still v2

	// Write again
	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-789",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)
	assertConfigV2(t, stableDirPath) // Make sure it's still v2
	assertConfigV3(t, experimentDirPath)

	// Promote
	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV3(t, stableDirPath) // Make sure it changed

	assertDeploymentID(t, dirs, "experiment-789", "")
}
