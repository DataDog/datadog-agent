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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	parControlProcess  = "datadog-agent-action-control"
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

func (s *linuxPARSplitSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// The split control plane must be the OPMS poller, while the expensive Go
	// executor is reclaimed after startup pre-warm.
	s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := s.Env().FakeIntake.Client().GetPARDequeueCount()
		assert.NoError(c, err)
		assert.Greater(c, count, 0, "par-control has not polled fakeintake")
	}, 2*time.Minute, 2*time.Second)
	s.waitForProcessState(parExecutorProcess, "Stopped", 2*time.Minute)
}

// TestSplitRunnerReportsLivenessToOPMS proves the always-on control plane runs
// the runner health-check loop the Go monolith owns via its CommonRunner. In
// split mode the monolith stands down, so if par-control did not health-check,
// the only OPMS traffic from an idle host would be task polling and the runner's
// liveness signal would disappear for every split-mode deployment. The interval
// is a fixed 30s contract (healthCheckInterval in the Go config constants), so
// the wait budget covers at least two intervals.
func (s *linuxPARSplitSuite) TestSplitRunnerReportsLivenessToOPMS() {
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := s.Env().FakeIntake.Client().GetPARHealthCheckCount()
		assert.NoError(c, err)
		assert.Greater(c, count, 0, "par-control never sent an OPMS runner health check")
	}, 90*time.Second, 5*time.Second)
}

func (s *linuxPARSplitSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if !s.IsDevMode() {
		s.Require().NoError(s.Env().FakeIntake.Client().FlushPAR())
	}
}

// TestSplitRunnerStartsExecutorOnDemand proves the packaged process-manager
// definitions, Rust control plane, local gRPC transport, and Go executor work
// together. The executor begins stopped, runs one real action, publishes the
// result to fakeintake, and is reclaimed again after the idle timeout.
func (s *linuxPARSplitSuite) TestSplitRunnerStartsExecutorOnDemand() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo par-split-e2e",
		"allowedCommands": []string{"rshell:echo"},
	})
	s.Require().NoError(err)

	result, err := s.Env().FakeIntake.Client().GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err)
	s.Require().True(result.Success, "split PAR action failed: %+v", result)
	s.Require().Equal(0, rshellExitCode(result), "unexpected rshell result: %+v", result)
	assert.Contains(s.T(), result.Outputs["stdout"], "par-split-e2e")

	s.waitForProcessState(parExecutorProcess, "Stopped", 2*time.Minute)
}

func (s *linuxPARSplitSuite) waitForProcessState(name, state string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
			"sudo %s --socket %s describe %s",
			procmgrCLI,
			procmgrSocket,
			name,
		))
		assert.NoError(c, err)
		assert.Contains(c, strings.ReplaceAll(output, " ", ""), "State:"+state)
	}, timeout, 2*time.Second, "%s should become %s", name, state)
}
