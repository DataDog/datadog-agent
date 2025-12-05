// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/process"
)

type linuxRunSuite struct {
	baseRunSuite
}

func TestLinuxRunSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxRunSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (s *linuxRunSuite) TestRunWhenAgentAlreadyRunning() {
	host := s.Env().RemoteHost

	// Ensure agent is running
	s.Require().True(s.Env().Agent.Client.IsReady(), "agent should be running")

	// execute the `agent run` subcommand
	cmd := `sudo datadog-agent run`
	// run command with timeout in case it succeeds/hangs
	out, err := runCommandWithTimeout(host, cmd, s.runTimeout())
	if err == nil {
		s.T().Log(out)
		s.FailNow("agent run command succeeded when it should have failed")
	}
	s.Require().NotErrorIs(err, context.DeadlineExceeded, "agent run command timed out")
	// make sure it didn't panic
	s.Require().NotContains(err.Error(), "panic: runtime error")
	// make sure it printed a reasonable human readable error
	s.Require().ErrorContains(err, " listen tcp 127.0.0.1:5001: bind: address already in use")
	// TODO: Once host.Execute is fixed to return the exit code, check that the exit code is ??
}

func (s *linuxRunSuite) TestRunAgentCtrlC() {
	host := s.Env().RemoteHost

	// stop the agent
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)
	_, err := svcManager.Stop("datadog-agent")
	s.Require().NoError(err)

	// wait for the agent to be fully stopped
	s.EventuallyWithT(func(c *assert.CollectT) {
		_, err := svcManager.Status("datadog-agent")
		// Status should return an error if the service is not running
		require.Error(c, err)
	}, 10*time.Second, 1*time.Second, "datadog Agent should be stopped")

	// execute the `agent run` subcommand
	cmd := `sudo datadog-agent run`

	// run command with timeout it
	_, _, stdout, err := host.Start(cmd)
	if err != nil {
		s.FailNow("failed to start agent run command", err)
	}

	s.T().Log("Agent run command started")
	// wait for the agent and checks to start
	s.readUntil(stdout, "Running")

	// logging the process list for debugging purposes
	res, err := host.Execute("pgrep -fl datadog-agent")
	s.Require().NoError(err)
	s.T().Log(res)

	// get PID of the agent
	pids, err := process.FindPID(host, "datadog-agent")
	s.Require().NoError(err)
	s.T().Log(pids)

	s.Require().Len(pids, 1)
	pid := pids[0]

	// send ctrl+c to the agent
	_, err = host.Execute(fmt.Sprintf(`sudo kill -INT %d`, pid))
	s.Require().NoError(err)

	// verify it recives the stop command
	s.readUntil(stdout, "shutting")

	// wait for the agent to stop
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		pids, err := process.FindPID(host, "datadog-agent")
		s.T().Log(pids)
		// verify there is an error
		assert.Error(c, err)
	}, 1*time.Minute, 1*time.Second, "%s should be stopped", "datadog-agent")

	// restart the agent
	_, err = svcManager.Start("datadog-agent")
	s.Require().NoError(err)

	// wait for the agent to start
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady(), "agent should be running")
	}, 1*time.Minute, 1*time.Second, "%s should be ready", "datadog-agent")
}
