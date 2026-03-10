// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/process"
)

const (
	privateActionRunnerEnabledConfig = `private_action_runner:
  enabled: true
  private_key: test_private_key_value_for_e2e_testing
  urn: test_urn_value_for_e2e_testing
`
	privateActionRunnerStartedLogLine = "Starting private-action-runner"
)

type linuxPrivateActionRunnerEnabledSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxPrivateActionRunnerEnabledSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxPrivateActionRunnerEnabledSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithoutFakeIntake(),
				scenec2.WithAgentOptions(agentparams.WithAgentConfig(privateActionRunnerEnabledConfig)),
			),
		),
	))
}

func (s *linuxPrivateActionRunnerEnabledSuite) TestPrivateActionRunnerStartsWhenEnabled() {
	host := s.Env().RemoteHost
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)

	// Start the private action runner service
	_, err := svcManager.Start(privateActionRunnerServiceName)
	s.Require().NoError(err)

	// Verify the service is running
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(privateActionRunnerServiceName)
		assert.NoError(c, statusErr)
		assert.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second, "private action runner service should be active when enabled")

	// Verify the process is running
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, pidErr := process.FindPID(host, "privateactionrunner")
		assert.NoError(c, pidErr)
		assert.NotEmpty(c, pids, "privateactionrunner process should be running")
	}, 2*time.Minute, 5*time.Second, "privateactionrunner process should be running when enabled")

	// Verify the log file exists
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, logExistsErr := host.Execute("sudo test -f " + privateActionRunnerLogFile)
		assert.NoError(c, logExistsErr)
	}, 2*time.Minute, 5*time.Second, "private action runner log file should exist")

	// Verify log contains startup message
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, logContainsErr := host.Execute(fmt.Sprintf("sudo grep -i %q %s", privateActionRunnerStartedLogLine, privateActionRunnerLogFile))
		assert.NoError(c, logContainsErr)
	}, 2*time.Minute, 5*time.Second, "private action runner log should contain the started message")
}

func (s *linuxPrivateActionRunnerEnabledSuite) TestPrivateActionRunnerServiceRestart() {
	host := s.Env().RemoteHost
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)

	// Ensure service is started
	_, err := svcManager.Start(privateActionRunnerServiceName)
	s.Require().NoError(err)

	// Wait for service to be running
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(privateActionRunnerServiceName)
		assert.NoError(c, statusErr)
		assert.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second)

	// Get the original PID
	var originalPID int
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, pidErr := process.FindPID(host, "privateactionrunner")
		assert.NoError(c, pidErr)
		assert.NotEmpty(c, pids)
		if len(pids) > 0 {
			originalPID = pids[0]
		}
	}, 2*time.Minute, 5*time.Second)

	// Restart the service
	_, err = svcManager.Restart(privateActionRunnerServiceName)
	s.Require().NoError(err)

	// Verify service is running again
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(privateActionRunnerServiceName)
		assert.NoError(c, statusErr)
		assert.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second, "private action runner should be active after restart")

	// Verify we have a new PID (service actually restarted)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, pidErr := process.FindPID(host, "privateactionrunner")
		assert.NoError(c, pidErr)
		assert.NotEmpty(c, pids)
		if len(pids) > 0 {
			assert.NotEqual(c, originalPID, pids[0], "PID should change after restart")
		}
	}, 2*time.Minute, 5*time.Second, "privateactionrunner should have a new PID after restart")
}
