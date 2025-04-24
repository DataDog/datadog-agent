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

const (
	oldInstallerURL     = "http://install.datadoghq.com/datadog-installer-7.63.0-installer-0.12.5-1-x86_64.exe"
	oldInstallerScript  = "http://install.datadoghq.com/Install-Datadog-7.63.0-installer-0.12.5-1.ps1"
	oldInstallerVersion = "7.62"
	oldAgentVersion     = "7.63.2"
)

// baseInstallSuite contains the common test methods for both script and exe installation
type baseInstallSuite struct {
	installerwindows.BaseSuite
}

// testInstallScriptSuite is a test suite that uses the script installer
type testInstallScriptSuite struct {
	baseInstallSuite
}

// testInstallExeSuite is a test suite that uses the exe installer
type testInstallExeSuite struct {
	baseInstallSuite
}

// TestInstallScript tests the usage of the Datadog installer script to install the Datadog Agent package.
func TestInstallScript(t *testing.T) {
	e2e.Run(t, &testInstallScriptSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestInstallExe tests the usage of the Datadog installer exe to install the Datadog Agent package.
//
// e.g. datadog-installer setup --flavor default
func TestInstallExe(t *testing.T) {
	e2e.Run(t, &testInstallExeSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// BeforeTest sets up the test
func (s *testInstallScriptSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	s.SetInstallerImpl(installerwindows.NewDatadogInstallScript(s.Env(),
		installerwindows.WithLocalFileOverride("CURRENT_AGENT"),
	))
}

// BeforeTest sets up the test
func (s *testInstallExeSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	s.SetInstallerImpl(installerwindows.NewDatadogInstallExe(s.Env(),
		installerwindows.WithLocalFileOverride("CURRENT_AGENT"),
	))
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog installer.
func (s *baseInstallSuite) TestInstallAgentPackage() {
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
func (s *baseInstallSuite) TestInstallFromOldInstaller() {
	s.Run("Install from old installer", func() {
		s.installOldInstallerAndAgent()
		s.Run("Install New Version", func() {
			s.installCurrent()
		})
	})
}

// TestFailedUnsupportedVersion Test that version <65 fails to install
func (s *baseInstallSuite) TestFailedUnsupportedVersion() {
	s.Run("Install unsupported agent", func() {
		s.installUnsupportedAgent()
	})
}

func (s *baseInstallSuite) mustInstallVersion(versionPredicate string, opts ...installerwindows.PackageOption) {
	// Arrange
	packageConfig, err := installerwindows.NewPackageConfig(opts...)
	s.Require().NoError(err)
	fmt.Println("packageConfig", packageConfig)

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

func (s *baseInstallSuite) installPrevious() {
	s.mustInstallVersion(
		s.StableAgentVersion().Version(),
		installerwindows.WithPackage(s.StableAgentVersion().OCIPackage()),
	)
}

func (s *baseInstallSuite) installCurrent() {
	s.mustInstallVersion(
		s.CurrentAgentVersion().Version(),
		installerwindows.WithPackage(s.CurrentAgentVersion().OCIPackage()),
	)
}

func (s *baseInstallSuite) upgradeToLatestExperiment() {
	s.MustStartExperimentCurrentVersion()

	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

func (s *baseInstallSuite) installOldInstallerAndAgent() {
	// Arrange
	agentVersion := fmt.Sprintf("%s-1", oldAgentVersion)
	// Act
	opts := []installerwindows.Option{
		installerwindows.WithExtraEnvVars(map[string]string{
			// all of these make sure we install old versions from install.datadoghq.com
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_INSTALLER": oldInstallerVersion,
			"DD_INSTALLER_REGISTRY_URL_DATADOG_INSTALLER":        "install.datadoghq.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT":     agentVersion,
			"DD_INSTALLER_REGISTRY_URL_DATADOG_AGENT":            "install.datadoghq.com",
			"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":            "install.datadoghq.com",
			"DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE":        "install.datadoghq.com",
		}),
		installerwindows.WithInstallerURL(oldInstallerURL),
	}

	// Add script option only for script-based installation
	if _, ok := s.InstallScript().(*installerwindows.DatadogInstallScript); ok {
		opts = append(opts, installerwindows.WithInstallerScript(oldInstallerScript))
	}

	output, err := s.InstallScript().Run(opts...)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, oldAgentVersion)
		})
}

func (s *baseInstallSuite) installUnsupportedAgent() {
	// Arrange
	// Act
	_, err := s.InstallScript().Run(
		installerwindows.WithExtraEnvVars(map[string]string{
			// install pre 7.65 version
			"DD_AGENT_MAJOR_VERSION": "7",
			"DD_AGENT_MINOR_VERSION": "64.0",
		}),
	)

	// Assert that the installation failed
	s.Require().ErrorContains(err, "does not support fleet automation")
}
