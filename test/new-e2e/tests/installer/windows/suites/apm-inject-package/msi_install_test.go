// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package injecttests

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"testing"
)

var (
	//go:embed resources/web.config
	webConfigFile []byte
	//go:embed resources/index.aspx
	aspxFile []byte
	//go:embed resources/DummyApp.java
	javaAppFile []byte
)

type testAgentMSIInstallsAPMInject struct {
	baseSuite
}

// TestAgentScriptInstallsAPMInject tests the usage of the install script to install the apm-inject package.
func TestAgentMSIInstallsAPMInject(t *testing.T) {
	e2e.Run(t, &testAgentMSIInstallsAPMInject{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testAgentMSIInstallsAPMInject) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseSuite.AfterTest(suiteName, testName)
}

// TestInstallFromMSI tests the Agent MSI can install the APM inject package with host instrumentation
func (s *testAgentMSIInstallsAPMInject) TestInstallFromMSI() {
	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		installerwindows.WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:3,java:1"),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	s.assertDriverInjections(true)
}

// TestEnableDisable tests that the enable and disable commands work
func (s *testAgentMSIInstallsAPMInject) TestEnableDisable() {
	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		installerwindows.WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:3,java:1"),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	s.assertDriverInjections(true)

	output, err := s.Env().RemoteHost.Execute(`&"C:\Program Files\Datadog\Datadog Agent\bin\scripts\host-instrumentation.bat" --disable`)
	s.Require().NoErrorf(err, "failed to run disable script: %s", output)

	s.assertDriverInjections(false)

	output, err = s.Env().RemoteHost.Execute(`&"C:\Program Files\Datadog\Datadog Agent\bin\scripts\host-instrumentation.bat" --enable`)
	s.Require().NoErrorf(err, "failed to run enable script: %s", output)

	s.assertDriverInjections(true)
}

// TestInstallFromMSIWithIIS tests the Agent MSI can install the APM inject package with IIS instrumentation
func (s *testAgentMSIInstallsAPMInject) TestInstallFromMSIWithIIS() {
	// Setup IIS
	iisHelper := installerwindows.NewIISHelper(s)
	iisHelper.SetupIIS()

	// Install with IIS instrumentation
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		installerwindows.WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:3"),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	// Start the IIS app to load the library
	defer iisHelper.StopIISApp()
	iisHelper.StartIISApp(webConfigFile, aspxFile)

	// Check that the .NET tracer is loaded
	libraryPath := iisHelper.GetLibraryPathFromInstrumentedIIS()
	s.Require().NotEmpty(libraryPath, "DD_DOTNET_TRACER_HOME should be set when instrumentation is enabled")
	s.Require().Contains(libraryPath, "datadog")
}

// TestInstallFromMSIWithJava tests the Agent MSI can install the APM inject package with Java instrumentation
func (s *testAgentMSIInstallsAPMInject) TestInstallFromMSIWithJava() {
	// Setup Java
	javaHelper := installerwindows.NewJavaHelper(s)
	javaHelper.SetupJava()

	// Install with Java instrumentation
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		installerwindows.WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=java:1"),
		installerwindows.WithMSILogFile("install.log"),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	// Start the Java app and check that the Datadog tracer is loaded
	output := javaHelper.StartJavaApp(javaAppFile)
	s.Require().Contains(output, "Datadog Tracer is available", "Datadog tracer should be loaded when instrumentation is enabled")
}

// installCurrentAgentVersionWithAPMInject installs the current agent version with APM inject via script
func (s *testAgentMSIInstallsAPMInject) installCurrentAgentVersion(opts ...installerwindows.MsiOption) {
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
