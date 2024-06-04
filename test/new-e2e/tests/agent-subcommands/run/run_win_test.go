// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package status

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type windowsRunSuite struct {
	baseRunSuite
}

func TestWindowsRunSuite(t *testing.T) {
	e2e.Run(t, &windowsRunSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (s *windowsRunSuite) TestRunWhenAgentAlreadyRunning() {
	host := s.Env().RemoteHost
	s.T().Log(host.HostOutput)

	// Ensure agent is running
	s.Require().True(s.Env().Agent.Client.IsReady(), "agent should be running")

	// execute the `agent run` subcommand
	path, err := windowsAgent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
	agentPath := filepath.Join(path, "bin", "agent.exe")
	cmd := fmt.Sprintf(`& "%s" run`, agentPath)
	// run command with timeout in case it succeeds/hangs
	var out string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go func() {
		out, err = host.Execute(cmd)
		cancel()
	}()
	<-ctx.Done()
	s.Require().ErrorIs(ctx.Err(), context.Canceled, "agent run command timed out")
	if err == nil {
		s.T().Log(out)
		s.FailNow("agent run command succeeded when it should have failed")
	}
	// make sure it didn't panic
	s.Require().NotContains(err.Error(), "panic: runtime error")
	// make sure it printed a reasonable human readable error
	s.Require().ErrorContains(err, "listen tcp 127.0.0.1:5001: bind: Only one usage of each socket address")
	// TODO: Once host.Execute is fixed to return the exit code, check that the exit code is ??
}
