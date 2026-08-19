// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package domain

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"

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

	procmgrServiceName = "dd-procmgr-service"
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

// TestUpgradeWithoutPasswordDoesNotLockOutAccount covers the regression where upgrading a host that was
// first installed with Agent 7.65 or earlier, without providing DDAGENTUSER_PASSWORD, created
// dd-procmgr-service with an empty password. The service then failed to log on and the SCM retried the
// failing logon indefinitely, locking out the domain account within a few minutes.
//
// The installer must now leave dd-procmgr-service disabled when no password is available, and re-enable
// it once a password is provided again.
func (suite *testUpgradeWithoutStoredPasswordSuite) TestUpgradeWithoutPasswordDoesNotLockOutAccount() {
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

	// Record the host time so the event log assertions below only consider the upgrade.
	upgradeStartTime, err := host.Execute("(Get-Date).ToString('o')")
	suite.Require().NoError(err)
	upgradeStartTime = strings.TrimSpace(upgradeStartTime)

	// Upgrade without providing the password.
	_, err = suite.InstallAgent(host,
		windowsAgent.WithPackage(suite.AgentPackage),
		windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "upgrade_without_password.log")))
	suite.Require().NoError(err, "should succeed to upgrade the Agent without providing the password")

	suite.Run("process manager is disabled", func() {
		config, err := windowsCommon.GetServiceConfig(host, procmgrServiceName)
		suite.Require().NoError(err)
		suite.Assert().Equal(windowsCommon.SERVICE_DISABLED, config.StartType,
			"%s must be disabled when the Agent user password is not available", procmgrServiceName)
	})

	// The failing logons happened once a minute and locked the account out after about 4 minutes, so
	// watch for longer than that before concluding the loop is gone.
	suite.Run("domain account is not locked out", func() {
		suite.Never(func() bool {
			out, err := host.Execute(fmt.Sprintf(
				`[bool](Search-ADAccount -LockedOut | Where-Object { $_.SamAccountName -eq '%s' })`, TestUser))
			if err != nil {
				suite.T().Logf("could not query locked out accounts: %v", err)
				return false
			}
			return strings.EqualFold(strings.TrimSpace(out), "True")
		}, 6*time.Minute, 30*time.Second, "the %s account must not be locked out after upgrading without a password", TestUser)
	})

	// Checked after the window above so that it covers the whole period the retry loop would have run in.
	suite.Run("process manager does not attempt to log on", func() {
		// The SCM never launches a disabled service, so there must be no service logon failures
		// (7000/7031/7038) and no failed account logons (4625) after the upgrade.
		suite.assertNoEventsAfter(host,
			fmt.Sprintf(`@{LogName='System'; ID=7000,7031,7038; StartTime=[datetime]'%s'}`, upgradeStartTime),
			procmgrServiceName)
		suite.assertNoEventsAfter(host,
			fmt.Sprintf(`@{LogName='Security'; ID=4625; StartTime=[datetime]'%s'}`, upgradeStartTime),
			TestUser)
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

	suite.Run("providing the password re-enables the process manager", func() {
		_, err := suite.InstallAgent(host,
			windowsAgent.WithPackage(suite.AgentPackage),
			windowsAgent.WithAgentUser(username),
			windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
			windowsAgent.WithInstallLogFile(filepath.Join(suite.SessionOutputDir(), "reinstall_with_password.log")))
		suite.Require().NoError(err, "should succeed to reinstall the Agent with the password provided")

		config, err := windowsCommon.GetServiceConfig(host, procmgrServiceName)
		suite.Require().NoError(err)
		suite.Assert().Equal(windowsCommon.SERVICE_DEMAND_START, config.StartType,
			"%s must be enabled again once the Agent user password is available", procmgrServiceName)
		suite.Assert().NoError(windowsCommon.StartService(host, procmgrServiceName),
			"%s should start once the Agent user password is available", procmgrServiceName)
	})
}

// assertNoEventsAfter asserts that no event matching filterHashTable mentions needle.
func (suite *testUpgradeWithoutStoredPasswordSuite) assertNoEventsAfter(host *components.RemoteHost, filterHashTable string, needle string) {
	entries, err := windowsCommon.GetEventLogEntriesWithFilterHashTable(host, filterHashTable)
	suite.Require().NoError(err)
	for _, entry := range entries {
		suite.Assert().NotContains(entry.Message, needle,
			"unexpected event %d mentioning %s: %s", entry.ID, needle, entry.Message)
	}
}
