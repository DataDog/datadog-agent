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
)

func TestInstallsOnDomainController(t *testing.T) {
	suites := []e2e.Suite[environments.WindowsHost]{
		&testBasicInstallSuite{},
		&testUpgradeSuite{},
		&testInstallUserSyntaxSuite{},
	}

	for _, suite := range suites {
		suite := suite
		t.Run(reflect.TypeOf(suite).Elem().Name(), func(t *testing.T) {
			t.Parallel()
			e2e.Run(t, suite, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgent(
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
