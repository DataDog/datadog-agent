// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttests implements E2E tests for the Datadog Agent package on Windows
package agenttests

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"
)

type testAgentInstallSuite struct {
	installerwindows.BaseSuite
}

// TestAgentInstalls tests the usage of the Datadog installer to install the Datadog Agent package.
func TestAgentInstalls(t *testing.T) {
	e2e.Run(t, &testAgentInstallSuite{},
		e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()))
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog installer.
func (s *testAgentInstallSuite) TestInstallAgentPackage() {
	out, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(map[string]string{
		"DD_INSTALLER_DEFAULT_PKG_INSTALL_DATADOG_AGENT": "False",
	}))
	s.T().Log(out)
	s.Require().NoError(err)
	s.Run("Install", func() {
		s.installAgent()
		s.Run("Uninstall", s.removeAgentPackage)
	})

	//purge the installer
	_, err = s.Installer().Purge()
	s.Require().NoError(err)
}

func (s *testAgentInstallSuite) installAgent() {
	// Arrange

	// Act
	output, err := s.Installer().InstallPackage(consts.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
}

func (s *testAgentInstallSuite) removeAgentPackage() {
	// Arrange

	// Act
	output, err := s.Installer().RemovePackage(consts.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to remove the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasNoDatadogAgentService()
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(consts.GetStableDirFor(consts.AgentPackage),
			"the package directory should be removed")
}

func (s *testAgentInstallSuite) TestRemoveAgentAfterMSIUninstall() {
	// Arrange
	out, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(map[string]string{
		"DD_INSTALLER_DEFAULT_PKG_INSTALL_DATADOG_AGENT": "False",
	}))
	s.T().Log(out)
	s.Require().NoError(err)
	s.installAgent()
	s.uninstallAgentWithMSI()

	// Act

	// Assert
	s.removeAgentPackage()

	// remove the installer
	_, err = s.Installer().Purge()
	s.Require().NoError(err)
}

func (s *testAgentInstallSuite) uninstallAgentWithMSI() {
	// Arrange

	// Act
	err := windowsAgent.UninstallAgent(s.Env().RemoteHost,
		filepath.Join(s.SessionOutputDir(), "uninstall.log"),
	)

	// Assert
	s.Require().NoErrorf(err, "failed to uninstall the Datadog Agent package")
	s.Require().Host(s.Env().RemoteHost).
		DirExists(consts.GetStableDirFor(consts.AgentPackage),
			"the package directory should still exist after manually uninstalling the Agent with the MSI")
}
