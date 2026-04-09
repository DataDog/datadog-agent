// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testDDOTExtensionMSI struct {
	BaseSuite
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
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-ddot-extension.log"),
		WithMSIArg("APIKEY="+installer.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
		WithMSIArg("DD_OTELCOLLECTOR_ENABLED=true"),
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
		WithMSILogFile("uninstall-ddot-extension.log"),
	))

	// Assert: extension directory and service are gone
	s.Require().Host(s.Env().RemoteHost).NoDirExists(ddotExtDir, "ddot extension should be removed on uninstall")
	s.Require().Host(s.Env().RemoteHost).HasNoService("datadog-otel-agent")
}

// TestUpgradeEnablesDDOTExtension verifies that upgrading from a stable Agent
// to the current version with DD_OTELCOLLECTOR_ENABLED=true installs and starts
// the DDOT extension.
func (s *testDDOTExtensionMSI) TestUpgradeEnablesDDOTExtension() {
	// Arrange: install a stable Agent without DDOT
	s.installPreviousAgentVersion()

	// Act: upgrade to current version with DDOT enabled
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("upgrade-enable-ddot.log"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
		WithMSIArg("DD_OTELCOLLECTOR_ENABLED=true"),
	))

	// Assert: DDOT extension is installed and running
	ddotExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", "ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(ddotExtDir, "ddot extension directory should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(ddotExtDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present in the ddot extension",
	)
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}

func (s *testDDOTExtensionMSI) installPreviousAgentVersion(opts ...MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []MsiOption{
		WithOption(WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-previous-version.log"),
		WithMSIArg("APIKEY=" + installer.GetAPIKey()),
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

const (
	stableDDOTVersion      = "7.78.0-rc.4-1"
	stableDDOTAgentVersion = "7.78.0-rc.4"
)

type testDDOTExtensionMSIUpgrade struct {
	testDDOTExtensionMSI
}

// TestDDOTExtensionMSIUpgrade verifies that the DDOT extension persists
// through an MSI upgrade from the previous stable version to the current pipeline version.
func TestDDOTExtensionMSIUpgrade(t *testing.T) {
	s := &testDDOTExtensionMSIUpgrade{}
	s.CreateStableAgent = func() (*AgentVersionManager, error) {
		oci, err := NewPackageConfig(
			WithName(consts.AgentPackage),
			WithVersion(stableDDOTVersion),
			WithRegistry(consts.BetaS3OCIRegistry),
		)
		if err != nil {
			return nil, err
		}
		msi, err := windowsagent.NewPackage(
			windowsagent.WithProduct("datadog-agent"),
			windowsagent.WithArch("x86_64"),
			windowsagent.WithChannel("beta"),
			windowsagent.WithVersion(stableDDOTVersion),
		)
		if err != nil {
			return nil, err
		}
		return NewAgentVersionManager(stableDDOTAgentVersion, stableDDOTVersion, oci, msi)
	}
	e2e.Run(t, s,
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

// TestUpgradePreservesDDOTExtension installs the previous stable Agent with DDOT
// enabled, then upgrades to the current pipeline version without passing
// DD_OTELCOLLECTOR_ENABLED, and verifies the extension is still present and running.
func (s *testDDOTExtensionMSIUpgrade) TestUpgradePreservesDDOTExtension() {
	// Arrange: install previous stable Agent with DDOT enabled
	s.installPreviousAgentVersion(
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.BetaS3OCIRegistry),
		WithMSIArg("DD_OTELCOLLECTOR_ENABLED=true"),
	)

	// Sanity: DDOT is running after the initial install
	ddotExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", "ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(ddotExtDir, "ddot extension directory should exist after initial install")
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	// Act: upgrade to current version without DD_OTELCOLLECTOR_ENABLED
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("upgrade-to-current.log"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
	))

	// Assert: DDOT extension persists through the upgrade
	ddotExtDir = filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", "ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(ddotExtDir, "ddot extension directory should exist after upgrade")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(ddotExtDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present after upgrade",
	)
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}
