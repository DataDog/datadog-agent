// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dotnettests

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
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
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", version.PackageVersion())),
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

// TestMSIThenRemoteUpgrade tests the dotnet library can be remotely upgraded from an Agent MSI installed version
func (s *testAgentMSIInstallsDotnetLibrary) TestMSIThenRemoteUpgrade() {
	defer s.cleanupAgentConfig()
	s.setAgentConfig()

	oldVersion := s.previousDotnetLibraryVersion
	newVersion := s.currentDotnetLibraryVersion

	// Install first version
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", oldVersion.PackageVersion())),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the expected version of the library is loaded
	s.assertSuccessfulPromoteExperiment(oldVersion.Version())
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, oldVersion.Version())

	// Start remote upgrade experiment
	_, err := s.startExperimentCurrentVersion()
	s.Require().NoError(err)
	s.assertSuccessfulStartExperiment(newVersion.Version())

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

	// Promote the experiment
	_, err = s.Installer().PromoteExperiment("datadog-apm-library-dotnet")
	s.Require().NoError(err)
	s.assertSuccessfulPromoteExperiment(newVersion.Version())
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
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", oldVersion.PackageVersion())),
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
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", newVersion.PackageVersion())),
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
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", version.PackageVersion())),
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
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", oldVersion.PackageVersion())),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Rollback during upgrade to the new version
	err := s.Installer().Install(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", newVersion.PackageVersion())),
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
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", version.PackageVersion())),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Uninstall the Agent
	err := s.Installer().Uninstall(
		installerwindows.WithMSILogFile("uninstall.log"),
	)
	s.Require().NoError(err)

	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the expected version of the library is loaded
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, version.Version())
}

func (s *testAgentMSIInstallsDotnetLibrary) setAgentConfig() {
	err := s.Env().RemoteHost.MkdirAll("C:\\ProgramData\\Datadog")
	s.Require().NoError(err)
	_, err = s.Env().RemoteHost.WriteFile(consts.ConfigPath, []byte(`
api_key: aaaaaaaaa
remote_updates: true
`))
	s.Require().NoError(err)
}

func (s *testAgentMSIInstallsDotnetLibrary) cleanupAgentConfig() {
	err := s.Env().RemoteHost.Remove(consts.ConfigPath)
	s.Require().NoError(err)
}

func (s *testAgentMSIInstallsDotnetLibrary) assertSuccessfulStartExperiment(version string) {
	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-apm-library-dotnet").
		WithExperimentVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		})
}

func (s *testAgentMSIInstallsDotnetLibrary) assertSuccessfulPromoteExperiment(version string) {
	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-apm-library-dotnet").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		}).
		WithExperimentVersionEqual("")
}

func (s *testAgentMSIInstallsDotnetLibrary) startExperimentCurrentVersion() (string, error) {
	return s.startExperimentWithCustomPackage(installerwindows.WithName("datadog-apm-library-dotnet"),
		installerwindows.WithAlias("apm-library-dotnet-package"),
		// TODO remove override once image is published in prod
		installerwindows.WithVersion(s.currentDotnetLibraryVersion.PackageVersion()),
		installerwindows.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithDevEnvOverrides("CURRENT_DOTNET_LIBRARY"),
	)
}

func (s *testAgentMSIInstallsDotnetLibrary) startExperimentWithCustomPackage(opts ...installerwindows.PackageOption) (string, error) {
	packageConfig, err := installerwindows.NewPackageConfig(opts...)
	s.Require().NoError(err)
	packageConfig, err = installerwindows.CreatePackageSourceIfLocal(s.Env().RemoteHost, packageConfig)
	s.Require().NoError(err)

	// Set catalog so daemon can find the package
	_, err = s.Installer().SetCatalog(installerwindows.Catalog{
		Packages: []installerwindows.PackageEntry{
			{
				Package: packageConfig.Name,
				Version: packageConfig.Version,
				URL:     packageConfig.URL(),
			},
		},
	})
	s.Require().NoError(err)
	return s.Installer().StartExperiment("datadog-apm-library-dotnet", packageConfig.Version)
}

func (s *testAgentMSIInstallsDotnetLibrary) installPreviousAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
		installerwindows.WithMSIArg(fmt.Sprintf("APIKEY=%s", s.getAPIKey())),
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
		installerwindows.WithMSIArg(fmt.Sprintf("APIKEY=%s", s.getAPIKey())),
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

func (s *testAgentMSIInstallsDotnetLibrary) getAPIKey() string {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if apiKey == "" || err != nil {
			apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
		}
	}
	return apiKey
}
