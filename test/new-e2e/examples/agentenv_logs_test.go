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
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/cenkalti/backoff/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
)

type vmFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func logsExampleStackDef(vmParams []ec2params.Option, agentParams ...agentparams.Option) *e2e.StackDefinition[e2e.FakeIntakeEnv] {
	return e2e.EnvFactoryStackDef(
		func(ctx *pulumi.Context) (*e2e.FakeIntakeEnv, error) {
			vm, err := ec2vm.NewEc2VM(ctx, vmParams...)
			if err != nil {
				return nil, err
			}

			fakeintakeExporter, err := aws.NewEcsFakeintake(vm.GetAwsEnvironment())
			if err != nil {
				return nil, err
			}

			agentParams = append(agentParams, agentparams.WithFakeintake(fakeintakeExporter))
			agentParams = append(agentParams, agentparams.WithIntegration("custom_logs.d", `logs:
- type: file
  path: "/tmp/test.log"
  service: "custom_logs"
  source: "custom"`))
			agentParams = append(agentParams, agentparams.WithLogs())

			installer, err := agent.NewInstaller(vm, agentParams...)
			if err != nil {
				return nil, err
			}
			return &e2e.FakeIntakeEnv{
				VM:         client.NewVM(vm),
				Agent:      client.NewAgent(installer),
				Fakeintake: client.NewFakeintake(fakeintakeExporter),
			}, nil
		},
	)
}

func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, logsExampleStackDef(nil))
}

func (s *vmFakeintakeSuite) TestLogs() {
	t := s.T()
	fakeintake := s.Env().Fakeintake
	// part 1: no logs
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
	// part 2: generate logs
	require.NoError(t, err)
	_, err = s.Env().VM.ExecuteWithError("echo 'totoro' > /tmp/test.log")
	require.NoError(t, err)
	// part 3: there should be logs
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
