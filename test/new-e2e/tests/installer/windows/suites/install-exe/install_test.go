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
)

// testInstallExeSuite is a test suite that uses the exe installer
type testInstallExeSuite struct {
	installerwindows.BaseSuite
}

// TestInstallExe tests the usage of the Datadog installer exe to install the Datadog Agent package.
//
// This test may end up being transitionary only. The exe is intended to pin/only install its own matching
// version of the Agent, but this is WIP. Once we migrate to the install script that installs the
// matching exe version this test may not be needed anymore.
func TestInstallExe(t *testing.T) {
	e2e.Run(t, &testInstallExeSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// BeforeTest sets up the test
func (s *testInstallExeSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	s.SetInstallScriptImpl(installerwindows.NewDatadogInstallExe(s.Env(),
		installerwindows.WithInstallScriptDevEnvOverrides("CURRENT_AGENT"),
	))
}

// TestInstallAgentPackage tests installing the Datadog Agent using the Datadog installer exe.
func (s *testInstallExeSuite) TestInstallAgentPackage() {
	// Arrange
	packageConfig, err := installerwindows.NewPackageConfig(
		installerwindows.WithPackage(s.CurrentAgentVersion().OCIPackage()),
	)
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
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		}).
		HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, s.CurrentAgentVersion().PackageVersion())
		}).
		WithExperimentVersionEqual("")
}
