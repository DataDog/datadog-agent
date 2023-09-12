// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
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
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
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
	fakeintake := s.Env().Fakeintake
	// part 1: no logs
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("custom_logs")
		assert.NoError(c, err)
		assert.Equal(c, len(logs), 0, "logs received while none expected")
	}, 5*time.Minute, 10*time.Second)
	// part 2: generate logs
	s.Env().VM.Execute("echo 'totoro' > /tmp/test.log")
	// part 3: there should be logs
	s.EventuallyWithT(func(c *assert.CollectT) {
		names, err := fakeintake.GetLogServiceNames()
		assert.NoError(c, err)
		assert.Greater(c, len(names), 0, "no logs received")
		logs, err := fakeintake.FilterLogs("custom_logs")
		assert.NoError(c, err)
		assert.Equal(c, len(logs), 1, "expecting 1 log from 'custom_logs'")
		logs, err = fakeintake.FilterLogs("custom_logs", fi.WithMessageContaining("totoro"))
		assert.NoError(c, err)
		assert.Equal(c, len(logs), 1, "expecting 1 log from 'custom_logs' with 'totoro' content")
	}, 5*time.Minute, 10*time.Second)
}
