// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

// Package ddottests implements E2E tests for the DDOT agent extension on Windows.

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testDDOTExtensionSubcommand struct {
	BaseSuite
}

// TestDDOTExtensionViaSubcommand verifies that the DDOT extension can be installed
// using the 'datadog-agent extension install' subcommand, and that purge removes it.
func TestDDOTExtensionViaSubcommand(t *testing.T) {
	e2e.Run(t, &testDDOTExtensionSubcommand{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

func (s *testDDOTExtensionSubcommand) AfterTest(_suiteName, _testName string) {
	s.Installer().Purge()
}

func (s *testDDOTExtensionSubcommand) TestInstallDDOTSubcommand() {
	// Install the base agent without DDOT via the install script.
	output, err := s.InstallScript().Run()
	s.Require().NoErrorf(err, "failed to install the Datadog Agent: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	// Install the ddot extension via the new 'datadog-agent extension install' subcommand.
	// Use the registry install path (same as the MSI install path) to locate agent.exe reliably.
	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err)
	agentExe := installPath + `\bin\agent.exe`
	agentPackageURL := "oci://" + consts.PipelineOCIRegistry + "/agent-package:pipeline-" + s.Env().Environment.PipelineID()
	cmd := fmt.Sprintf(`& "%s" extension install --url %s ddot`, agentExe, agentPackageURL)
	output, err = s.Env().RemoteHost.Execute(cmd)
	s.Require().NoErrorf(err, "failed to install ddot extension via subcommand: %s", output)

	// Assert: DDOT extension files exist under the agent package.
	ddotExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", "ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(ddotExtDir, "ddot extension directory should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(ddotExtDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present in the ddot extension",
	)
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}

type testDDOTExtensionInstallScript struct {
	BaseSuite
}

// TestDDOTExtensionViaInstallScript verifies that the DDOT extension is installed
// when DD_OTELCOLLECTOR_ENABLED=true is passed to the install script, and that
// purge removes it.
func TestDDOTExtensionViaInstallScript(t *testing.T) {
	e2e.Run(t, &testDDOTExtensionInstallScript{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

func (s *testDDOTExtensionInstallScript) AfterTest(_suiteName, _testName string) {
	s.Installer().Purge()
}

func (s *testDDOTExtensionInstallScript) TestInstallAndPurgeDDOTExtension() {
	// Act: install the Agent via the install script with DDOT enabled
	output, err := s.InstallScript().Run(
		WithExtraEnvVars(map[string]string{
			"DD_OTELCOLLECTOR_ENABLED": "true",
		}),
	)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	// Assert: DDOT extension files exist under the agent package
	ddotExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", "ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(ddotExtDir, "ddot extension directory should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(ddotExtDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present in the ddot extension",
	)
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	// Act: purge all packages
	_, err = s.Installer().Purge()
	s.Require().NoError(err)

	// Assert: extension directory and service are gone
	s.Require().Host(s.Env().RemoteHost).NoDirExists(ddotExtDir, "ddot extension should be removed after purge")
	s.Require().Host(s.Env().RemoteHost).HasNoService("datadog-otel-agent")
}
