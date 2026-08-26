// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package domain

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"

	scenwindows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	platformCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	installtest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test"
)

const (
	TestDomain   = "datadogqalab.com"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"

	// preLSASecretAgentVersion is an Agent version from before 7.66, which is when the installer
	// started saving the Agent user password in the LSA secret store. Installing this version first
	// reproduces the state of a host that has only ever been upgraded without re-providing the
	// password.
	preLSASecretAgentVersion = "7.65.2-1"

	procmgrServiceName             = "dd-procmgr-service"
	privateActionRunnerServiceName = "datadog-agent-action"
)

func TestInstallsOnDomainController(t *testing.T) {
	suites := []e2e.Suite[environments.WindowsHost]{
		&testBasicInstallSuite{},
		&testUpgradeSuite{},
		&testInstallUserSyntaxSuite{},
		&testUpgradeWithoutStoredPasswordSuite{},
	}

	for _, suite := range suites {
		suite := suite
		t.Run(reflect.TypeOf(suite).Elem().Name(), func(t *testing.T) {
			t.Parallel()
			e2e.Run(t, suite,
				// Keep the stack alive on failure so the team can investigate Active
				// Directory provisioning failures (see WINA-1965). A Datadog log monitor
				// on the "SkipDeleteOnFailure feature is enabled" line notifies
				// #windows-products-ops.
				e2e.WithSkipDeleteOnFailure(),
				e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgent(
					awsHostWindows.WithRunOptions(
						scenwindows.WithActiveDirectoryOptions(
							activedirectory.WithDomainController(TestDomain, TestPassword),
							activedirectory.WithDomainUser(TestUser, TestPassword),
						),
					),
				)))
		})
	}
}

type testInstallSuite struct {
	windows.BaseAgentInstallerSuite[environments.WindowsHost]
}

func (suite *testInstallSuite) testGivenDomainUserCanInstallAgent(username string) {
	host := suite.Env().RemoteHost

	_, err := suite.InstallAgent(host,
		windowsAgent.WithPackage(suite.AgentPackage),
		windowsAgent.WithAgentUser(username),
		windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithFakeIntake(suite.Env().FakeIntake),
		windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "TC-INS-DC-006_install.log")))

	suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")

	suite.Run("user is a member of expected groups", func() {
		installtest.AssertAgentUserGroupMembership(suite.T(), host,
			windowsCommon.MakeDownLevelLogonName(TestDomain, TestUser),
		)
	})
	tc := suite.NewTestClientForHost(suite.Env().RemoteHost)
	tc.CheckAgentVersion(suite.T(), suite.AgentPackage.AgentVersion())
	platformCommon.CheckAgentBehaviour(suite.T(), tc)
	suite.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := suite.Env().FakeIntake.Client().RouteStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
	}, 5*time.Minute, 10*time.Second)
}

type testBasicInstallSuite struct {
	testInstallSuite
}

func (suite *testBasicInstallSuite) TestGivenDomainUserCanInstallAgent() {
	suite.testGivenDomainUserCanInstallAgent(fmt.Sprintf("%s\\%s", TestDomain, TestUser))
}

type testInstallUserSyntaxSuite struct {
	testInstallSuite
}

func (suite *testInstallUserSyntaxSuite) TestGivenDomainUserCanInstallAgent() {
	suite.testGivenDomainUserCanInstallAgent(fmt.Sprintf("%s@%s", TestUser, TestDomain))
}

type testUpgradeSuite struct {
	windows.BaseAgentInstallerSuite[environments.WindowsHost]
}

func (suite *testUpgradeSuite) TestGivenDomainUserCanUpgradeAgent() {
	host := suite.Env().RemoteHost

	_, err := suite.InstallAgent(host,
		windowsAgent.WithLastStablePackage(),
		windowsAgent.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
		windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithFakeIntake(suite.Env().FakeIntake),
		windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "TC-UPG-DC-001_install_last_stable.log")))

	suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")

	tc := suite.NewTestClientForHost(suite.Env().RemoteHost)
	platformCommon.CheckAgentBehaviour(suite.T(), tc)

	_, err = suite.InstallAgent(host,
		windowsAgent.WithPackage(suite.AgentPackage),
		windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "TC-UPG-DC-001_upgrade.log")))
	suite.Require().NoError(err, "should succeed to upgrade an Agent on a Domain Controller")

	tc.CheckAgentVersion(suite.T(), suite.AgentPackage.AgentVersion())
	platformCommon.CheckAgentBehaviour(suite.T(), tc)
	suite.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := suite.Env().FakeIntake.Client().RouteStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
	}, 5*time.Minute, 10*time.Second)
}

type testUpgradeWithoutStoredPasswordSuite struct {
	windows.BaseAgentInstallerSuite[environments.WindowsHost]
}

// TestUpgradeWithoutPasswordKeepsProcessManagerEnabled covers upgrading a host first
// installed with Agent 7.65 or earlier without providing DDAGENTUSER_PASSWORD. Before
// dd-procmgr-service moved to LocalSystem (#55529), that upgrade disabled the service to
// avoid domain account lockout (#55130). With LocalSystem procmgr, the service stays enabled
// because it no longer logs on as the Agent user. Agent-profile children must still spawn
// using the SCM-stored datadogagent credential when the installer LSA secret is missing.
func (suite *testUpgradeWithoutStoredPasswordSuite) TestUpgradeWithoutPasswordKeepsProcessManagerEnabled() {
	host := suite.Env().RemoteHost
	username := fmt.Sprintf("%s\\%s", TestDomain, TestUser)

	// 7.65 does not store the Agent user password in the LSA secret store, so the upgrade below has no
	// password available to it.
	previousAgentPackage, err := windowsAgent.NewPackage(windowsAgent.WithVersion(preLSASecretAgentVersion))
	suite.Require().NoError(err, "should resolve the %s package", preLSASecretAgentVersion)

	_, err = suite.InstallAgent(host,
		windowsAgent.WithPackage(previousAgentPackage),
		windowsAgent.WithAgentUser(username),
		windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithFakeIntake(suite.Env().FakeIntake),
		windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "install_pre_lsa_secret.log")))
	suite.Require().NoError(err, "should succeed to install Agent %s with a domain account & password", preLSASecretAgentVersion)

	// Reset fakeintake so the post-upgrade payload check below can't pass on data sent by the pre-upgrade install.
	suite.Require().NoError(suite.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	// Upgrade without providing the password.
	_, err = suite.InstallAgent(host,
		windowsAgent.WithPackage(suite.AgentPackage),
		windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "upgrade_without_password.log")))
	suite.Require().NoError(err, "should succeed to upgrade the Agent without providing the password")

	suite.Run("process manager stays enabled as LocalSystem", func() {
		config, err := windowsCommon.GetServiceConfig(host, procmgrServiceName)
		suite.Require().NoError(err)
		suite.Assert().Equal(windowsCommon.SERVICE_DEMAND_START, config.StartType,
			"%s must stay enabled when it runs as LocalSystem", procmgrServiceName)

		account, err := windowsCommon.GetServiceAccountName(host, procmgrServiceName)
		suite.Require().NoError(err)
		suite.Assert().Equal("LocalSystem", account,
			"%s must run as LocalSystem after upgrade", procmgrServiceName)

		suite.Assert().NoError(windowsCommon.StartService(host, procmgrServiceName),
			"%s should start without the Agent user password once it runs as LocalSystem", procmgrServiceName)
	})

	suite.Run("private action runner uses domain agent account", func() {
		account, err := windowsCommon.GetServiceAccountName(host, privateActionRunnerServiceName)
		suite.Require().NoError(err)
		suite.Assert().Equal(
			windowsCommon.MakeDownLevelLogonName(TestDomain, TestUser),
			account,
			"%s must run as the domain Agent user after no-password upgrade",
			privateActionRunnerServiceName,
		)
	})

	suite.Run("agent-profile children spawn without installer LSA password", func() {
		suite.assertAgentProfileSpawnWithoutInstallerLSAPassword(
			host,
			windowsCommon.MakeDownLevelLogonName(TestDomain, TestUser),
		)
	})

	suite.Run("core Agent is unaffected", func() {
		tc := suite.NewTestClientForHost(host)
		tc.CheckAgentVersion(suite.T(), suite.AgentPackage.AgentVersion())
		platformCommon.CheckAgentBehaviour(suite.T(), tc)
		suite.EventuallyWithT(func(c *assert.CollectT) {
			stats, err := suite.Env().FakeIntake.Client().RouteStats()
			assert.NoError(c, err)
			assert.NotEmpty(c, stats)
		}, 5*time.Minute, 10*time.Second)
	})

	suite.Run("providing the password keeps process manager healthy", func() {
		_, err := suite.InstallAgent(host,
			windowsAgent.WithPackage(suite.AgentPackage),
			windowsAgent.WithAgentUser(username),
			windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
			windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "reinstall_with_password.log")))
		suite.Require().NoError(err, "should succeed to reinstall the Agent with the password provided")

		config, err := windowsCommon.GetServiceConfig(host, procmgrServiceName)
		suite.Require().NoError(err)
		suite.Assert().Equal(windowsCommon.SERVICE_DEMAND_START, config.StartType,
			"%s must stay enabled after reinstall with the Agent user password", procmgrServiceName)
		suite.Assert().NoError(windowsCommon.StartService(host, procmgrServiceName),
			"%s should start once the Agent user password is available", procmgrServiceName)
	})
}
