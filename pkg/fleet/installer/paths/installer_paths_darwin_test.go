// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package paths

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDarwinPathsMatchTheLayout pins every constant to the macOS layout. The Linux values these
// were split away from are all FHS paths that happen to exist on macOS, so an edit that
// reintroduced one would compile, run, and only fail on a real machine.
func TestDarwinPathsMatchTheLayout(t *testing.T) {
	assert.Equal(t, "/opt/datadog-packages", PackagesPath)
	assert.Equal(t, "/opt/datadog-agent/etc/managed", ConfigsPath)
	assert.Equal(t, "/opt/datadog-packages/tmp", RootTmpDir)
	assert.Equal(t, "/opt/datadog-agent", DefaultUserConfigsDir)
	assert.Equal(t, "/opt/datadog-agent/etc", AgentConfigDir)
	assert.Equal(t, "/opt/datadog-agent/etc-exp", AgentConfigDirExp)
	assert.Equal(t, "/opt/datadog-agent/run", RunPath)
	assert.Equal(t, "/opt/datadog-agent/etc", DatadogDataDir)
	assert.Equal(t, "/opt/datadog-packages/datadog-agent/stable/embedded/bin/installer", StableInstallerPath)
	assert.Equal(t, "/opt/datadog-packages/datadog-agent/experiment/embedded/bin/installer", ExperimentInstallerPath)
	assert.Equal(t, "", DatadogProgramFilesDir)
}

// TestNoPathEscapesTheTwoRoots is the invariant behind the individual values: a directory holds
// either state, under /opt/datadog-agent, or code, under /opt/datadog-packages. Nothing lives
// anywhere else, and in particular nothing lives under /etc or /var.
func TestNoPathEscapesTheTwoRoots(t *testing.T) {
	for name, path := range map[string]string{
		"PackagesPath":            PackagesPath,
		"ConfigsPath":             ConfigsPath,
		"RootTmpDir":              RootTmpDir,
		"DefaultUserConfigsDir":   DefaultUserConfigsDir,
		"AgentConfigDir":          AgentConfigDir,
		"AgentConfigDirExp":       AgentConfigDirExp,
		"RunPath":                 RunPath,
		"DatadogDataDir":          DatadogDataDir,
		"StableInstallerPath":     StableInstallerPath,
		"ExperimentInstallerPath": ExperimentInstallerPath,
	} {
		rooted := strings.HasPrefix(path, "/opt/datadog-agent") || strings.HasPrefix(path, "/opt/datadog-packages")
		assert.True(t, rooted, "%s is %q, which is rooted at neither the state root nor the pool", name, path)
	}
}

// TestStateAndCodeDoNotMix guards the organizing rule directly: the state paths must not resolve
// into the pool, and the pool paths must not resolve into the state root.
func TestStateAndCodeDoNotMix(t *testing.T) {
	const stateRoot = "/opt/datadog-agent"

	for name, path := range map[string]string{
		"ConfigsPath":       ConfigsPath,
		"AgentConfigDir":    AgentConfigDir,
		"AgentConfigDirExp": AgentConfigDirExp,
		"RunPath":           RunPath,
		"DatadogDataDir":    DatadogDataDir,
	} {
		assert.True(t, strings.HasPrefix(path, stateRoot+"/"), "%s (%q) is not under the state root", name, path)
		assert.NotContains(t, path, PackagesPath, "%s (%q) mixes state into the pool", name, path)
	}

	for name, path := range map[string]string{
		"RootTmpDir":              RootTmpDir,
		"StableInstallerPath":     StableInstallerPath,
		"ExperimentInstallerPath": ExperimentInstallerPath,
	} {
		assert.True(t, strings.HasPrefix(path, PackagesPath+"/"), "%s (%q) is not under the pool", name, path)
	}

	// The experiment configuration directory is a sibling of the stable one, not a child: the
	// configuration copy, the recursive ownership pass over etc and the first-install
	// save-and-restore must all be able to leave it alone.
	assert.Equal(t, AgentConfigDir+"-exp", AgentConfigDirExp)
	assert.NotContains(t, AgentConfigDirExp, AgentConfigDir+"/")

	// The pool is addressed only through its links, never through a version.
	assert.Contains(t, StableInstallerPath, "/stable/")
	assert.Contains(t, ExperimentInstallerPath, "/experiment/")
}
