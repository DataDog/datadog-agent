// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttests implements E2E tests for the Datadog Agent package on Windows
package agenttests

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"testing"
)

type testAgentInstallSuite struct {
	installerwindows.BaseInstallerSuite
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
		s.Run("Uninstall", s.uninstallAgent)
	})
}

func (s *testAgentInstallSuite) installAgent() {
	// Arrange

	// Act
	output, err := s.Installer().InstallPackage(installerwindows.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
}

func (s *testAgentInstallSuite) uninstallAgent() {
	// Arrange

	// Act
	output, err := s.Installer().RemovePackage(installerwindows.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to remove the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasNoDatadogAgentService()
}
