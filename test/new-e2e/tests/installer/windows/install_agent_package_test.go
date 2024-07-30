// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	"testing"
)

type testAgentInstallSuite struct {
	baseSuite
}

// TestAgentInstalls tests the usage of the Datadog Installer to install the Datadog Agent package.
func TestAgentInstalls(t *testing.T) {
	e2e.Run(t, &testAgentInstallSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithInstaller(),
			)))
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog Installer.
func (s *testAgentInstallSuite) TestInstallAgentPackage() {
	s.Run("Install", func() {
		s.installAgent()
		s.Run("Uninstall", s.uninstallAgent)
	})
}

func (s *testAgentInstallSuite) installAgent() {
	// Arrange

	// Act
	output, err := s.installer.InstallPackage(AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
}

func (s *testAgentInstallSuite) uninstallAgent() {
	// Arrange

	// Act
	output, err := s.installer.RemovePackage(AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to remove the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasNoDatadogAgentService()
}

// TestUpgradeAgentPackage tests that it's possible to upgrade the Datadog Agent using the Datadog Installer.
func (s *testAgentInstallSuite) TestUpgradeAgentPackage() {
	s.Run("Install stable", func() {
		s.installStableAgent()
		s.Run("Upgrade to latest using an experiment", func() {
			s.startLatestExperiment()
			s.Run("Stop experiment", s.stopExperiment)
		})
	})
}

func (s *testAgentInstallSuite) installStableAgent() {
	// Arrange

	// Act
	output, err := s.installer.InstallPackage(AgentPackage,
		installer.WithRegistry("public.ecr.aws/datadog"),
		installer.WithVersion("latest"),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().NoErrorf(err, "failed to install the stable Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, "Agent 7.55.1")
		}).
		DirExists(GetStableDirFor(AgentPackage))
}

func (s *testAgentInstallSuite) startLatestExperiment() {
	// Arrange

	// Act
	output, err := s.installer.InstallExperiment(AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to upgrade to the latest Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().NotContains(version, "7.55.1")
		}).
		DirExists(GetExperimentDirFor(AgentPackage))
}

func (s *testAgentInstallSuite) stopExperiment() {
	// Arrange

	// Act
	output, err := s.installer.RemoveExperiment(AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to remove the experiment for the Datadog Agent package: %s", output)

	// Remove experiment uninstalls the experimental version but also re-installs the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, "Agent 7.55.1")
		}).
		DirExists(GetStableDirFor(AgentPackage)).
		NoDirExists(GetExperimentDirFor(AgentPackage))
}
