// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	unixinstaller "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

type testAgentMSIInstallsDDOT struct {
	BaseSuite
}

// TestAgentMSIInstallsDDOTPackage verifies the Agent MSI can install and remove the DDOT OCI package.
func TestAgentMSIInstallsDDOTPackage(t *testing.T) {
	e2e.Run(t, &testAgentMSIInstallsDDOT{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

func (s *testAgentMSIInstallsDDOT) AfterTest(_suiteName, _testName string) {
	s.Installer().Purge()
}

func (s *testAgentMSIInstallsDDOT) TestInstallDDOTFromMSI() {
	// Arrange: install previous Agent MSI (mirror other suites)
	s.installPreviousAgentVersion()

	// Act: install current Agent MSI; opt-in to DDOT via DD_OTELCOLLECTOR_ENABLED
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-ddot.log"),
		WithMSIArg("APIKEY="+unixinstaller.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL=installtesting.datad0g.com.internal.dda-testing.com"),
		WithMSIArg("DD_OTELCOLLECTOR_ENABLED=true"),
	))

	// Assert: DDOT package stable directory exists and contains otel-agent.exe
	stableDir := consts.GetStableDirFor("datadog-agent-ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(stableDir, "stable link for ddot package should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(stableDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present in embedded/bin",
	)
}

func (s *testAgentMSIInstallsDDOT) TestUninstallDDOTFromMSI() {
	// Arrange: install previous Agent MSI (baseline)
	s.installPreviousAgentVersion()
	// Install current Agent MSI with DDOT enabled via DD_OTELCOLLECTOR_ENABLED
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-ddot.log"),
		WithMSIArg("APIKEY="+unixinstaller.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL=installtesting.datad0g.com.internal.dda-testing.com"),
		WithMSIArg("DD_OTELCOLLECTOR_ENABLED=true"),
	))

	stableDir := consts.GetStableDirFor("datadog-agent-ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(stableDir)

	// Act: uninstall the Agent (DDOT and other OCI packages are purged by default)
	s.Require().NoError(s.Installer().Uninstall(
		WithMSILogFile("uninstall-ddot.log"),
	))

	// Assert: DDOT package directory removed
	s.Require().Host(s.Env().RemoteHost).NoDirExists(stableDir, "ddot package directory should be removed on uninstall when requested")
}

// installPreviousAgentVersion mirrors the helper used in other MSI suites to lay down the stable agent first.
func (s *testAgentMSIInstallsDDOT) installPreviousAgentVersion(opts ...MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []MsiOption{
		WithOption(WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-previous-version.log"),
		WithMSIArg("APIKEY=" + unixinstaller.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
	}
	options = append(options, opts...)
	s.Require().NoError(s.Installer().Install(options...))

	// sanity: ensure the stable version is installed
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}
