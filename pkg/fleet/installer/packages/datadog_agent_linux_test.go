// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
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
	assert.Contains(t, agentDirectories, installerFile.Directory{
		Path:  privilegedRshellPolicyDir,
		Mode:  0755,
		Owner: "root",
		Group: "root",
	})
	policyPermission := installerFile.Permission{
		Path:      "rshell.d",
		Owner:     "root",
		Group:     "root",
		Recursive: true,
	}
	assert.Contains(t, agentConfigPermissions, policyPermission)

	basePermissionIndex := -1
	policyPermissionIndex := -1
	for i, permission := range agentConfigPermissions {
		if permission.Path == "." && permission.Recursive {
			basePermissionIndex = i
		}
		if permission == policyPermission {
			policyPermissionIndex = i
		}
	}
	require.NotEqual(t, -1, basePermissionIndex)
	require.NotEqual(t, -1, policyPermissionIndex)
	assert.Less(t, basePermissionIndex, policyPermissionIndex, "root ownership must be restored after the recursive dd-agent ownership pass")
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
