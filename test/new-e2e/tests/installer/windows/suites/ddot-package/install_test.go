// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddottests implements a minimal E2E test for installing the DDOT OCI package on Windows.
package ddottests

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testDDOTInstallSuite struct {
	installerwindows.BaseSuite
}

// TestDDOTInstalls installs the DDOT (otel) OCI package via the Datadog installer and verifies files exist.
// This intentionally does not assert service behavior yet.
func TestDDOTInstalls(t *testing.T) {
	e2e.Run(t, &testDDOTInstallSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

func (s *testDDOTInstallSuite) TestInstallDDOTPackage() {
	// Arrange: ensure core Agent is installed (infrastructure directories, configs)
	// We use the MSI so repo paths exist, then we use OCI to install DDOT.
	s.uninstallAgentIfPresent()
	s.installAgentWithMSI()

	// Act: install DDOT from current pipeline via OCI
	output, err := s.Installer().InstallPackage("datadog-ddot")

	// Assert: package install succeeded and files exist in repository
	s.Require().NoErrorf(err, "failed to install the DDOT package: %s", output)
	stableDir := consts.GetStableDirFor("datadog-agent-ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(stableDir, "stable link for ddot package should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(stableDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present in embedded/bin",
	)
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(stableDir, "etc", "datadog-agent", "otel-config.yaml.example"),
		"otel-config.yaml.example should be present in etc/datadog-agent",
	)
}

func (s *testDDOTInstallSuite) installAgentWithMSI() {
	// Install current pipeline Agent MSI to establish base directories/config
	_, err := windowsAgent.InstallAgent(
		s.Env().RemoteHost,
		windowsAgent.WithPackage(s.CurrentAgentVersion().MSIPackage()),
		windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install-agent-msi.log")),
		windowsAgent.WithZeroAPIKey(),
	)
	s.Require().NoErrorf(err, "failed to install Agent with MSI")
}

func (s *testDDOTInstallSuite) uninstallAgentIfPresent() {
	_ = windowsAgent.UninstallAgent(s.Env().RemoteHost, filepath.Join(s.SessionOutputDir(), "uninstall-agent-msi.log"))
	s.Installer().Purge()
}
