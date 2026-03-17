// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddottests implements E2E tests for the DDOT agent extension on Windows.
package ddottests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

type testDDOTExtensionMSI struct {
	installerwindows.BaseSuite
}

// TestDDOTExtensionViaMSI verifies that the DDOT extension is installed when
// DD_OTELCOLLECTOR_ENABLED=true is passed to the MSI, and that uninstalling the
// MSI removes the extension.
func TestDDOTExtensionViaMSI(t *testing.T) {
	e2e.Run(t, &testDDOTExtensionMSI{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

func (s *testDDOTExtensionMSI) TestInstallAndUninstallDDOTExtension() {
	// Arrange: install a previous agent version as a baseline
	s.installPreviousAgentVersion()

	// Act: install current Agent MSI with DDOT enabled
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-ddot-extension.log"),
		installerwindows.WithMSIArg("APIKEY="+installer.GetAPIKey()),
		installerwindows.WithMSIArg("SITE=datadoghq.com"),
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
		installerwindows.WithMSIArg("DD_OTELCOLLECTOR_ENABLED=true"),
	))

	// Assert: DDOT extension files exist under the agent package
	ddotExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", "ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(ddotExtDir, "ddot extension directory should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(ddotExtDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present in the ddot extension",
	)
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	// Act: uninstall the Agent MSI (purges OCI packages including extensions)
	s.Require().NoError(s.Installer().Uninstall(
		installerwindows.WithMSILogFile("uninstall-ddot-extension.log"),
	))

	// Assert: extension directory and service are gone
	s.Require().Host(s.Env().RemoteHost).NoDirExists(ddotExtDir, "ddot extension should be removed on uninstall")
	s.Require().Host(s.Env().RemoteHost).HasNoService("datadog-otel-agent")
}

func (s *testDDOTExtensionMSI) installPreviousAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
		installerwindows.WithMSIArg("APIKEY=" + installer.GetAPIKey()),
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
