// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/require"
)

type testInstallExeWithAgentUserSuite struct {
	BaseSuite
	agentUser string
}

// TestInstallExeWithAgentUser tests the Datadog installer exe with a custom user
func TestInstallExeWithAgentUser(t *testing.T) {
	agentUser := "customuser"
	require.NotEqual(t, windowsAgent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")

	e2e.Run(t, &testInstallExeWithAgentUserSuite{
		agentUser: agentUser,
	},
		e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()),
	)
}

// TestInstallExeWithAgentUser tests the Datadog installer exe with a custom user
func (s *testInstallExeWithAgentUserSuite) TestInstallExeWithAgentUser() {
	// Arrange

	// Act
	out, err := s.InstallScript().Run(WithExtraEnvVars(map[string]string{
		"DD_AGENT_USER_NAME": s.agentUser,
	}))
	s.T().Log(out)

	// Assert
	if s.NoError(err) {
		fmt.Printf("%s\n", out)
	}
	s.Require().NoErrorf(err, "installer exe failed")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", s.agentUser)
	identity, err := windowsCommon.GetIdentityForUser(s.Env().RemoteHost, s.agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasAService("datadogagent").
		WithIdentity(identity)

	// Reinstall the same Agent again, but this time without the custom user arg
	// it should keep the same user (settings read from registry)
	out, err = s.InstallScript().Run()
	s.T().Log(out)
	s.Require().NoErrorf(err, "installer exe failed")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", s.agentUser)

}

// TestInstallExeChangesAgentUser tests that the installer exe changes the agent user when the Agent is already installed
func (s *testInstallExeWithAgentUserSuite) TestInstallExeChangesAgentUser() {
	s.TestInstallExeWithAgentUser()
	s.agentUser = s.agentUser + "2"
	s.TestInstallExeWithAgentUser()
}
