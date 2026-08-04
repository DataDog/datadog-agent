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

type linuxPARSplitLifecycleSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxPARSplitLifecycleSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	config := fmt.Sprintf(`private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: false
  urn: %s
  private_key: %s
`, urn, privateKey)

	e2e.Run(t, &linuxPARSplitLifecycleSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

func (s *linuxPARSplitLifecycleSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// These states jointly prove that the package contains both process
	// definitions and that the installed par-control asked dd-procmgrd to start
	// the otherwise on-demand executor.
	s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	s.waitForProcessState(parExecutorProcess, "Running", 2*time.Minute)
}

// TestControlShutdownReapsExecutor proves the installed process lifecycle in
// both directions: stopping par-control causes it to stop the executor through
// dd-procmgrd, and starting par-control brings the executor back. Restoring the
// initial state also keeps same-host test retries independent.
func (s *linuxPARSplitLifecycleSuite) TestControlShutdownReapsExecutor() {
	s.Require().NoError(s.runProcmgr("stop", parControlProcess))
	defer func() {
		s.Require().NoError(s.runProcmgr("start", parControlProcess))
		s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
		s.waitForProcessState(parExecutorProcess, "Running", 2*time.Minute)
	}()

	s.waitForProcessState(parControlProcess, "Stopped", 2*time.Minute)
	s.waitForProcessState(parExecutorProcess, "Stopped", 2*time.Minute)
}

func (s *linuxPARSplitLifecycleSuite) runProcmgr(command, name string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo %s --socket %s %s %s",
		procmgrCLI,
		procmgrSocket,
		command,
		name,
	))
	return err
}

func (s *linuxPARSplitLifecycleSuite) waitForProcessState(name, state string, timeout time.Duration) {
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
