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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	parControlProcess  = "datadog-agent-par-control"
	parExecutorProcess = "datadog-agent-action-executor"
	procmgrCLI         = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	procmgrSocket      = "/var/run/datadog-procmgrd/dd-procmgrd.sock"
)

type linuxPARSplitSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxPARSplitSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	config := fmt.Sprintf(`private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: false
  urn: %s
  private_key: %s
  idle_timeout_seconds: 5
  actions_allowlist:
    - %s
`, urn, privateKey, runCommandAction)

	e2e.Run(t, &linuxPARSplitSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(config),
					agentparams.WithFile(
						"/etc/datadog-agent/environment",
						"DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true\n",
						true,
					),
				),
			),
		),
	))
}

// TestSplitControlPlaneEndToEnd exercises the complete split-runner path in one
// ordered flow: OPMS polling and liveness, idle executor reclamation, on-demand
// action execution and publication, and graceful control-plane shutdown. Keeping
// the flow in one test avoids order dependencies between suite methods.
func (s *linuxPARSplitSuite) TestSplitControlPlaneEndToEnd() {
	client := s.Env().FakeIntake.Client()
	s.Require().NoError(client.FlushPAR(), "reset PAR state so same-host retries are independent")

	s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	// Exactly one process may poll OPMS in split mode. Confirm the monolithic
	// runner has exited before attributing the requests below to par-control.
	s.waitForProcessState(parMonolithProcess, "Exited", 2*time.Minute)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPARDequeueCount()
		require.NoError(c, err)
		require.Greater(c, count, 0, "par-control should poll OPMS")
	}, 2*time.Minute, 2*time.Second)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPARHealthCheckCount()
		require.NoError(c, err)
		require.Greater(c, count, 0, "par-control should report runner liveness")
	}, 90*time.Second, 5*time.Second)

	// The initial pre-warmed executor should be reclaimed while no tasks exist.
	s.waitForProcessState(parExecutorProcess, "Stopped", 2*time.Minute)

	taskID := uuid.New().String()
	s.Require().NoError(client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo par-split-e2e",
		"allowedCommands": []string{"rshell:echo"},
	}))

	result, err := client.GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err)
	s.Require().True(result.Success, "split PAR action failed: %+v", result)
	s.Require().Equal(0, rshellExitCode(s.T(), result), "unexpected rshell result: %+v", result)
	s.Require().Contains(result.Outputs["stdout"], "par-split-e2e")

	// Successful execution proves the stopped executor was started on demand;
	// verify it is reclaimed again after the configured idle timeout.
	s.waitForProcessState(parExecutorProcess, "Stopped", 2*time.Minute)

	// Stopping par-control through its supervisor must not deadlock on a nested
	// process-manager Stop RPC. Restore it so a same-host retry starts cleanly.
	defer func() {
		s.Require().NoError(s.runProcmgr("start", parControlProcess))
		s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	}()

	started := time.Now()
	s.Require().NoError(s.runProcmgr("stop", parControlProcess))
	s.Require().Less(time.Since(started), 15*time.Second, "par-control should stop promptly")
	s.waitForProcessState(parControlProcess, "Stopped", 10*time.Second)
	s.waitForProcessState(parExecutorProcess, "Stopped", 10*time.Second)
}

func (s *linuxPARSplitSuite) runProcmgr(command, name string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo %s --socket %s %s %s", procmgrCLI, procmgrSocket, command, name,
	))
	return err
}

func (s *linuxPARSplitSuite) waitForProcessState(name, state string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
			"sudo %s --socket %s describe %s", procmgrCLI, procmgrSocket, name,
		))
		require.NoError(c, err)
		require.Contains(c, strings.ReplaceAll(output, " ", ""), "State:"+state)
	}, timeout, 2*time.Second, "%s should become %s", name, state)
}
