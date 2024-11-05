// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"

	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/test-infra-definitions/components/ecs"
	tifEcs "github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"
)

type ECSFargateSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func getFargateProvisioner(configMap runner.ConfigMap) e2e.TypedProvisioner[environments.ECS] {
	return ecs.Provisioner(
		ecs.WithECSOptions(tifEcs.WithFargateCapacityProvider()),
		ecs.WithFargateWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake) (*ecsComp.Workload, error) {
			return cpustress.FargateAppDefinition(e, clusterArn, apiKeySSMParamName, fakeIntake)
		}),
		ecs.WithExtraConfigParams(configMap),
	)
}

func TestECSFargateTestSuite(t *testing.T) {
	t.Parallel()
	s := ECSFargateSuite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		getFargateProvisioner(nil),
	),
	}

	e2e.Run(t, &s, e2eParams...)
}

func (s *ECSFargateSuite) TestProcessCheck() {
	t := s.T()
	// PROCS-4219
	flake.Mark(t)

	// Flush fake intake to remove any payloads which may have
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 5*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "stress-ng-cpu [run]")
	assertContainersCollected(t, payloads, []string{"stress-ng"})
}

func (s *ECSFargateSuite) TestProcessCheckInCoreAgent() {
	t := s.T()
	// PROCS-4219
	flake.Mark(t)

	extraConfig := runner.ConfigMap{
		"ddagent:extraEnvVars": auto.ConfigValue{Value: "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED=true"},
	}

	s.UpdateEnv(getFargateProvisioner(extraConfig))

	// Flush fake intake to remove any payloads which may have
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 5*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "stress-ng-cpu [run]")
	requireProcessNotCollected(t, payloads, "process-agent")
	assertContainersCollected(t, payloads, []string{"stress-ng"})
}
