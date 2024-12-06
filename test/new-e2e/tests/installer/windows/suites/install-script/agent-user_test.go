// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/stretchr/testify/require"
	"testing"
)

type testInstallScriptWithAgentUserSuite struct {
	installerwindows.BaseInstallerSuite
	agentUser string
}

// TestInstallScriptWithAgentUser tests the Datadog Install script with a custom user
func TestInstallScriptWithAgentUser(t *testing.T) {
	agentUser := "customuser"
	require.NotEqual(t, windowsAgent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")

	e2e.Run(t, &testInstallScriptWithAgentUserSuite{
		agentUser: agentUser,
	},
		e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()),
	)
}

// TestInstallScriptWithAgentUser tests the Datadog Install script with a custom user
func (s *testInstallScriptWithAgentUserSuite) TestInstallScriptWithAgentUser() {
	// Arrange

	// Act
	out, err := s.Installer().RunInstallScript(map[string]string{
		"DD_AGENT_USER_NAME": s.agentUser,
	})
	s.T().Log(out)

	// Assert
	s.Require().NoErrorf(err, "install script failed")
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
