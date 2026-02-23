// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dotnettests

import (
	_ "embed"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"testing"
)

type testAgentMSIInstallsDotnetLibrary struct {
	baseIISSuite
	previousDotnetLibraryVersion installerwindows.PackageVersion
	currentDotnetLibraryVersion  installerwindows.PackageVersion
}

// TestDotnetInstalls tests the usage of the Datadog installer and the MSI to install the apm-library-dotnet-package package.
func TestAgentMSIInstallsDotnetLibrary(t *testing.T) {
	e2e.Run(t, &testAgentMSIInstallsDotnetLibrary{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testAgentMSIInstallsDotnetLibrary) SetupSuite() {
	s.baseIISSuite.SetupSuite()

	s.previousDotnetLibraryVersion = installerwindows.NewVersionFromPackageVersion(os.Getenv("PREVIOUS_DOTNET_VERSION_PACKAGE"))
	if s.previousDotnetLibraryVersion.PackageVersion() == "" {
		s.previousDotnetLibraryVersion = installerwindows.NewVersionFromPackageVersion("3.19.0-pipeline.67299728.beta.sha-c05ddfb1-1")
	}
	s.currentDotnetLibraryVersion = installerwindows.NewVersionFromPackageVersion(os.Getenv("CURRENT_DOTNET_VERSION_PACKAGE"))
	if s.currentDotnetLibraryVersion.PackageVersion() == "" {
		s.currentDotnetLibraryVersion = installerwindows.NewVersionFromPackageVersion("3.19.0-pipeline.67351320.beta.sha-c05ddfb1-1")
	}
}

func (s *testAgentMSIInstallsDotnetLibrary) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseIISSuite.AfterTest(suiteName, testName)
}

// TestInstallFromMSI tests the Agent MSI can install the dotnet library OCI package
func (s *testAgentMSIInstallsDotnetLibrary) TestInstallFromMSI() {
	// Arrange
	version := s.currentDotnetLibraryVersion

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)
	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Assert
	s.assertSuccessfulPromoteExperiment(version.Version())
	// Check that the expected version of the library is loaded
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, version.Version())
}

// TestUpgradeWithMSI tests the dotnet library can be upgraded from the MSI
func (s *testAgentMSIInstallsDotnetLibrary) TestUpgradeWithMSI() {
	flake.Mark(s.T())
	oldVersion := s.previousDotnetLibraryVersion
	newVersion := s.currentDotnetLibraryVersion

	// Install first version
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+oldVersion.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the expected version of the library is loaded
	s.assertSuccessfulPromoteExperiment(oldVersion.Version())
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, oldVersion.Version())

	// Install the second version using a new Agent MSI
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+newVersion.Version()),
		installerwindows.WithMSILogFile("upgrade.log"),
	)

	// Check that the new version became the stable version
	s.assertSuccessfulPromoteExperiment(newVersion.Version())

	// Check that the old version of the library is still loaded since we have not restarted yet
	oldLibraryPathAgain := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPathAgain, oldVersion.Version())
	s.Require().Equal(oldLibraryPath, oldLibraryPathAgain)

	// Restart the IIS application
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the new version of the library is loaded
	newLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(newLibraryPath, newVersion.Version())
	s.Require().NotEqual(oldLibraryPath, newLibraryPath)
}

// TestMSIRollbackRemovesLibrary tests that the dotnet library is removed when the MSI installation fails
func (s *testAgentMSIInstallsDotnetLibrary) TestMSIRollbackRemovesLibrary() {
	// Arrange
	version := s.currentDotnetLibraryVersion

	// Act
	err := s.Installer().Install(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install-rollback.log"),
		installerwindows.WithMSIArg("WIXFAILWHENDEFERRED=1"),
	)
	s.Require().Error(err)

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(`C:/ProgramData/Datadog/Installer/packages/datadog-apm-library-dotnet`)
}

// TestMSISkipRollbackIfInstalled tests that the newly installed dotnet library is not removed when the MSI upgrade fails
// if another version of the library was already installed.
func (s *testAgentMSIInstallsDotnetLibrary) TestMSISkipRollbackIfInstalled() {
	// Arrange
	oldVersion := s.previousDotnetLibraryVersion
	newVersion := s.currentDotnetLibraryVersion

	// Install first version
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+oldVersion.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Rollback during upgrade to the new version
	err := s.Installer().Install(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+newVersion.Version()),
		installerwindows.WithMSILogFile("install-rollback.log"),
		installerwindows.WithMSIArg("WIXFAILWHENDEFERRED=1"),
	)
	s.Require().Error(err)

	// Start IIS and check that the NEW version of the library is loaded
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
	newLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(newLibraryPath, newVersion.Version())
}

// TestUninstallKeepsLibrary tests that the dotnet library is not removed when the Agent MSI is uninstalled
func (s *testAgentMSIInstallsDotnetLibrary) TestUninstallKeepsLibrary() {
	version := s.currentDotnetLibraryVersion

	// Install the dotnet library with the Agent MSI
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Uninstall the Agent
	err := s.Installer().Uninstall(
		installerwindows.WithMSILogFile("uninstall.log"),
		installerwindows.WithMSIArg("KEEP_INSTALLED_PACKAGES=1"),
	)
	s.Require().NoError(err)

	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the expected version of the library is loaded
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, version.Version())
}

// TestUninstallScript validates that instrumentation is disabled after we run the uninstall script
func (s *testAgentMSIInstallsDotnetLibrary) TestUninstallScript() {
	// Arrange
	version := s.currentDotnetLibraryVersion

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)
	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Assert
	s.assertSuccessfulPromoteExperiment(version.Version())
	// Check that the expected version of the library is loaded
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, version.Version())

	output, err := s.Env().RemoteHost.Execute(`&"C:\Program Files\Datadog\Datadog Agent\bin\scripts\iis-instrumentation.bat" --uninstall`)
	s.Require().NoErrorf(err, "failed to run uninstall script: %s", output)

	s.stopIISApp()
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
	newLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Empty(newLibraryPath)
}

// TestUninstallScript validates that instrumentation is disabled after we run the uninstall script
func (s *testAgentMSIInstallsDotnetLibrary) TestMSIPurge() {
	// Arrange
	version := s.currentDotnetLibraryVersion

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)
	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Assert
	s.assertSuccessfulPromoteExperiment(version.Version())
	// Check that the expected version of the library is loaded
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, version.Version())

	// uninstall the MSI, it will run purge by default
	options := []installerwindows.MsiOption{
		installerwindows.WithMSILogFile("uninstall.log"),
	}
	s.Require().NoError(s.Installer().Uninstall(options...))

	// verify it is uninstalled
	s.stopIISApp()
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
	newLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Empty(newLibraryPath)
}

func (s *testAgentMSIInstallsDotnetLibrary) TestMSIPurgeDisabled() {
	// Arrange
	version := s.currentDotnetLibraryVersion

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)
	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Assert
	s.assertSuccessfulPromoteExperiment(version.Version())
	// Check that the expected version of the library is loaded
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, version.Version())

	// uninstall the MSI with KEEP_INSTALLED_PACKAGES=1 (equivalent to PURGE=0)
	options := []installerwindows.MsiOption{
		installerwindows.WithMSILogFile("uninstall.log"),
		installerwindows.WithMSIArg("KEEP_INSTALLED_PACKAGES=1"),
	}
	s.Require().NoError(s.Installer().Uninstall(options...))

	// verify it is installed
	// Restart the IIS application
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the version of the library is still loaded
	newLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(newLibraryPath, version.Version())
}

// TestDisableEnableScript validates that the enable and disable commands work
func (s *testAgentMSIInstallsDotnetLibrary) TestDisableEnableScript() {
	// Arrange
	version := s.currentDotnetLibraryVersion

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install.log"),
	)
	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Assert
	s.assertSuccessfulPromoteExperiment(version.Version())
	// Check that the expected version of the library is loaded
	libraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(libraryPath, version.Version())

	output, err := s.Env().RemoteHost.Execute(`&"C:\Program Files\Datadog\Datadog Agent\bin\scripts\iis-instrumentation.bat" --disable`)
	s.Require().NoErrorf(err, "failed to run uninstall script: %s", output)

	s.stopIISApp()
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
	libraryPath = s.getLibraryPathFromInstrumentedIIS()
	s.Require().Empty(libraryPath)

	output, err = s.Env().RemoteHost.Execute(`&"C:\Program Files\Datadog\Datadog Agent\bin\scripts\iis-instrumentation.bat" --enable`)
	s.Require().NoErrorf(err, "failed to run uninstall script: %s", output)

	s.stopIISApp()
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
	libraryPath = s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(libraryPath, version.Version())
}

// TestEnableSSIOnReinstall tests that SSI can be enabled when reinstalling the same version
// of the agent after an initial installation without SSI enabled
func (s *testAgentMSIInstallsDotnetLibrary) TestEnableSSIOnReinstall() {
	version := s.currentDotnetLibraryVersion

	// First install: Install agent without SSI enabled
	s.installCurrentAgentVersion(
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSILogFile("install-no-ssi.log"),
	)

	// Start IIS and verify instrumentation is not active
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
	libraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Empty(libraryPath, "library should not be loaded when SSI is not enabled")

	// Second install: Reinstall with SSI enabled
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:"+version.Version()),
		installerwindows.WithMSILogFile("install-with-ssi.log"),
	)

	// Verify the library package is still at the expected version
	s.assertSuccessfulPromoteExperiment(version.Version())

	// Restart IIS and verify instrumentation is now active
	s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
	libraryPath = s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(libraryPath, version.Version(), "library should be loaded when SSI is enabled")
}

func (s *testAgentMSIInstallsDotnetLibrary) installPreviousAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
		installerwindows.WithMSIArg("APIKEY=" + installer.GetAPIKey()),
		installerwindows.WithMSIArg("SITE=datadoghq.com"),
	}
	options = append(options, opts...)
	s.Require().NoError(s.Installer().Install(options...))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentMSIInstallsDotnetLibrary) installCurrentAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.CurrentAgentVersion().Version()

	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-current-version.log"),
		installerwindows.WithMSIArg("APIKEY=" + installer.GetAPIKey()),
		installerwindows.WithMSIArg("SITE=datadoghq.com"),
	}
	options = append(options, opts...)
	s.Require().NoError(s.Installer().Install(
		options...,
	))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}
