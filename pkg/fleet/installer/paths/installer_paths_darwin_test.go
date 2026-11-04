// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDarwinPathsMatchTheLayout pins every constant to the macOS layout. The Linux values these
// were split away from are all FHS paths that happen to exist on macOS, so an edit that
// reintroduced one would compile, run, and only fail on a real machine.
func TestDarwinPathsMatchTheLayout(t *testing.T) {
	assert.Equal(t, "", ConfigsPath)
	assert.Equal(t, "/opt/datadog-agent", DefaultUserConfigsDir)
	assert.Equal(t, "/opt/datadog-agent/etc", AgentConfigDir)
	assert.Equal(t, "/opt/datadog-agent/etc-exp", AgentConfigDirExp)
	assert.Equal(t, "/opt/datadog-packages/run", RunPath)
	assert.Equal(t, "/opt/datadog-agent/etc", DatadogDataDir)
	assert.Equal(t, "/opt/datadog-agent/embedded/bin/installer", StableInstallerPath)
	assert.Equal(t, "", DatadogProgramFilesDir)
}

// TestEverythingTheAgentOwnsIsUnderTheInstallRoot is the invariant behind the individual values:
// macOS has one root and nothing lives outside it, in particular nothing under /etc or /var.
func TestEverythingTheAgentOwnsIsUnderTheInstallRoot(t *testing.T) {
	const installRoot = "/opt/datadog-agent"

	for name, path := range map[string]string{
		"DefaultUserConfigsDir":   DefaultUserConfigsDir,
		"AgentConfigDir":          AgentConfigDir,
		"AgentConfigDirExp":       AgentConfigDirExp,
		"DatadogDataDir":          DatadogDataDir,
		"StableInstallerPath":     StableInstallerPath,
		"ExperimentInstallerPath": ExperimentInstallerPath,
	} {
		assert.True(t, strings.HasPrefix(path, installRoot), "%s is %q, which is outside the install root", name, path)
		assert.NotContains(t, path, PackagesPath, "%s (%q) reaches into a package pool macOS does not have", name, path)
	}
}

// TestTheExperimentConfigDirIsASibling guards the one relationship the configuration experiment
// depends on: etc-exp sits beside etc rather than inside it, so the configuration copy, the
// recursive ownership pass over etc and the first-install save-and-restore can leave it alone.
func TestTheExperimentConfigDirIsASibling(t *testing.T) {
	assert.Equal(t, AgentConfigDir+"-exp", AgentConfigDirExp)
	assert.NotContains(t, AgentConfigDirExp, AgentConfigDir+"/")
}

// TestThereIsOnlyOneInstallerBinary pins that macOS runs no version experiment: the installer the
// daemon execs is the installer that ships in the package, at one address forever.
func TestThereIsOnlyOneInstallerBinary(t *testing.T) {
	assert.Equal(t, StableInstallerPath, ExperimentInstallerPath)
}

// TestTheInstallerRunPathIsOutsideTheAgentsTree pins that the root daemon's state is not kept
// anywhere under the install root, which the Agent's account can write to.
func TestTheInstallerRunPathIsOutsideTheAgentsTree(t *testing.T) {
	assert.False(t, strings.HasPrefix(RunPath, DefaultUserConfigsDir+"/"))
	assert.True(t, strings.HasPrefix(RunPath, PackagesPath+"/"))
	assert.False(t, strings.HasPrefix(InstallerLogPath, DefaultUserConfigsDir+"/"))
	assert.True(t, strings.HasPrefix(InstallerLogPath, PackagesPath+"/"))
}

func testOwner() dirOwner {
	return dirOwner{uid: os.Getuid(), gid: -1}
}

func TestEnsureTrustedDirCreatesAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	require.NoError(t, ensureTrustedDir(dir, testOwner()))

	info, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestEnsureTrustedDirRepairsAWritableDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	require.NoError(t, os.Mkdir(dir, 0755))
	require.NoError(t, os.Chmod(dir, 0777))

	require.NoError(t, ensureTrustedDir(dir, testOwner()))

	info, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestEnsureTrustedDirRefusesASymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	require.NoError(t, os.Mkdir(target, 0755))
	link := filepath.Join(root, "run")
	require.NoError(t, os.Symlink(target, link))

	assert.Error(t, ensureTrustedDir(link, testOwner()))
}

func TestEnsureTrustedDirRefusesADirectoryItCannotRepair(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can hand a directory to any account")
	}
	dir := filepath.Join(t.TempDir(), "run")
	require.NoError(t, os.Mkdir(dir, 0755))

	// Owned by the test's account, so a different expected owner cannot be enforced unprivileged.
	assert.Error(t, ensureTrustedDir(dir, dirOwner{uid: os.Getuid() + 1, gid: -1}))
}
