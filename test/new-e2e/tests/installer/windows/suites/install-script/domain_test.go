// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/test-infra-definitions/components/activedirectory"

	"testing"
)

const (
	TestDomain   = "datadogqalab.com"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
)

type testInstallScriptOnDCSuite struct {
	installerwindows.BaseSuite
}

// TestInstallScriptWithAgentUserOnDC tests tests the Datadog Install script with a custom user and password on a Domain Controller.
func TestInstallScriptWithAgentUserOnDC(t *testing.T) {
	e2e.Run(t, &testInstallScriptOnDCSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithActiveDirectoryOptions(
					activedirectory.WithDomainController(TestDomain, TestPassword),
					activedirectory.WithDomainUser(TestUser, TestPassword),
				),
			),
		),
	)
}

// TestInstallScriptWithAgentUser tests the Datadog Install script with a custom user and password.
//
// Use a DC since this requires the password be set.
func (s *testInstallScriptOnDCSuite) TestInstallScriptWithAgentUser() {
	// Arrange

	// Act
	out, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(map[string]string{
		"DD_AGENT_USER_NAME":     TestUser,
		"DD_AGENT_USER_PASSWORD": TestPassword,
	}))
	s.T().Log(out)

	// Assert
	if s.NoError(err) {
		fmt.Printf("%s\n", out)
	}
	s.Require().NoErrorf(err, "install script failed")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", TestUser)
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, TestUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasAService("datadogagent").
		WithIdentity(identity)
}
