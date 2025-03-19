// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

type testInstallScriptSuite struct {
	installerwindows.BaseSuite
}

// TestAgentInstalls tests the usage of the Datadog installer to install the Datadog Agent package.
func TestInstallScript(t *testing.T) {
	e2e.Run(t, &testInstallScriptSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog installer.
func (s *testInstallScriptSuite) TestInstallAgentPackage() {
	s.Run("Fresh install", func() {
		s.installPrevious()
		s.Run("Install different Agent version", func() {
			s.UpgradeToLatestExperiment()
			s.Run("Reinstall last stable", func() {
				s.installPrevious()
			})
		})
	})
}

// TestInstallFromOldInstaller tests installing the Datadog Agent package from an old installer.
// shows we can correctly use the script to uninstall the old agent + installer MSIs
func (s *testInstallScriptSuite) TestInstallFromOldInstaller() {
	s.Run("Install from old installer", func() {
		s.installOldInstallerAndAgent()
		s.Run("Install New Version", func() {
			s.installCurrent()
		})
	})
}

func (s *testInstallScriptSuite) mustInstallScriptVersion(versionPredicate string, opts ...installerwindows.PackageOption) {
	// Arrange
	packageConfig, err := installerwindows.NewPackageConfig(opts...)
	s.Require().NoError(err)

	// Act
	output, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(map[string]string{
		"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT": packageConfig.Version,
		"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":        packageConfig.Registry,
	}))

	// Assert
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, versionPredicate)
		})

}

func (s *testInstallScriptSuite) installPrevious() {
	s.mustInstallScriptVersion(
		s.StableAgentVersion().Version(),
		// TODO: switch to prod stable entry when available
		installerwindows.WithPipeline("59254108"),
		installerwindows.WithDevEnvOverrides("PREVIOUS_AGENT"),
	)
}

func (s *testInstallScriptSuite) installCurrent() {
	s.mustInstallScriptVersion(
		s.CurrentAgentVersion().GetNumberAndPre(),
		installerwindows.WithPipeline(s.Env().Environment.PipelineID()),
		installerwindows.WithDevEnvOverrides("CURRENT_AGENT"),
	)
}

func (s *testInstallScriptSuite) UpgradeToLatestExperiment() {
	s.MustStartExperimentCurrentVersion()

	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())
}

func (s *testInstallScriptSuite) installOldInstallerAndAgent() {
	// Arrange
	// Act
	output, err := s.InstallScript().Run(
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_INSTALLER": "7.62",
			"DD_INSTALLER_REGISTRY_URL_DATADOG_INSTALLER":        "install.datadoghq.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT":     "7.63.2-1",
			"DD_INSTALLER_REGISTRY_URL_DATADOG_AGENT":            "install.datadoghq.com",
			"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":            "install.datadoghq.com",
			// make sure to install the agent
			"DD_INSTALLER_DEFAULT_PKG_INSTALL_DATADOG_AGENT": "true",
		}),
	)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, "7.63.2-1")
		})
}
