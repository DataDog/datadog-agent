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

const (
	oldInstallerURL     = "http://dd-agent.s3.amazonaws.com/datadog-installer-7.63.0-installer-0.12.5-1-x86_64.exe"
	oldInstallerScript  = "http://dd-agent.s3.amazonaws.com/Install-Datadog-7.63.0-installer-0.12.5-1.ps1"
	oldInstallerVersion = "7.62"
	oldAgentVersion     = "7.63.2"
)

// TestInstallScript tests the usage of the Datadog installer script to install the Datadog Agent package.
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
			s.upgradeToLatestExperiment()
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

// TestInstallwithIIS tests that the installer install script properly installs the agent with IIS
func (s *testInstallScriptSuite) TestInstallwithIIS() {
	// Arrange
	// Act
	s.installCurrent(map[string]string{
		"DD_APM_INSTRUMENTATION_ENABLED":   "iis",
		"DD_APM_INSTRUMENTATION_LIBRARIES": "dotnet:3",
	})
}

// TestInstallIgnoreMajorMinor tests that the installer install script properly ignores
// the major / minor version when installing the agent
//
// This test replaces TestFailedUnsupportedVersion which used the major/minor parameters.
// These options were only used during the preview, we haven't documented them publicly since.
// Customers are now expected to download a per-version .exe file.
func (s *testInstallScriptSuite) TestInstallIgnoreMajorMinor() {
	// Arrange
	// Act
	_, err := s.InstallScript().Run(
		installerwindows.WithExtraEnvVars(map[string]string{
			// install pre 7.65 version
			"DD_AGENT_MAJOR_VERSION": "7",
			"DD_AGENT_MINOR_VERSION": "64.0",
		}),
	)

	// Assert
	// should ignore params and install current agent version
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

func (s *testInstallScriptSuite) mustInstallVersion(versionPredicate string, opts ...installerwindows.PackageOption) {
	s.mustInstallVersionWithEnvVars(versionPredicate, nil, opts...)
}

func (s *testInstallScriptSuite) mustInstallVersionWithEnvVars(versionPredicate string, extraEnvVars []map[string]string, opts ...installerwindows.PackageOption) {
	// Arrange
	packageConfig, err := installerwindows.NewPackageConfig(opts...)
	s.Require().NoError(err)

	// Merge default env vars with extra env vars
	envVars := map[string]string{
		"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT": packageConfig.Version,
		"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":        packageConfig.Registry,
	}

	// Add any extra environment variables
	for _, extraEnv := range extraEnvVars {
		for key, value := range extraEnv {
			envVars[key] = value
		}
	}

	// Act
	output, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(envVars))

	// Assert
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, versionPredicate)
		})
}

func (s *testInstallScriptSuite) installPrevious() {
	s.mustInstallVersion(
		s.StableAgentVersion().Version(),
		installerwindows.WithPackage(s.StableAgentVersion().OCIPackage()),
	)
}

func (s *testInstallScriptSuite) installCurrent(extraEnvVars ...map[string]string) {
	opts := []installerwindows.PackageOption{
		installerwindows.WithPackage(s.CurrentAgentVersion().OCIPackage()),
	}
	s.mustInstallVersionWithEnvVars(
		s.CurrentAgentVersion().Version(),
		extraEnvVars,
		opts...,
	)
}

func (s *testInstallScriptSuite) upgradeToLatestExperiment() {
	s.MustStartExperimentCurrentVersion()

	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

func (s *testInstallScriptSuite) installOldInstallerAndAgent() {
	// Arrange
	agentVersion := fmt.Sprintf("%s-1", oldAgentVersion)
	// Act
	opts := []installerwindows.Option{
		installerwindows.WithExtraEnvVars(map[string]string{
			// all of these make sure we install old versions from install.datadoghq.com
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_INSTALLER": oldInstallerVersion,
			"DD_INSTALLER_REGISTRY_URL_DATADOG_INSTALLER":        "dd-agent.s3.amazonaws.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT":     agentVersion,
			"DD_INSTALLER_REGISTRY_URL_DATADOG_AGENT":            "dd-agent.s3.amazonaws.com",
			"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":            "dd-agent.s3.amazonaws.com",
			"DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE":        "dd-agent.s3.amazonaws.com",
		}),
		installerwindows.WithInstallerURL(oldInstallerURL),
		installerwindows.WithInstallerScript(oldInstallerScript),
	}

	output, err := s.InstallScript().Run(opts...)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, oldAgentVersion)
		})
}

// TestInstallCurrentWithOldInstallerScript ensures the old installer script can install the current Agent version
//
// We're switching from .ps1 setup script that runs `bootstrap` subcommand to .exe itself. Some customers
// may have a cached or modified copy of the .ps1 script. This test ensures they can still install the current Agent version.
func (s *testInstallScriptSuite) TestInstallCurrentWithOldInstallerScript() {
	// Arrange
	packageConfig, err := installerwindows.NewPackageConfig(
		installerwindows.WithPackage(s.CurrentAgentVersion().OCIPackage()),
	)
	s.Require().NoError(err)

	// Act
	opts := []installerwindows.Option{
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT": packageConfig.Version,
			"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":        packageConfig.Registry,
		}),
		installerwindows.WithInstallerScript(oldInstallerScript),
	}

	output, err := s.InstallScript().Run(opts...)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}
