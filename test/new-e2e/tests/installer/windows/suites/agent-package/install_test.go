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
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithInstaller(),
			)))
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog installer.
func (s *testAgentInstallSuite) TestInstallAgentPackage() {
	s.Run("Install", func() {
		s.installAgent()
		s.Run("Uninstall", s.removeAgentPackage)
	})
}

// TestSetupScriptInstallInfo tests that the Fleet Automation setup script correctly writes install_info with the setup script tool.
func (s *testAgentInstallSuite) TestSetupScriptInstallInfo() {
	s.Run("Install via setup script", func() {
		s.installAgentViaSetupScript()
		s.Run("Verify install_info", s.verifySetupScriptInstallInfo)
		s.Run("Uninstall", s.uninstallAgentWithMSI)
	})
}

func (s *testAgentInstallSuite) installAgent() {
	// Arrange

	// Act
	output, err := s.Installer().InstallPackage(consts.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
}

func (s *testAgentInstallSuite) installAgentViaSetupScript() {
	installExe := installerwindows.NewDatadogInstallExe(s.Env())

	// Act - This calls the same path as Fleet Automation setup script: installer.exe setup --flavor default
	output, err := installExe.Run()

	// Assert
	s.Require().NoErrorf(err, "failed to install via setup script: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
}

func (s *testAgentInstallSuite) verifySetupScriptInstallInfo() {
	// Arrange
	installInfoPath := "C:\\ProgramData\\Datadog\\install_info"

	// Act
	installInfoContent, err := s.Env().RemoteHost.ReadFile(installInfoPath)

	// Assert
	s.Require().NoError(err, "should be able to read install_info file")
	s.Require().Contains(string(installInfoContent), "tool: installer",
		"install_info should contain the setup script installation method")
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
	s.installAgent()
	s.uninstallAgentWithMSI()

	// Act

	// Assert
	s.removeAgentPackage()
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
