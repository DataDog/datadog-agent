// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"strings"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

type ECSFargateSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func getFargateProvisioner(configMap runner.ConfigMap) provisioners.TypedProvisioner[environments.ECS] {
	return ecs.Provisioner(
		ecs.WithRunOptions(
			scenecs.WithECSOptions(scenecs.WithFargateCapacityProvider()),
			scenecs.WithFargateWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake) (*ecsComp.Workload, error) {
				return cpustress.FargateAppDefinition(e, clusterArn, apiKeySSMParamName, fakeIntake)
			}),
		),
		ecs.WithExtraConfigParams(configMap),
	)
}

func TestECSFargateTestSuite(t *testing.T) {
	t.Parallel()
	s := ECSFargateSuite{}

	extraConfig := runner.ConfigMap{
		"ddagent:extraEnvVars": auto.ConfigValue{Value: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED=false"},
	}

	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		getFargateProvisioner(extraConfig),
	),
	}

	e2e.Run(t, &s, e2eParams...)
}

func (s *ECSFargateSuite) TestProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		assertProcessCollectedNew(c, payloads, false, "stress-ng-cpu [run]")
		assertProcessCollectedNew(c, payloads, false, "process-agent")
		assertContainersCollectedNew(c, payloads, []string{"stress-ng"})
		assertContainerStates(c, payloads, map[string]agentmodel.ContainerState{
			"stress-ng": agentmodel.ContainerState_running,
		})
		assertFargateHostname(t, payloads)
	}, 5*time.Minute, 10*time.Second)
}

type ECSFargateCoreAgentSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestECSFargateCoreAgentTestSuite(t *testing.T) {
	t.Parallel()
	s := ECSFargateCoreAgentSuite{}

	extraConfig := runner.ConfigMap{
		"ddagent:extraEnvVars": auto.ConfigValue{Value: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED=true"},
	}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		getFargateProvisioner(extraConfig),
	),
	}

	e2e.Run(t, &s, e2eParams...)
}

func (s *ECSFargateCoreAgentSuite) TestProcessCheckInCoreAgent() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		assertProcessCollectedNew(c, payloads, false, "stress-ng-cpu [run]")
		requireProcessNotCollected(c, payloads, "process-agent")
		assertContainersCollectedNew(c, payloads, []string{"stress-ng"})
		assertContainerStates(c, payloads, map[string]agentmodel.ContainerState{
			"stress-ng": agentmodel.ContainerState_running,
		})
		assertFargateHostname(t, payloads)
	}, 5*time.Minute, 10*time.Second)
}

func assertFargateHostname(t assert.TestingT, payloads []*aggregator.ProcessPayload) {
	for _, payload := range payloads {
		assert.Truef(t, strings.HasPrefix(payload.HostName, "fargate_task:"),
			"hostname expected to start with 'fargate_task:', but got '%s'", payload.HostName)
	}
}
