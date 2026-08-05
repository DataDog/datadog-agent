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
	parControlProcess  = "datadog-agent-par-control"
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

// TestControlStartsExecutor proves the package ships both process definitions and
// that dd-procmgrd starts par-control, which starts the on-demand executor.
func (s *linuxPARSplitLifecycleSuite) TestControlStartsExecutor() {
	s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	s.waitForProcessState(parExecutorProcess, "Running", 2*time.Minute)
}

// TestControlStopsWithoutReenteringSupervisor proves par-control exits promptly on
// SIGTERM instead of making a nested Stop RPC that would block behind dd-procmgrd's
// in-progress stop. The supervisor keeps owning the executor.
func (s *linuxPARSplitLifecycleSuite) TestControlStopsWithoutReenteringSupervisor() {
	defer func() {
		s.Require().NoError(s.runProcmgr("start", parControlProcess))
		s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	}()

	started := time.Now()
	s.Require().NoError(s.runProcmgr("stop", parControlProcess))
	s.Require().Less(time.Since(started), 15*time.Second, "par-control stop should not hit its 30s timeout")

	s.waitForProcessState(parControlProcess, "Stopped", 10*time.Second)
	s.waitForProcessState(parExecutorProcess, "Running", 10*time.Second)
}

func (s *linuxPARSplitLifecycleSuite) runProcmgr(command, name string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo %s --socket %s %s %s", procmgrCLI, procmgrSocket, command, name,
	))
	return err
}

func (s *linuxPARSplitLifecycleSuite) waitForProcessState(name, state string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
			"sudo %s --socket %s describe %s", procmgrCLI, procmgrSocket, name,
		))
		assert.NoError(c, err)
		assert.Contains(c, strings.ReplaceAll(output, " ", ""), "State:"+state)
	}, timeout, 2*time.Second, "%s should become %s", name, state)
}
