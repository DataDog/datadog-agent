// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package domain

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/activedirectory"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"time"
)

const (
	TestDomain   = "datadogqalab.com"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
)

func TestInstallsOnDomainController(t *testing.T) {
	suites := []e2e.Suite[activedirectory.Env]{
		&testInstallSuite{},
		&testUpgradeSuite{},
	}

	for _, suite := range suites {
		t.Run(reflect.TypeOf(suite).Name(), func(t *testing.T) {
			t.Parallel()
			e2e.Run(t, suite, e2e.WithProvisioner(activedirectory.Provisioner(
				activedirectory.WithActiveDirectoryOptions(
					activedirectory.WithDomainName(TestDomain),
					activedirectory.WithDomainPassword(TestPassword),
					activedirectory.WithDomainUser(TestUser, TestPassword),
				))))
		})
	}
}

type testInstallSuite struct {
	windows.BaseAgentInstallerSuite[activedirectory.Env]
}

func (suite *testInstallSuite) TestGivenDomainUserCanInstallAgent() {
	host := suite.Env().DomainControllerHost

	_, err := suite.InstallAgent(host,
		windowsAgent.WithPackage(suite.AgentPackage),
		windowsAgent.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
		windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithFakeIntake(suite.Env().FakeIntake),
		windowsAgent.WithInstallLogFile("TC-INS-DC-006_install.log"))

	suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")

	agent := suite.NewAgentClientForHost(suite.Env().DomainControllerHost)
	windowsAgent.TestAgentVersion(suite.T(), suite.AgentPackage.AgentVersion(), agent.Version())

	suite.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := suite.Env().FakeIntake.Client().RouteStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
	}, 5*time.Minute, 10*time.Second)
}

type testUpgradeSuite struct {
	windows.BaseAgentInstallerSuite[activedirectory.Env]
}

func (suite *testUpgradeSuite) TestGivenDomainUserCanUpgradeAgent() {

	host := suite.Env().DomainControllerHost

	_, err := suite.InstallAgent(host,
		windowsAgent.WithLastStablePackage(),
		windowsAgent.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
		windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithFakeIntake(suite.Env().FakeIntake),
		windowsAgent.WithInstallLogFile("TC-UPG-DC-001_install_last_stable.log"))

	suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")
	suite.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := suite.Env().FakeIntake.Client().RouteStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
	}, 5*time.Minute, 10*time.Second)

	_, err = suite.InstallAgent(host,
		windowsAgent.WithPackage(suite.AgentPackage),
		windowsAgent.WithInstallLogFile("TC-UPG-DC-001_upgrade.log"))
	suite.Require().NoError(err, "should succeed to upgrade an Agent on a Domain Controller")

	agent := suite.NewAgentClientForHost(suite.Env().DomainControllerHost)
	windowsAgent.TestAgentVersion(suite.T(), suite.AgentPackage.AgentVersion(), agent.Version())

	suite.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := suite.Env().FakeIntake.Client().RouteStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
	}, 5*time.Minute, 10*time.Second)
}
