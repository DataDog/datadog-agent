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
	assert.Equal(t, "/opt/datadog-agent/etc/managed", ConfigsPath)
	assert.Equal(t, "/opt/datadog-agent", DefaultUserConfigsDir)
	assert.Equal(t, "/opt/datadog-agent/etc", AgentConfigDir)
	assert.Equal(t, "/opt/datadog-agent/etc-exp", AgentConfigDirExp)
	assert.Equal(t, "/opt/datadog-agent/run", RunPath)
	assert.Equal(t, "/opt/datadog-agent/etc", DatadogDataDir)
	assert.Equal(t, "/opt/datadog-agent/embedded/bin/installer", StableInstallerPath)
	assert.Equal(t, "", DatadogProgramFilesDir)
}

// TestEverythingTheAgentOwnsIsUnderTheInstallRoot is the invariant behind the individual values:
// macOS has one root and nothing lives outside it, in particular nothing under /etc or /var.
func TestEverythingTheAgentOwnsIsUnderTheInstallRoot(t *testing.T) {
	const installRoot = "/opt/datadog-agent"

	for name, path := range map[string]string{
		"ConfigsPath":             ConfigsPath,
		"DefaultUserConfigsDir":   DefaultUserConfigsDir,
		"AgentConfigDir":          AgentConfigDir,
		"AgentConfigDirExp":       AgentConfigDirExp,
		"RunPath":                 RunPath,
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
