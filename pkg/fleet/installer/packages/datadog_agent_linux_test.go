// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"os"
	"path/filepath"
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
		{Path: "embedded/bin/privateactionrunner", Owner: "root", Group: "root", Mode: 0755},
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

func TestPrivilegedRshellUnitIsAlwaysWritten(t *testing.T) {
	// The helper reads the configuration when it starts, so every systemd
	// install writes its unit instead of filtering on the configuration.
	for _, units := range [][]string{agentService.SystemdUnitsStable, agentService.ProcmgrUnitsStable} {
		assert.Contains(t, units, "datadog-agent-rshell-privileged.service")
		assert.NotContains(t, units, legacyPrivilegedRshellSocket, "the helper creates its own socket")
	}
	for _, units := range [][]string{agentService.SystemdUnitsExp, agentService.ProcmgrUnitsExp} {
		assert.Contains(t, units, "datadog-agent-rshell-privileged-exp.service")
		assert.NotContains(t, units, legacyPrivilegedRshellSocketExp, "the helper creates its own socket")
	}
}

func TestLegacyPrivilegedRshellSocketLinksAreRemoved(t *testing.T) {
	root := t.TempDir()
	for _, link := range legacyPrivilegedRshellSocketLinks {
		linkPath := filepath.Join(root, string(link))
		require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0755))
		// Dangling, as after a DEB to OCI migration removed the target.
		require.NoError(t, os.Symlink("/lib/systemd/system/"+legacyPrivilegedRshellSocket, linkPath))
	}
	unrelated := filepath.Join(root, "sockets.target.wants", "dbus.socket")
	require.NoError(t, os.Symlink("/lib/systemd/system/dbus.socket", unrelated))

	require.NoError(t, legacyPrivilegedRshellSocketLinks.EnsureAbsent(context.Background(), root))

	for _, link := range legacyPrivilegedRshellSocketLinks {
		_, err := os.Lstat(filepath.Join(root, string(link)))
		assert.ErrorIs(t, err, os.ErrNotExist, string(link))
	}
	_, err := os.Lstat(unrelated)
	assert.NoError(t, err, "unrelated socket links must be kept")
}
