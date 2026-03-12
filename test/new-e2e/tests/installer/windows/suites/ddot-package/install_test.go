// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddottests implements E2E tests for the DDOT agent extension on Windows.
package ddottests

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

type testDDOTExtensionInstallScript struct {
	installerwindows.BaseSuite
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
		installerwindows.WithExtraEnvVars(map[string]string{
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
