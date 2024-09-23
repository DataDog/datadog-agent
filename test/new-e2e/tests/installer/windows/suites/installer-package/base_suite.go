// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installertests

import (
	"embed"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

//go:embed fixtures/sample_config
var fixturesFS embed.FS

// baseInstallerSuite is the base test suite for tests of the installer MSI
type baseInstallerSuite struct {
	installerwindows.BaseInstallerSuite
}

func (s *baseInstallerSuite) freshInstall() {
	// Arrange

	// Act
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithMSILogFile("fresh-install.log"),
	))

	// Assert
	s.requireInstalled()
	s.Require().Host(s.Env().RemoteHost).
		HasAService(installerwindows.ServiceName).
		// the service cannot start because of the missing API key
		WithStatus("Stopped")
}

func (s *baseInstallerSuite) startServiceWithConfigFile() {
	// Arrange
	s.Env().RemoteHost.CopyFileFromFS(fixturesFS, "fixtures/sample_config", installerwindows.ConfigPath)

	// Act
	s.Require().NoError(common.StartService(s.Env().RemoteHost, installerwindows.ServiceName))

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasAService(installerwindows.ServiceName).
		WithStatus("Running")
}

func (s *baseInstallerSuite) requireInstalled() {
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(installerwindows.BinaryPath).
		WithSignature(agent.GetCodeSignatureThumbprints()).
		WithVersionMatchPredicate(func(version string) {
			s.Require().NotEmpty(version)
		}).
		HasAService(installerwindows.ServiceName).
		WithIdentity(common.GetIdentityForSID(common.LocalSystemSID)).
		HasRegistryKey(installerwindows.RegistryKeyPath).
		WithValueEqual("installedUser", agent.DefaultAgentUserName)
}

func (s *baseInstallerSuite) requireUninstalled() {
	s.Require().Host(s.Env().RemoteHost).
		NoFileExists(installerwindows.BinaryPath).
		HasNoService(installerwindows.ServiceName).
		HasNoRegistryKey(installerwindows.RegistryKeyPath)
}
