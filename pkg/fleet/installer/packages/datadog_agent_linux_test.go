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

func TestInstallableUnitsFiltersPrivilegedRshell(t *testing.T) {
	units := []string{
		"datadog-agent.service",
		"datadog-agent-rshell-privileged.service",
		privilegedRshellSocketStable,
		"datadog-agent-action.service",
	}

	assert.Equal(t, []string{"datadog-agent.service", "datadog-agent-action.service"}, installableUnits(t.TempDir(), units))
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
	assert.Equal(t,
		[]string{"datadog-agent-exp.service", privilegedRshellSocketExp},
		experimentStartUnits(packagePath, "datadog-agent-exp.service"),
	)

	getLandlockABIVersion = func() (int, error) { return 0, errors.New("landlock unavailable") }
	assert.Equal(t,
		[]string{"datadog-agent-exp.service"},
		experimentStartUnits(packagePath, "datadog-agent-exp.service"),
	)
}
