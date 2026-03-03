// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	unixinstaller "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"testing"
)

var (
	//go:embed resources/apm-inject/web.config
	apmInjectWebConfigFile []byte
	//go:embed resources/apm-inject/index.aspx
	apmInjectAspxFile []byte
	//go:embed resources/apm-inject/DummyApp.java
	apmInjectJavaAppFile []byte
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
		WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:3,java:1"),
		WithMSILogFile("install.log"),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	s.assertDriverInjections(true)
}

// TestEnableDisable tests that the enable and disable commands work
func (s *testAgentMSIInstallsAPMInject) TestEnableDisable() {
	// Act
	s.installCurrentAgentVersion(
		WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:3,java:1"),
		WithMSILogFile("install.log"),
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
	iisHelper := NewIISHelper(s)
	iisHelper.SetupIIS()

	// Install with IIS instrumentation
	s.installCurrentAgentVersion(
		WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:3"),
		WithMSILogFile("install.log"),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	// Start the IIS app to load the library
	defer iisHelper.StopIISApp()
	iisHelper.StartIISApp(apmInjectWebConfigFile, apmInjectAspxFile)

	// Check that the .NET tracer is loaded
	libraryPath := iisHelper.GetLibraryPathFromInstrumentedIIS()
	s.Require().NotEmpty(libraryPath, "DD_DOTNET_TRACER_HOME should be set when instrumentation is enabled")
	s.Require().Contains(libraryPath, "datadog")
}

// TestInstallFromMSIWithJava tests the Agent MSI can install the APM inject package with Java instrumentation
func (s *testAgentMSIInstallsAPMInject) TestInstallFromMSIWithJava() {
	// Setup Java
	javaHelper := NewJavaHelper(s)
	javaHelper.SetupJava()

	// Install with Java instrumentation
	s.installCurrentAgentVersion(
		WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=host"),
		// TODO: remove override once image is published in prod
		WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com"),
		WithMSIArg("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT="+s.currentAPMInjectVersion.PackageVersion()),
		WithMSIArg("DD_APM_INSTRUMENTATION_LIBRARIES=java:1"),
		WithMSILogFile("install.log"),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	// Start the Java app and check that the Datadog tracer is loaded
	output := javaHelper.StartJavaApp(apmInjectJavaAppFile)
	s.Require().Contains(output, "Datadog Tracer is available", "Datadog tracer should be loaded when instrumentation is enabled")
}

// installCurrentAgentVersionWithAPMInject installs the current agent version with APM inject via script
func (s *testAgentMSIInstallsAPMInject) installCurrentAgentVersion(opts ...MsiOption) {
	agentVersion := s.CurrentAgentVersion().Version()

	options := []MsiOption{
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-current-version.log"),
		WithMSIArg("APIKEY=" + unixinstaller.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
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
