// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"testing"
)

type testAgentUpgradeSuite struct {
	installerwindows.BaseInstallerSuite
}

// TestAgentUpgrades tests the usage of the Datadog installer to upgrade the Datadog Agent package.
func TestAgentUpgrades(t *testing.T) {
	e2e.Run(t, &testAgentUpgradeSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithInstaller(),
			)))
}

// TestUpgradeAgentPackage tests that it's possible to upgrade the Datadog Agent using the Datadog installer.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
	s.Run("Install stable", func() {
		s.installStableAgent()
		s.Run("Upgrade to latest using an experiment", func() {
			s.startLatestExperiment()
			s.Run("Stop experiment", s.stopExperiment)
		})
	})
}

func (s *testAgentUpgradeSuite) installStableAgent() {
	// Arrange

	// Act
	output, err := s.Installer().InstallPackage(installerwindows.AgentPackage,
		installer.WithRegistry("public.ecr.aws/datadog"),
		installer.WithVersion("latest"),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().NoErrorf(err, "failed to install the stable Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		}).
		DirExists(installerwindows.GetStableDirFor(installerwindows.AgentPackage))
}

func (s *testAgentUpgradeSuite) startLatestExperiment() {
	// Arrange

	// Act
	output, err := s.Installer().InstallExperiment(installerwindows.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to upgrade to the latest Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		}).
		DirExists(installerwindows.GetExperimentDirFor(installerwindows.AgentPackage))
}

func (s *testAgentUpgradeSuite) stopExperiment() {
	// Arrange

	// Act
	output, err := s.Installer().RemoveExperiment(installerwindows.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to remove the experiment for the Datadog Agent package: %s", output)

	// Remove experiment uninstalls the experimental version but also re-installs the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		}).
		DirExists(installerwindows.GetStableDirFor(installerwindows.AgentPackage)).
		NoDirExists(installerwindows.GetExperimentDirFor(installerwindows.AgentPackage))
}
