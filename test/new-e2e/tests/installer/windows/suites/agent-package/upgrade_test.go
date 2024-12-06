// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
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

// TestDowngradeAgentPackage tests that it's possible to downgrade the Datadog Agent using the Datadog installer.
func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
	// Arrange
	_, err := s.Installer().InstallPackage(installerwindows.AgentPackage)
	s.Require().NoErrorf(err, "failed to install the stable Datadog Agent package")

	// Act
	_, err = s.Installer().InstallExperiment(installerwindows.AgentPackage,
		installer.WithRegistry("install.datadoghq.com"),
		installer.WithVersion(s.StableAgentVersion().PackageVersion()),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().NoErrorf(err, "failed to downgrade to stable Datadog Agent package")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		}).
		DirExists(installerwindows.GetStableDirFor(installerwindows.AgentPackage))
}

func (s *testAgentUpgradeSuite) TestExperimentFailure() {
	// Arrange
	s.Run("Install stable", func() {
		s.installStableAgent()
	})

	// Act
	_, err := s.Installer().InstallExperiment(installerwindows.AgentPackage,
		installer.WithRegistry("install.datadoghq.com"),
		installer.WithVersion("unknown-version"),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().Error(err, "expected an error when trying to start an experiment with an unknown version")
	s.stopExperiment()
	// TODO: is this the same test as TestStopWithoutExperiment?
}

func (s *testAgentUpgradeSuite) TestExperimentCurrentVersion() {
	// Arrange
	s.Run("Install stable", func() {
		s.installStableAgent()
	})

	// Act
	_, err := s.Installer().InstallExperiment(installerwindows.AgentPackage,
		installer.WithRegistry("install.datadoghq.com"),
		installer.WithVersion(s.StableAgentVersion().PackageVersion()),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().Error(err, "expected an error when trying to start an experiment with the same version as the current one")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		}).
		DirExists(installerwindows.GetStableDirFor(installerwindows.AgentPackage))
}

func (s *testAgentUpgradeSuite) TestStopWithoutExperiment() {
	// Arrange
	s.Run("Install stable", func() {
		s.installStableAgent()
	})

	// Act

	// Assert
	s.stopExperiment()
	// TODO: Currently uninstalls stable then reinstalls stable. functional but a waste.
}

func (s *testAgentUpgradeSuite) installStableAgent() {
	// Arrange

	// Act
	output, err := s.Installer().InstallPackage(installerwindows.AgentPackage,
		installer.WithRegistry("install.datadoghq.com"),
		installer.WithVersion(s.StableAgentVersion().PackageVersion()),
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
		DirExists(installerwindows.GetStableDirFor(installerwindows.AgentPackage))
}
