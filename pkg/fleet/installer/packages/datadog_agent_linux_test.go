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

// TestAgentPackagePermissionsRootOwnRootRunPaths checks that root-run binaries
// and their directories are root-owned, on top of the dd-agent default.
func TestAgentPackagePermissionsRootOwnRootRunPaths(t *testing.T) {
	ddAgentDefaultIndex := -1
	rootOwned := map[string]bool{}
	for i, p := range agentPackagePermissions {
		if p.Path == "." && p.Owner == "dd-agent" && p.Recursive {
			ddAgentDefaultIndex = i
		}
		if p.Owner == "root" && p.Group == "root" {
			rootOwned[p.Path] = true
			assert.Greater(t, i, ddAgentDefaultIndex,
				"root pin %q must be applied after the recursive dd-agent default", p.Path)
		}
	}
	require.GreaterOrEqual(t, ddAgentDefaultIndex, 0, "the package tree must default to dd-agent ownership")

	for _, path := range []string{
		".",
		"embedded",
		"embedded/bin",
		"embedded/bin/installer",
		"embedded/bin/privateactionrunner",
		"embedded/bin/rshell",
		"embedded/bin/system-probe",
		"embedded/bin/security-agent",
	} {
		assert.Truef(t, rootOwned[path], "%q is run by or contains a root binary and must be root-owned", path)
	}
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

func TestLegacyPrivilegedRshellSocketUnitsCleanedFromEveryInstallPath(t *testing.T) {
	// A DEB-to-OCI promotion runs as OCI but the stale socket unit sits under
	// the DEB path, so cleanup must cover every package type's unit directory.
	assert.ElementsMatch(t, installerFile.Paths{legacyPrivilegedRshellSocket, legacyPrivilegedRshellSocketExp}, legacyPrivilegedRshellSocketUnits)
	assert.ElementsMatch(t, []string{ociUnitsPath, debUnitsPath, rpmUnitsPath}, unitInstallPaths)

	for _, dir := range unitInstallPaths {
		root := t.TempDir()
		for _, unit := range legacyPrivilegedRshellSocketUnits {
			require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(root, dir, string(unit)), nil, 0644))
		}
		for _, unit := range legacyPrivilegedRshellSocketUnits {
			require.NoError(t, legacyPrivilegedRshellSocketUnits.EnsureAbsent(context.Background(), filepath.Join(root, dir)))
			_, err := os.Lstat(filepath.Join(root, dir, string(unit)))
			assert.ErrorIs(t, err, os.ErrNotExist, "%s under %s", unit, dir)
		}
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

// TestEnsureRootExecutablesRejectSymlink restores the regular-file validation
// the permission consolidation dropped: a symlinked root-executed binary must
// abort the install rather than have Permission.Ensure follow it and chown the
// target to root.
func TestEnsureRootExecutablesRejectSymlink(t *testing.T) {
	for _, rel := range rootExecutableRelPaths {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0755))
			require.NoError(t, os.Symlink("/tmp/attacker-controlled", filepath.Join(root, rel)))
			err := ensureRootExecutablesAreRegularFiles(HookContext{Context: context.Background(), PackagePath: root})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not a regular file")
		})
	}
}

// TestEnsureRootExecutablesAcceptRegularOrAbsent accepts regular files and
// tolerates a binary a given package flavor does not ship.
func TestEnsureRootExecutablesAcceptRegularOrAbsent(t *testing.T) {
	root := t.TempDir()
	// Only one of the two present: the other being absent must not error.
	rel := rootExecutableRelPaths[0]
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte("#!/bin/true\n"), 0755))
	assert.NoError(t, ensureRootExecutablesAreRegularFiles(HookContext{Context: context.Background(), PackagePath: root}))
}
