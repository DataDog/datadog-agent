// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"context"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"testing"
)

type linuxRunSuite struct {
	baseRunSuite
}

func TestLinuxRunSuite(t *testing.T) {
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
