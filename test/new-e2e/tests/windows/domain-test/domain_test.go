package domain

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/active_directory"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

const (
	TestDomain   = "datadogqalab.com"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
)

func TestInstallsOnDomainController(t *testing.T) {
	suites := []e2e.Suite[active_directory.ActiveDirectoryEnv]{
		&testInstallSuite{},
		&testUpgradeSuite{},
	}

	for _, suite := range suites {
		e2e.Run(t, suite, e2e.WithProvisioner(active_directory.Provisioner(
			active_directory.WithActiveDirectoryOptions(
				active_directory.WithDomainName(TestDomain),
				active_directory.WithDomainPassword(TestPassword),
				active_directory.WithDomainUser(TestUser, TestPassword),
			))))
	}
}

type testInstallSuite struct {
	windows.BaseAgentInstallerSuite
}

func (suite *testInstallSuite) TestGivenDomainUserCanInstallAgent() {
	host := suite.Env().DomainControllerHost

	_, err := suite.InstallAgent(host,
		windows.WithPackage(suite.AgentPackage),
		windows.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
		windows.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windows.WithValidApiKey(),
		windows.WithFakeIntake(suite.Env().FakeIntake),
		windows.WithInstallLogFile("TC-INS-DC-006_install.log"))

	suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")
	suite.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := suite.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)
}

type testUpgradeSuite struct {
	windows.BaseAgentInstallerSuite
}

func (suite *testUpgradeSuite) TestGivenDomainUserCanUpgradeAgent() {

	host := suite.Env().DomainControllerHost

	_, err := suite.InstallAgent(host,
		windows.WithLastStablePackage(),
		windows.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
		windows.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windows.WithValidApiKey(),
		windows.WithFakeIntake(suite.Env().FakeIntake),
		windows.WithInstallLogFile("TC-UPG-DC-001_install_last_stable.log"))

	suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")
	suite.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := suite.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)

	_, err = suite.InstallAgent(host,
		windows.WithPackage(suite.AgentPackage),
		windows.WithInstallLogFile("TC-UPG-DC-001_upgrade.log"))

	suite.Require().NoError(err, "should succeed to upgrade an Agent on a Domain Controller")
	suite.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := suite.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)
}
