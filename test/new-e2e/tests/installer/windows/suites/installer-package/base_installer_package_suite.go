// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installertests

import (
	"embed"

	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

//go:embed fixtures/sample_config*
var fixturesFS embed.FS

// baseInstallerPackageSuite is the base test suite for tests of the installer MSI
type baseInstallerPackageSuite struct {
	installerwindows.BaseSuite
}

func (s *baseInstallerPackageSuite) freshInstall() {
	// Arrange

	// Act
	s.InstallWithDiagnostics(
		installerwindows.WithMSILogFile("fresh-install.log"),
	)

	// Assert
	s.requireInstalled()
	// the service cannot start because of the missing API key
	s.requireRunning()
}

func (s *baseInstallerPackageSuite) startServiceWithConfigFile() {
	// Arrange
	s.Env().RemoteHost.CopyFileFromFS(fixturesFS, "fixtures/sample_config_enabled", consts.ConfigPath)

	// Act
	s.Require().NoError(common.StartService(s.Env().RemoteHost, consts.ServiceName))

	// Assert
	s.requireRunning()
}

func (s *baseInstallerPackageSuite) requireRunning() {
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogInstallerService()
}

func (s *baseInstallerPackageSuite) requireNotRunning() {
	s.Require().Host(s.Env().RemoteHost).
		HasAService(consts.ServiceName).
		WithStatus("Stopped").
		// no named pipe when service is not running
		HasNoNamedPipe(consts.NamedPipe)
}

func (s *baseInstallerPackageSuite) requireInstalled() {
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithSignature(agent.GetCodeSignatureThumbprints()).
		WithVersionMatchPredicate(func(version string) {
			s.Require().NotEmpty(version)
		}).
		HasAService(consts.ServiceName).
		WithIdentity(common.GetIdentityForSID(common.LocalSystemSID)).
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agent.DefaultAgentUserName)
}

func (s *baseInstallerPackageSuite) requireUninstalled() {
	s.Require().Host(s.Env().RemoteHost).
		NoFileExists(consts.BinaryPath).
		HasNoService(consts.ServiceName).
		HasNoRegistryKey(consts.RegistryKeyPath)
}
