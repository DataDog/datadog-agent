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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/process"
)

const (
	// Binary and default unix socket for the on-demand executor (split deployment).
	privateActionRunnerBinary     = "/opt/datadog-agent/embedded/bin/privateactionrunner"
	privateActionRunnerConfigPath = "/etc/datadog-agent/datadog.yaml"
	executorSocketPath            = "/opt/datadog-agent/run/par-executor.sock"

	// Substring long enough (>15 chars) to force pgrep -f, so it matches the
	// run-executor argument in the command line rather than the binary name.
	executorProcessMatch = "privateactionrunner run-executor"

	executorListeningLogLine = "Private action runner executor listening on"
	executorReadyLogLine     = "Private action runner executor ready to accept actions"
)

type linuxPrivateActionRunnerExecutorSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxPrivateActionRunnerExecutorSuite(t *testing.T) {
	t.Parallel()
	// Disable idle self-shutdown so the executor stays up for the whole test
	// (it would otherwise self-exit after being idle with no in-flight actions).
	config := GenerateTestPrivateActionRunnerConfig(t) + `  executor:
    idle_shutdown_timeout_seconds: 0
`
	e2e.Run(t, &linuxPrivateActionRunnerExecutorSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithoutFakeIntake(),
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

	// run-executor is a foreground subcommand, not the packaged systemd service.
	// Launch it detached as dd-agent so it can bind its socket under
	// /opt/datadog-agent/run and read the agent IPC cert from /etc/datadog-agent.
	launch := fmt.Sprintf(
		`sudo -u dd-agent nohup %s run-executor --cfgpath=%s </dev/null >/dev/null 2>&1 &`,
		privateActionRunnerBinary, privateActionRunnerConfigPath,
	)
	host.MustExecute(launch)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, err := process.FindPID(host, executorProcessMatch)
		assert.NoError(c, err)
		assert.NotEmpty(c, pids, "run-executor process should be running")
	}, 2*time.Minute, 5*time.Second, "executor process should be running")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, "sudo test -S "+executorSocketPath)
	}, 2*time.Minute, 5*time.Second, "executor gRPC socket should exist")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -F %q %s", executorListeningLogLine, privateActionRunnerLogFile))
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -F %q %s", executorReadyLogLine, privateActionRunnerLogFile))
	}, 2*time.Minute, 5*time.Second, "executor log should report listening and ready")
}
