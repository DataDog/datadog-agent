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
	privateActionRunnerServiceName = "datadog-agent-action"
	privateActionRunnerLogFile     = "/var/log/datadog/private-action-runner.log"

	privateActionRunnerDisabledConfig = `private_action_runner:
  enabled: false
`
	privateActionRunnerDisabledLogLine = "private-action-runner is not enabled. Set private_action_runner.enabled: true in your datadog.yaml file or set the environment variable DD_PRIVATE_ACTION_RUNNER_ENABLED=true."
)

type linuxPrivateActionRunnerSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxPrivateActionRunnerSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxPrivateActionRunnerSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithoutFakeIntake(),
				scenec2.WithAgentOptions(agentparams.WithAgentConfig(privateActionRunnerDisabledConfig)),
			),
		),
	))
}

func (s *linuxPrivateActionRunnerSuite) TestPrivateActionRunnerStopsWhenDisabled() {
	host := s.Env().RemoteHost
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)

	_, err := svcManager.Start(privateActionRunnerServiceName)
	s.Require().NoError(err)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, statusErr := svcManager.Status(privateActionRunnerServiceName)
		assert.Error(c, statusErr)
	}, 2*time.Minute, 5*time.Second, "private action runner service should stop when disabled")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, pidErr := process.FindPID(host, "privateactionrunner")
		assert.Error(c, pidErr)
		assert.Empty(c, pids)
	}, 2*time.Minute, 5*time.Second, "privateactionrunner process should not be running when disabled")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, logExistsErr := host.Execute(fmt.Sprintf("sudo test -f %s", privateActionRunnerLogFile))
		assert.NoError(c, logExistsErr)
	}, 2*time.Minute, 5*time.Second, "private action runner log file should exist")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, logContainsErr := host.Execute(fmt.Sprintf("sudo grep -F %q %s", privateActionRunnerDisabledLogLine, privateActionRunnerLogFile))
		assert.NoError(c, logContainsErr)
	}, 2*time.Minute, 5*time.Second, "private action runner log should contain the disabled message")
}
