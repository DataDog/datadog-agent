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
func (suite *testAgentInstallSuite) TestInstallAgentPackage() {
	suite.Run("Install", func() {
		// Arrange

		// Act
		output, err := suite.installer.InstallPackage(AgentPackage)

		// Assert
		suite.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
		suite.Require().Host(suite.Env().RemoteHost).HasARunningDatadogAgentService()
	})

	suite.Run("Uninstall", func() {
		// Arrange

		// Act
		output, err := suite.installer.RemovePackage(AgentPackage)

		// Assert
		suite.Require().NoErrorf(err, "failed to remove the Datadog Agent package: %s", output)
		suite.Require().Host(suite.Env().RemoteHost).HasNoDatadogAgentService()
	})
}

// TestUpgradeAgentPackage tests that it's possible to upgrade the Datadog Agent using the Datadog Installer.
func (suite *testAgentInstallSuite) TestUpgradeAgentPackage() {
	suite.Run("Install stable", func() {
		// Arrange

		// Act
		// public.ecr.aws/datadog/agent-package:7.55
		output, err := suite.installer.InstallPackage(AgentPackage,
			installer.WithRegistry("public.ecr.aws/datadog"),
			installer.WithVersion("7.55.1-1"),
			installer.WithAuthentication(""),
		)

		// Assert
		suite.Require().NoErrorf(err, "failed to install the stable Datadog Agent package: %s", output)
		suite.Require().Host(suite.Env().RemoteHost).
			HasARunningDatadogAgentService().
			WithVersionMatchPredicate(func(version string) {
				suite.Require().Contains(version, "Agent 7.55.1")
			}).
			DirExists(GetStableDirFor(AgentPackage))
	})

	suite.Run("Upgrade to latest using an experiment", func() {
		// Arrange

		// Act
		output, err := suite.installer.InstallExperiment(AgentPackage)

		// Assert
		suite.Require().NoErrorf(err, "failed to upgrade to the latest Datadog Agent package: %s", output)
		suite.Require().Host(suite.Env().RemoteHost).
			HasARunningDatadogAgentService().
			WithVersionMatchPredicate(func(version string) {
				suite.Require().NotContains(version, "7.55.1")
			}).
			DirExists(GetExperimentDirFOr(AgentPackage))
	})

	suite.Run("Stop experiment", func() {
		// Arrange

		// Act
		output, err := suite.installer.RemoveExperiment(AgentPackage)

		// Assert
		suite.Require().NoErrorf(err, "failed to remove the experiment for the Datadog Agent package: %s", output)

		// Remove experiment uninstalls the experimental version but also re-installs the stable version
		suite.Require().Host(suite.Env().RemoteHost).
			HasARunningDatadogAgentService().
			WithVersionMatchPredicate(func(version string) {
				suite.Require().Contains(version, "Agent 7.55.1")
			}).
			DirExists(GetStableDirFor(AgentPackage)).
			NoDirExists(GetExperimentDirFOr(AgentPackage))
	})
}
