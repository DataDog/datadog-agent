// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dotnettests

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"

	"testing"
)

type testAgentScriptInstallsDotnetLibrary struct {
	baseIISSuite
	previousDotnetLibraryVersion installerwindows.PackageVersion
	currentDotnetLibraryVersion  installerwindows.PackageVersion
}

// TestDotnetInstalls tests the usage of the Datadog installer and the MSI to install the apm-library-dotnet-package package.
func TestAgentScriptInstallsDotnetLibrary(t *testing.T) {
	e2e.Run(t, &testAgentScriptInstallsDotnetLibrary{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testAgentScriptInstallsDotnetLibrary) SetupSuite() {
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

func (s *testAgentScriptInstallsDotnetLibrary) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseIISSuite.AfterTest(suiteName, testName)
}

// TestInstallFromScript tests the Agent script can install the dotnet library OCI package
func (s *testAgentScriptInstallsDotnetLibrary) TestInstallFromScript() {
	// Arrange
	version := s.currentDotnetLibraryVersion

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED": "iis",
			// TODO: remove override once image is published in prod
			"DD_INSTALLER_REGISTRY_URL":        "install.datad0g.com.internal.dda-testing.com",
			"DD_APM_INSTRUMENTATION_LIBRARIES": "dotnet:" + version.Version(),
		}),
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

// installCurrentAgentVersion installs the current agent version with script
func (s *testAgentScriptInstallsDotnetLibrary) installCurrentAgentVersion(opts ...installerwindows.Option) {
	output, err := s.InstallScript().Run(opts...)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}
