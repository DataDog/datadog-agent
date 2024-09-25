// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	installer "github.com/DataDog/datadog-agent/test/new-e2e/pkg/components/datadog-installer"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/stretchr/testify/assert"
	"testing"
)

type testAgentInstallWithAgentUserSuite struct {
	installerwindows.BaseInstallerSuite
	agentUser string
}

// TestAgentInstalls tests the usage of the Datadog installer to install the Datadog Agent package.
func TestAgentInstallsWithAgentUser(t *testing.T) {
	agentUser := "customuser"
	assert.NotEqual(t, windowsAgent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")

	e2e.Run(t, &testAgentInstallWithAgentUserSuite{
		agentUser: agentUser,
	},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithInstaller(
					installer.WithAgentUser("customuser"),
				),
			)))
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog installer.
func (s *testAgentInstallWithAgentUserSuite) TestInstallAgentPackage() {
	s.Run("Install", func() {
		s.installAgent()
	})
}

func (s *testAgentInstallWithAgentUserSuite) installAgent() {
	// Arrange

	// Act
	_, err := s.Installer().InstallPackage(installerwindows.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(installerwindows.RegistryKeyPath).
		WithValueEqual("installedUser", s.agentUser)
	identity, err := windowsCommon.GetIdentityForUser(s.Env().RemoteHost, s.agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasAService("datadogagent").
		WithIdentity(identity)
}
