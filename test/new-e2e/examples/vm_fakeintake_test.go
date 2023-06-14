// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"testing"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
	ec2vm "github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2VM"
	"github.com/cenkalti/backoff/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vmFakeintakeSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, e2e.AgentStackDef(nil))
}

func (s *vmFakeintakeSuite) TestVM() {
	output := s.Env().VM.Execute("ls")
	require.NotEmpty(s.T(), output)
}

func (s *vmFakeintakeSuite) TestAgent() {
	err := s.Env().Agent.WaitForReady()
	require.NoError(s.T(), err)
	output := s.Env().Agent.Status()
	require.Contains(s.T(), output.Content, "Getting the status from the agent")
	isReady, err := s.Env().Agent.IsReady()
	require.NoError(s.T(), err)
	assert.True(s.T(), isReady, "Agent is not ready")
}

func (s *vmFakeintakeSuite) TestMetrics() {
	t := s.T()
	err := backoff.Retry(func() error {
		metrics, err := s.Env().Fakeintake.Client.FilterMetrics("system.uptime")
		if err != nil {
			return err
		}
		if len(metrics) == 0 {
			return errors.New("No metrics yet")
		}
		if metrics[len(metrics)-1].Points[len(metrics[len(metrics)-1].Points)-1].Value == 0 {
			return errors.New("")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	require.NoError(t, err)
}

func (s *vmFakeintakeSuite) TestCheckRuns() {
	t := s.T()
	err := backoff.Retry(func() error {
		checkRuns, err := s.Env().Fakeintake.Client.GetCheckRun("datadog.agent.up")
		if err != nil {
			return err
		}
		if len(checkRuns) == 0 {
			return errors.New("No check run yet")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	require.NoError(t, err)
}

func LogsExampleStackDef(vmParams []e2e.Ec2VMOption, agentParams ...func(*agent.Params) error) *e2e.StackDefinition[e2e.AgentEnv] {
	return e2e.EnvFactoryStackDef(
		func(ctx *pulumi.Context) (*e2e.AgentEnv, error) {
			vm, err := ec2vm.NewEc2VM(ctx, vmParams...)
			if err != nil {
				return nil, err
			}

			fakeintakeExporter, err := ecs.NewEcsFakeintake(vm.Infra)
			if err != nil {
				return nil, err
			}

			agentParams = append(agentParams, agent.WithFakeintake(fakeintakeExporter))
			agentParams = append(agentParams, agent.WithIntegration("custom_logs.d", `logs:
- type: file
  path: "/tmp/test.log"
  service: "custom_logs"
  source: "custom"`))
			agentParams = append(agentParams, agent.WithLogs())

			installer, err := agent.NewInstaller(vm, agentParams...)
			if err != nil {
				return nil, err
			}
			return &e2e.AgentEnv{
				VM:         client.NewVM(vm),
				Agent:      client.NewAgent(installer),
				Fakeintake: client.NewFakeintake(fakeintakeExporter),
			}, nil
		},
	)
}

func (s *vmFakeintakeSuite) TestLogs() {
	s.UpdateEnv(LogsExampleStackDef(nil))
	t := s.T()
	fakeintake := s.Env().Fakeintake
	err := backoff.Retry(func() error {
		logs, err := fakeintake.FilterLogs("custom_logs")
		if err != nil {
			return err
		}
		if len(logs) != 0 {
			return errors.New("logs received while none expected")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	require.NoError(t, err)
	_, err = s.Env().VM.ExecuteWithError("echo 'totoro' > /tmp/test.log")
	require.NoError(t, err)
	err = backoff.Retry(func() error {
		names, err := fakeintake.GetLogServiceNames()
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return errors.New("no logs received")
		}
		logs, err := fakeintake.FilterLogs("custom_logs")
		if err != nil {
			return err
		}
		if len(logs) != 1 {
			return errors.New("no logs received")
		}
		logs, err = fakeintake.FilterLogs("custom_logs", fi.WithMessageContaining("totoro"))
		if err != nil {
			return err
		}
		if len(logs) != 1 {
			return fmt.Errorf("received %v logs with 'tororo', expecting 1", len(logs))
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))

	require.NoError(t, err)
}
