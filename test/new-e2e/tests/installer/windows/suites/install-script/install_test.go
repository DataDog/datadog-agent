// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	agentVersion "github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"testing"
)

type testInstallScriptSuite struct {
	installerwindows.BaseSuite
}

// TestAgentInstalls tests the usage of the Datadog installer to install the Datadog Agent package.
func TestInstallScript(t *testing.T) {
	e2e.Run(t, &testInstallScriptSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog installer.
func (s *testInstallScriptSuite) TestInstallAgentPackage() {
	s.Run("Fresh install", func() {
		s.InstallLastStable()
		s.Run("Install different Agent version", func() {
			s.UpgradeToLatestExperiment()
			s.Run("Reinstall last stable", func() {
				s.InstallLastStable()
			})
		})
	})
}

func (s *testInstallScriptSuite) InstallLastStable() {
	// Arrange

	// Act
	output, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(map[string]string{
		"DD_AGENT_MINOR_VERSION": fmt.Sprintf("%d.0", s.CurrentAgentVersion().Minor-1),
	}))

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			actualVersion, err := agentVersion.New(version, "")
			s.Require().NoError(err, "Agent version was in the wrong format")
			s.Require().Equal(s.CurrentAgentVersion().Minor-1, actualVersion.Minor)
		})
}

func (s *testInstallScriptSuite) UpgradeToLatestExperiment() {
	// Act
	output, err := s.Installer().InstallExperiment(installerwindows.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}
