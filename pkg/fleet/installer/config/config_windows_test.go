// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

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
	_, err = os.Stat(filepath.Join(managedDir, "v2", "datadog.yaml"))
	assert.NoError(t, err)
	_, err = os.Lstat(filepath.Join(managedDir, "v2", "application_monitoring.yaml"))
	assert.NoError(t, err)
	_, err = os.Lstat(filepath.Join(managedDir, "v2", "conf.d", "mycheck.d", "config.yaml"))
	assert.NoError(t, err)

	_, err = os.Lstat(filepath.Join(managedDir, "stable"))
	assert.NoError(t, err)
	_, err = os.Lstat(filepath.Join(managedDir, "experiment"))
	assert.NoError(t, err)

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

	_, err = os.Lstat(filepath.Join(managedDir, "stable"))
	assert.NoError(t, err)

	// Check the files are not here anymore, except application_monitoring.yaml
	_, err = os.Lstat(filepath.Join(managedDir, "stable", "datadog.yaml"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Lstat(filepath.Join(managedDir, "stable", "application_monitoring.yaml"))
	assert.NoError(t, err)
	_, err = os.Lstat(filepath.Join(managedDir, "stable", "conf.d", "mycheck.d", "config.yaml"))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	// v3Dir/conf.d/mychecks.d/config.yaml exists
	_, err = os.Lstat(filepath.Join(v3Dir, "conf.d", "mycheck.d", "config.yaml"))
	assert.NoError(t, err)
}

func assertDeploymentID(t *testing.T, dirs *Directories, stableDeploymentID string, experimentDeploymentID string) {
	state, err := dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, stableDeploymentID, state.StableDeploymentID)
	assert.Equal(t, experimentDeploymentID, state.ExperimentDeploymentID)
}

func TestConfigV2ToV3(t *testing.T) {
	originalDirPath := t.TempDir()
	originalManagedDirPath := filepath.Join(originalDirPath, "managed", "datadog-agent")
	err := os.MkdirAll(originalManagedDirPath, 0755)
	assert.NoError(t, err)

	// Create a v2 tree
	writeConfigV2(t, originalDirPath)
	assertConfigV2(t, originalDirPath) // Make sure it's correct

	// Convert v2 to v3
	backupDirPath := t.TempDir()
	dirs := &Directories{
		StablePath:     originalDirPath,
		ExperimentPath: backupDirPath,
	}

	assertDeploymentID(t, dirs, "", "")

	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-456",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)

	// On windows, experimentsDirPath contains the backup of the stable state
	assertDeploymentID(t, dirs, "", "experiment-456")

	assertConfigV2(t, backupDirPath) // Make sure nothing changed
	assertConfigV3(t, originalDirPath)

	// Promote
	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV3(t, originalDirPath) // Make sure it changed

	assertDeploymentID(t, dirs, "experiment-456", "")
}

func TestConfigV2Rollback(t *testing.T) {
	originalDirPath := t.TempDir()
	originalManagedDirPath := filepath.Join(originalDirPath, "managed", "datadog-agent")
	err := os.MkdirAll(originalManagedDirPath, 0755)
	assert.NoError(t, err)

	// Create a v2 tree
	writeConfigV2(t, originalDirPath)
	assertConfigV2(t, originalDirPath) // Make sure it's correct

	// Convert v2 to v3
	backupDirPath := t.TempDir()
	dirs := &Directories{
		StablePath:     originalDirPath,
		ExperimentPath: backupDirPath,
	}

	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-456",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)

	// On windows, experimentsDirPath contains the backup of the stable state
	assertConfigV2(t, backupDirPath) // Make sure nothing changed
	assertConfigV3(t, originalDirPath)

	assertDeploymentID(t, dirs, "", "experiment-456")

	// Rollback
	err = dirs.RemoveExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV2(t, originalDirPath) // Make sure it went back to the original state

	// Write again
	err = dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "experiment-789",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "info"}`)},
		},
	})
	assert.NoError(t, err)
	assertConfigV2(t, backupDirPath)
	assertConfigV3(t, originalDirPath)

	// Promote
	err = dirs.PromoteExperiment(context.Background())
	assert.NoError(t, err)
	assertConfigV3(t, originalDirPath) // Make sure it changed

	assertDeploymentID(t, dirs, "experiment-789", "")
}

func TestSecureCreateTargetDirectoryWithSourcePermissions(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source")
	targetPath := filepath.Join(t.TempDir(), "target")
	// Set the source path to a directory with known SDDL
	sddl := "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;CI;FA;;;WD)"
	assert.NoError(t, paths.SecureCreateDirectory(sourcePath, sddl))

	// Create the target directory with the same permissions as the source directory
	assert.NoError(t, secureCreateTargetDirectoryWithSourcePermissions(sourcePath, targetPath))

	// Check the target directory has the same permissions as the source directory
	targetSD, err := windows.GetNamedSecurityInfo(targetPath, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	assert.NoError(t, err)
	sourceSD, err := windows.GetNamedSecurityInfo(sourcePath, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	assert.NoError(t, err)
	assert.Equal(t, sourceSD.String(), targetSD.String())
}

// everyoneCanRead reports whether path's DACL contains an allow ACE granting read access
// to the Everyone group (S-1-1-0).
func everyoneCanRead(t *testing.T, path string) bool {
	t.Helper()
	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	assert.NoError(t, err)

	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	assert.NoError(t, err)
	dacl, _, err := sd.DACL()
	assert.NoError(t, err)

	const (
		genericRead  = 0x80000000 // GENERIC_READ, if not mapped to file-specific rights
		fileReadData = 0x1        // FILE_READ_DATA, set when GENERIC_READ is mapped to FILE_GENERIC_READ
	)
	for i := 0; i < int(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		assert.NoError(t, windows.GetAce(dacl, uint32(i), &ace))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(everyone) && ace.Mask&(genericRead|fileReadData) != 0 {
			return true
		}
	}
	return false
}

// TestOperationApply_ApplicationMonitoringEveryoneRead verifies the world-readable config file
// (mode 0644 on Linux) gets an Everyone-read ACL on Windows, so non-admin identities such as
// IIS App Pool can read it.
func TestOperationApply_ApplicationMonitoringEveryoneRead(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "application_monitoring.yaml")
	assert.NoError(t, os.WriteFile(filePath, []byte("enabled: true\n"), 0600))

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationPatch,
		FilePath:          "/application_monitoring.yaml",
		Patch:             []byte(`[{"op": "replace", "path": "/enabled", "value": false}]`),
	}
	assert.NoError(t, op.apply(context.Background(), root))

	assert.True(t, everyoneCanRead(t, filePath), "application_monitoring.yaml should grant Everyone read")
}

// TestOperationApply_RestrictedFileNoEveryoneRead verifies a restricted config file (mode 0640
// on Linux) does NOT get an Everyone-read ACL on Windows.
func TestOperationApply_RestrictedFileNoEveryoneRead(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "datadog.yaml")
	assert.NoError(t, os.WriteFile(filePath, []byte("log_level: info\n"), 0600))

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	op := &FileOperation{
		FileOperationType: FileOperationMergePatch,
		FilePath:          "/datadog.yaml",
		Patch:             []byte(`{"log_level":"debug"}`),
	}
	assert.NoError(t, op.apply(context.Background(), root))

	assert.False(t, everyoneCanRead(t, filePath), "datadog.yaml (0640) should not grant Everyone read")
}

func TestRemoveExperiment_RestoresApplicationMonitoringEveryoneRead(t *testing.T) {
	stablePath := t.TempDir()
	experimentPath := t.TempDir()
	filePath := filepath.Join(stablePath, "application_monitoring.yaml")
	assert.NoError(t, os.WriteFile(filePath, []byte("enabled: true\n"), 0600))
	assert.NoError(t, paths.SetFileReadableByEveryone(filePath))
	assert.True(t, everyoneCanRead(t, filePath), "application_monitoring.yaml should start with Everyone read")

	dirs := &Directories{
		StablePath:     stablePath,
		ExperimentPath: experimentPath,
	}
	err := dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "delete-application-monitoring",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationDelete, FilePath: "/application_monitoring.yaml"},
		},
	})
	assert.NoError(t, err)
	assert.NoFileExists(t, filePath)

	assert.NoError(t, dirs.RemoveExperiment(context.Background()))
	assert.FileExists(t, filePath)
	assert.True(t, everyoneCanRead(t, filePath), "application_monitoring.yaml should regain Everyone read after rollback restore")
}

// TestDeploymentIDAfterRollback reproduces the bug where RemoveExperiment incorrectly
// copies the experiment deployment ID to stable during rollback.
func TestDeploymentIDAfterRollback(t *testing.T) {
	stablePath := t.TempDir()
	experimentPath := t.TempDir()

	dirs := &Directories{
		StablePath:     stablePath,
		ExperimentPath: experimentPath,
	}

	// Initial state: no deployment IDs
	assertDeploymentID(t, dirs, "", "")

	// Start a config experiment with deployment ID "abc-def-ghi"
	err := dirs.WriteExperiment(context.Background(), Operations{
		DeploymentID: "abc-def-ghi",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		},
	})
	assert.NoError(t, err)

	// After WriteExperiment:
	// - stable should have empty deployment ID (no changes promoted yet)
	// - experiment should have "abc-def-ghi"
	assertDeploymentID(t, dirs, "", "abc-def-ghi")

	// Now rollback the experiment
	err = dirs.RemoveExperiment(context.Background())
	assert.NoError(t, err)

	// BUG: After RemoveExperiment, stable deployment ID is incorrectly set to "abc-def-ghi"
	// Expected: stable should remain empty since no config was ever promoted
	// Actual: stable gets "abc-def-ghi" copied from experiment during restore
	state, err := dirs.GetState()
	assert.NoError(t, err)

	// This assertion will FAIL, exposing the bug:
	// RemoveExperiment calls backupOrRestoreDirectory(experiment -> stable) which copies
	// the deployment ID from experiment/.deployment-id to stable/.deployment-id
	assert.Equal(t, "", state.StableDeploymentID,
		"BUG: stable_config_version should remain empty after rollback, but got: %s",
		state.StableDeploymentID)
	assert.Equal(t, "", state.ExperimentDeploymentID, "experiment should be deleted")
}
