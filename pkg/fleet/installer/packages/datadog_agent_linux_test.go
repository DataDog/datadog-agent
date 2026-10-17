// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	installerFile "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivilegedRshellPolicyDirectoryPermissions(t *testing.T) {
	assert.Equal(t, "/etc/datadog-agent-rshell", privilegedRshellPolicyDir)
	assert.Contains(t, agentDirectories, installerFile.Directory{
		Path:  privilegedRshellPolicyDir,
		Mode:  0755,
		Owner: "root",
		Group: "root",
	})
	assert.NotEqual(t, "/etc/datadog-agent", filepath.Dir(privilegedRshellPolicyDir),
		"the policy directory must not be below the dd-agent-owned config directory")
}

func TestPrivilegedRshellPackagePermissionsProtectParentDirectories(t *testing.T) {
	assert.Equal(t, installerFile.Permissions{
		{Path: "embedded", Owner: "root", Group: "root", Mode: 0755},
		{Path: "embedded/bin", Owner: "root", Group: "root", Mode: 0755},
		{Path: privilegedRshellBinaryRelPath, Owner: "root", Group: "root", Mode: 0755},
	}, privilegedRshellPackagePermissions)
	for _, permission := range privilegedRshellPackagePermissions {
		assert.NotEqual(t, ".", permission.Path, "the package root must keep dd-agent ownership")
	}
}

func TestEnsurePrivilegedRshellPermissionsRejectsSymlink(t *testing.T) {
	packagePath := t.TempDir()
	binaryPath := filepath.Join(packagePath, privilegedRshellBinaryRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
	require.NoError(t, os.Symlink("/bin/true", binaryPath))

	err := ensurePrivilegedRshellPermissions(HookContext{Context: context.Background(), PackagePath: packagePath})
	require.ErrorContains(t, err, "privileged rshell helper is not a regular file")
}

func TestPrivilegedRshellSupported(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("privileged rshell is supported only on amd64 and arm64")
	}

	packagePath := t.TempDir()
	binaryPath := filepath.Join(packagePath, privilegedRshellBinaryRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
	require.NoError(t, os.WriteFile(binaryPath, []byte("rshell"), 0755))

	originalGetLandlockABIVersion := getLandlockABIVersion
	t.Cleanup(func() { getLandlockABIVersion = originalGetLandlockABIVersion })

	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI, nil }
	assert.True(t, privilegedRshellSupported(packagePath))

	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI - 1, nil }
	assert.False(t, privilegedRshellSupported(packagePath))

	getLandlockABIVersion = func() (int, error) { return 0, errors.New("landlock unavailable") }
	assert.False(t, privilegedRshellSupported(packagePath))

	require.NoError(t, os.Remove(binaryPath))
	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI, nil }
	assert.False(t, privilegedRshellSupported(packagePath))
}

func TestPrivilegedRshellEnabled(t *testing.T) {
	configDir := t.TempDir()

	originalEnv, envWasSet := os.LookupEnv(privilegedRshellEnabledEnv)
	require.NoError(t, os.Unsetenv(privilegedRshellEnabledEnv))
	t.Cleanup(func() {
		if envWasSet {
			require.NoError(t, os.Setenv(privilegedRshellEnabledEnv, originalEnv))
		} else {
			require.NoError(t, os.Unsetenv(privilegedRshellEnabledEnv))
		}
	})

	assert.False(t, privilegedRshellEnabled(configDir), "the helper must be disabled when config is absent")

	require.NoError(t, os.WriteFile(filepath.Join(configDir, "datadog.yaml"), []byte(`
private_action_runner:
  restricted_shell:
    privileged:
      enabled: true
`), 0640))
	assert.True(t, privilegedRshellEnabled(configDir))

	managedDir := filepath.Join(configDir, "managed", agentPackage, "stable")
	require.NoError(t, os.MkdirAll(managedDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(managedDir, "datadog.yaml"), []byte(`
private_action_runner:
  restricted_shell:
    privileged:
      enabled: false
`), 0640))
	assert.False(t, privilegedRshellEnabled(configDir), "managed config must override the base config")

	require.NoError(t, os.Setenv(privilegedRshellEnabledEnv, "true"))
	assert.True(t, privilegedRshellEnabled(configDir), "the environment must override file config")

	require.NoError(t, os.Setenv(privilegedRshellEnabledEnv, "not-a-bool"))
	assert.False(t, privilegedRshellEnabled(configDir), "an invalid environment value must fail closed")
}

func TestSyncPrivilegedRshellFlag(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("privileged rshell is supported only on amd64 and arm64")
	}

	packagePath := t.TempDir()
	binaryPath := filepath.Join(packagePath, privilegedRshellBinaryRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
	require.NoError(t, os.WriteFile(binaryPath, []byte("rshell"), 0755))

	originalGetLandlockABIVersion := getLandlockABIVersion
	t.Cleanup(func() { getLandlockABIVersion = originalGetLandlockABIVersion })
	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI, nil }

	configDir := t.TempDir()
	flagPath := filepath.Join(t.TempDir(), "rshell", "enabled")

	t.Setenv(privilegedRshellEnabledEnv, "true")
	require.NoError(t, syncPrivilegedRshellFlag(configDir, packagePath, flagPath))
	assert.FileExists(t, flagPath, "the flag must be created when the feature is enabled")

	t.Setenv(privilegedRshellEnabledEnv, "false")
	require.NoError(t, syncPrivilegedRshellFlag(configDir, packagePath, flagPath))
	assert.NoFileExists(t, flagPath, "the flag must be removed when the feature is disabled")
	require.NoError(t, syncPrivilegedRshellFlag(configDir, packagePath, flagPath), "removing a missing flag must succeed")

	t.Setenv(privilegedRshellEnabledEnv, "true")
	getLandlockABIVersion = func() (int, error) { return 0, errors.New("landlock unavailable") }
	require.NoError(t, syncPrivilegedRshellFlag(configDir, packagePath, flagPath))
	assert.NoFileExists(t, flagPath, "the flag must not be created on unsupported hosts")
}

func TestRunDatadogAgentPackageCommandRejectsUnknownCommand(t *testing.T) {
	require.ErrorContains(t, runDatadogAgentPackageCommand(context.Background(), "nope"), "unknown datadog-agent package command")
}
