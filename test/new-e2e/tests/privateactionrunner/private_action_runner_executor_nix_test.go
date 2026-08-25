// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// Binary and default unix socket for the on-demand executor (split deployment).
	privateActionRunnerBinary     = "/opt/datadog-agent/embedded/bin/privateactionrunner"
	privateActionRunnerConfigPath = "/etc/datadog-agent/datadog.yaml"
	executorSocketPath            = "/opt/datadog-agent/run/par-executor.sock"

	executorListeningLogLine = "Private action runner executor listening on"
	executorReadyLogLine     = "Private action runner executor ready to accept actions"
)

type linuxPrivateActionRunnerExecutorSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxPrivateActionRunnerExecutorSuite(t *testing.T) {
	t.Parallel()

	config := GenerateTestPrivateActionRunnerConfig(t)

	e2e.Run(t, &linuxPrivateActionRunnerExecutorSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

// TestExecutorStartsAndListens launches the on-demand executor subcommand and
// asserts it comes up: the process runs, the gRPC unix socket is created, and
// the log reports the server listening and ready.
func (s *linuxPrivateActionRunnerExecutorSuite) TestExecutorStartsAndListens() {
	host := s.Env().RemoteHost
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)

	// The executor's remote-config client talks to the core Agent. Stop the Agent
	// first so the executor has subscribed to AP_RUNNER_KEYS before the Agent's
	// remote-config service performs its first fetch.
	_, err := svcManager.Stop(coreAgentServiceName)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_, _ = svcManager.Start(coreAgentServiceName)
	})
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, statusErr := svcManager.Status(coreAgentServiceName)
		require.Error(c, statusErr)
	}, 30*time.Second, time.Second, "core Agent should be stopped")

	PushFakeRunnerKeysConfig(s.T(), s.Env().FakeIntake.Client())

	// run-executor is a foreground subcommand, not the packaged systemd service.
	// Launch it detached as dd-agent so it can bind its socket under
	// /opt/datadog-agent/run and read the agent IPC cert from /etc/datadog-agent.
	// The pid is captured directly (rather than found via pgrep) because pgrep -f
	// matches on the full command line, so it would also match the very shell
	// invocation used to search for it.
	launch := fmt.Sprintf(
		`sudo -u dd-agent bash -c 'nohup %s run-executor --cfgpath=%s </dev/null >/dev/null 2>&1 & echo $!'`,
		privateActionRunnerBinary, privateActionRunnerConfigPath,
	)
	pid := strings.TrimSpace(host.MustExecute(launch))

	// Allow retrying this test on the same host: kill the detached executor and
	// reset the artifacts it leaves behind so a retry doesn't observe stale state.
	s.T().Cleanup(func() {
		_, _ = host.Execute("sudo kill " + pid)
		_, _ = host.Execute("sudo rm -f " + executorSocketPath)
		_, _ = host.Execute("sudo truncate -s 0 " + privateActionRunnerLogFile)
	})

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, "sudo kill -0 "+pid)
	}, 2*time.Minute, 5*time.Second, "executor process should be running")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, "sudo test -S "+executorSocketPath)
	}, 2*time.Minute, 5*time.Second, "executor gRPC socket should exist")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -F %q %s", executorListeningLogLine, privateActionRunnerLogFile))
	}, 2*time.Minute, 5*time.Second, "executor log should report listening")

	// The executor is now listening but waiting for its first signing-key update.
	// Start the core Agent only after the AP_RUNNER_KEYS subscription is in place;
	// its first remote-config request can then include the product immediately.
	_, err = svcManager.Start(coreAgentServiceName)
	s.Require().NoError(err)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(coreAgentServiceName)
		require.NoError(c, statusErr)
		require.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second, "core Agent should be active")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -F %q %s", executorReadyLogLine, privateActionRunnerLogFile))
	}, 5*time.Minute, 5*time.Second, "executor log should report ready")
}
