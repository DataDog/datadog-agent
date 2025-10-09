// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddottests implements E2E tests for installing the DDOT OCI package via the Agent MSI on Windows.
package ddottests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

type testAgentMSIInstallsDDOT struct {
	installerwindows.BaseSuite
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

	// Act: install current Agent MSI and request DDOT install via MSI properties
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-ddot.log"),
		installerwindows.WithMSIArg(fmt.Sprintf("APIKEY=%s", s.getAPIKey())),
		installerwindows.WithMSIArg("SITE=datadoghq.com"),
		installerwindows.WithMSIArg("DD_DDOT_ENABLED=1"),
		// Use testing registry override until images are published to prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
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
	// Install current Agent MSI with DDOT enabled
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-ddot.log"),
		installerwindows.WithMSIArg(fmt.Sprintf("APIKEY=%s", s.getAPIKey())),
		installerwindows.WithMSIArg("SITE=datadoghq.com"),
		installerwindows.WithMSIArg("DD_DDOT_ENABLED=1"),
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
	))

	stableDir := consts.GetStableDirFor("datadog-agent-ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(stableDir)

	// Act: uninstall the Agent and request DDOT removal
	s.Require().NoError(s.Installer().Uninstall(
		installerwindows.WithMSILogFile("uninstall-ddot.log"),
		installerwindows.WithMSIArg("DD_DDOT_UNINSTALL=1"),
	))

	// Assert: DDOT package directory removed
	s.Require().Host(s.Env().RemoteHost).NoDirExists(stableDir, "ddot package directory should be removed on uninstall when requested")
}

func (s *testAgentMSIInstallsDDOT) getAPIKey() string {
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

// installPreviousAgentVersion mirrors the helper used in other MSI suites to lay down the stable agent first.
func (s *testAgentMSIInstallsDDOT) installPreviousAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
		installerwindows.WithMSIArg(fmt.Sprintf("APIKEY=%s", s.getAPIKey())),
		installerwindows.WithMSIArg("SITE=datadoghq.com"),
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
