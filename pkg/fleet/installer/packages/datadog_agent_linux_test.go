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
	originalConfigDir := privilegedRshellConfigDir
	privilegedRshellConfigDir = func(HookContext) string { return configDir }
	t.Cleanup(func() { privilegedRshellConfigDir = originalConfigDir })

	originalEnv, envWasSet := os.LookupEnv(privilegedRshellEnabledEnv)
	require.NoError(t, os.Unsetenv(privilegedRshellEnabledEnv))
	t.Cleanup(func() {
		if envWasSet {
			require.NoError(t, os.Setenv(privilegedRshellEnabledEnv, originalEnv))
		} else {
			require.NoError(t, os.Unsetenv(privilegedRshellEnabledEnv))
		}
	})

	ctx := HookContext{Context: context.Background()}
	assert.False(t, privilegedRshellEnabled(ctx), "the helper must be disabled when config is absent")

	require.NoError(t, os.WriteFile(filepath.Join(configDir, "datadog.yaml"), []byte(`
private_action_runner:
  restricted_shell:
    privileged:
      enabled: true
`), 0640))
	assert.True(t, privilegedRshellEnabled(ctx))

	managedDir := filepath.Join(configDir, "managed", agentPackage, "stable")
	require.NoError(t, os.MkdirAll(managedDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(managedDir, "datadog.yaml"), []byte(`
private_action_runner:
  restricted_shell:
    privileged:
      enabled: false
`), 0640))
	assert.False(t, privilegedRshellEnabled(ctx), "managed config must override the base config")

	require.NoError(t, os.Setenv(privilegedRshellEnabledEnv, "true"))
	assert.True(t, privilegedRshellEnabled(ctx), "the environment must override file config")
}

func TestInstallableUnitsFiltersPrivilegedRshell(t *testing.T) {
	units := []string{
		"datadog-agent.service",
		"datadog-agent-rshell-privileged.service",
		privilegedRshellSocketStable,
		"datadog-agent-action.service",
	}

	ctx := HookContext{Context: context.Background(), PackagePath: t.TempDir()}
	assert.Equal(t, []string{"datadog-agent.service", "datadog-agent-action.service"}, installableUnits(ctx, units))
	assert.Len(t, units, 4, "filtering must not mutate the lifecycle cleanup list")
}

func TestExperimentStartUnits(t *testing.T) {
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
	t.Setenv(privilegedRshellEnabledEnv, "true")
	ctx := HookContext{Context: context.Background(), PackagePath: packagePath}
	assert.Equal(t,
		[]string{"datadog-agent-exp.service", privilegedRshellSocketExp},
		experimentStartUnits(ctx, "datadog-agent-exp.service"),
	)

	t.Setenv(privilegedRshellEnabledEnv, "false")
	assert.Equal(t,
		[]string{"datadog-agent-exp.service"},
		experimentStartUnits(ctx, "datadog-agent-exp.service"),
	)

	t.Setenv(privilegedRshellEnabledEnv, "true")
	getLandlockABIVersion = func() (int, error) { return 0, errors.New("landlock unavailable") }
	assert.Equal(t,
		[]string{"datadog-agent-exp.service"},
		experimentStartUnits(ctx, "datadog-agent-exp.service"),
	)
}
