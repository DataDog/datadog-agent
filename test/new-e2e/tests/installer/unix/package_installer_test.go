// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"strings"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type packageInstallerSuite struct {
	packageBaseSuite
}

func testInstaller(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageInstallerSuite{
		packageBaseSuite: newPackageSuite("installer", os, arch, method, awshost.WithoutFakeIntake()),
	}
}

func (s *packageInstallerSuite) TestInstall() {
	s.RunInstallScript("DD_NO_AGENT_INSTALL=true")
	defer s.Purge()

	bootstrapperVersion := s.host.BootstrapperVersion()
	installerVersion := s.host.InstallerVersion()
	assert.Equal(s.T(), bootstrapperVersion, installerVersion)

	state := s.host.State()
	state.AssertGroupExists("dd-agent")
	state.AssertUserExists("dd-agent")
	state.AssertUserHasGroup("dd-agent", "dd-agent")

	state.AssertDirExists("/etc/datadog-agent", 0755, "dd-agent", "dd-agent")
	state.AssertDirExists("/var/log/datadog", 0755, "dd-agent", "dd-agent")
	state.AssertDirExists("/opt/datadog-packages/run", 0755, "dd-agent", "dd-agent")

	state.AssertDirExists("/opt/datadog-installer", 0755, "root", "root")
	state.AssertDirExists("/opt/datadog-packages", 0755, "root", "root")
	state.AssertDirExists("/opt/datadog-packages/tmp", 0755, "dd-agent", "dd-agent")
	state.AssertDirExists("/opt/datadog-packages/run", 0755, "dd-agent", "dd-agent")
	state.AssertDirExists("/opt/datadog-packages/datadog-installer", 0755, "root", "root")

	state.AssertSymlinkExists("/usr/bin/datadog-bootstrap", "/opt/datadog-installer/bin/installer/installer", "root", "root")

	if s.installMethod != InstallMethodAnsible {
		// DD_NO_AGENT_INSTALL isn't supported on ansible, so the symlink is updated to the agent's installer
		state.AssertSymlinkExists("/usr/bin/datadog-installer", "/opt/datadog-packages/datadog-installer/stable/bin/installer/installer", "root", "root")
	}

	state.AssertUnitsLoaded("datadog-installer.service", "datadog-installer-exp.service")
	state.AssertUnitsEnabled("datadog-installer.service")
	state.AssertUnitsNotEnabled("datadog-installer-exp.service")
	state.AssertUnitsDead("datadog-installer-exp.service")
	var installerUnitState string
	assert.Eventually(s.T(), func() bool {
		state := s.host.State()
		unit, ok := state.Units["datadog-installer.service"]
		if !ok {
			installerUnitState = "not found"
			return false
		}
		if unit.SubState != host.Dead {
			installerUnitState = string(unit.SubState)
			return false
		}
		return true
	}, 60*time.Second, 1*time.Second, "datadog-installer.service should be dead but is %s", installerUnitState)
}

func (s *packageInstallerSuite) TestInstallWithRemoteUpdates() {
	if s.installMethod == InstallMethodAnsible {
		s.T().Skip("Ansible doesn't support installer in agent yet")
	}

	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(s.T(), "datadog-agent-installer.service")

	state := s.host.State()

	state.AssertUnitsLoaded("datadog-agent-installer.service")
	state.AssertUnitsRunning("datadog-agent-installer.service")

	state.AssertUnitsNotLoaded("datadog-agent-installer-exp.service") // Only loaded during experiment
}

func (s *packageInstallerSuite) TestUninstall() {
	s.RunInstallScript("DD_NO_AGENT_INSTALL=true")
	s.Purge()

	state := s.host.State()

	// state that never should get removed
	state.AssertGroupExists("dd-agent")
	state.AssertUserExists("dd-agent")
	state.AssertUserHasGroup("dd-agent", "dd-agent")

	state.AssertDirExists("/var/log/datadog", 0755, "dd-agent", "dd-agent")

	// state that should get removed
	state.AssertPathDoesNotExist("/opt/datadog-installer")
	state.AssertPathDoesNotExist("/opt/datadog-packages")

	state.AssertPathDoesNotExist("/usr/bin/datadog-bootstrap")
	state.AssertPathDoesNotExist("/usr/bin/datadog-installer")
}

func (s *packageInstallerSuite) TestReInstall() {
	s.RunInstallScript("DD_NO_AGENT_INSTALL=true")
	defer s.Purge()
	stateBefre := s.host.State()
	installerBinBefore, ok := stateBefre.Stat("/usr/bin/datadog-installer")
	assert.True(s.T(), ok)

	s.RunInstallScript("DD_NO_AGENT_INSTALL=true")
	stateAfter := s.host.State()
	installerBinAfter, ok := stateAfter.Stat("/usr/bin/datadog-installer")
	assert.True(s.T(), ok)

	assert.Equal(s.T(), installerBinBefore.ModTime, installerBinAfter.ModTime)
	s.host.AssertPackageInstalledByInstaller("datadog-installer")
}

func (s *packageInstallerSuite) TestUpdateInstallerOCI() {
	// Install prod
	err := s.RunInstallScriptProdOci(
		"DD_REMOTE_UPDATES=true",
		envForceVersion("datadog-installer", "7.58.0-installer-0.5.1-1"),
	)
	defer s.Purge()
	assert.NoError(s.T(), err)

	versionDisk := s.Env().RemoteHost.MustExecute("/opt/datadog-packages/datadog-installer/stable/bin/installer/installer version")
	assert.Equal(s.T(), "7.58.0-installer-0.5.1\n", versionDisk)
	assert.Eventually(s.T(), func() bool {
		versionRunning, err := s.Env().RemoteHost.Execute("sudo datadog-installer status")
		s.T().Logf("checking version: %s, err: %v", versionRunning, err)
		return err == nil && strings.Contains(versionRunning, "7.58.0-installer-0.5.1")
	}, 30*time.Second, 1*time.Second)

	// Install from QA registry
	err = s.RunInstallScriptWithError(
		"DD_REMOTE_UPDATES=true",
	)
	assert.NoError(s.T(), err)

	versionDisk = s.Env().RemoteHost.MustExecute("/opt/datadog-packages/datadog-agent/stable/embedded/bin/installer version")
	assert.NotEqual(s.T(), "7.58.0-installer-0.5.1\n", versionDisk)
	assert.Eventually(s.T(), func() bool {
		versionRunning, err := s.Env().RemoteHost.Execute("sudo datadog-installer status")
		s.T().Logf("checking version: %s, err: %v", versionRunning, err)
		return err == nil && !strings.Contains(versionRunning, "7.58.0-installer-0.5.1")
	}, 30*time.Second, 1*time.Second)
}

func (s *packageInstallerSuite) TestInstallWithUmask() {
	oldmask := s.host.SetUmask("0027")
	defer s.host.SetUmask(oldmask)
	s.TestInstall()
}
