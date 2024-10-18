// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/ecsagentparams"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"
)

type ECSFargateSuite struct {
	e2e.BaseSuite[fargateCPUStressEnv]
}

type fargateCPUStressEnv struct {
	environments.ECS
}

func ecsFargateCPUStressProvisioner() e2e.PulumiEnvRunFunc[fargateCPUStressEnv] {
	return func(ctx *pulumi.Context, env *fargateCPUStressEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		params := ecs.GetProvisionerParams(
			ecs.WithAwsEnv(&awsEnv),
			ecs.WithECSFargateCapacityProvider(),
			ecs.WithAgentOptions(
				ecsagentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true"),
			),
		)

		if err := ecs.Run(ctx, &env.ECS, params); err != nil {
			return err
		}

		return nil
	}
}

func TestECSFargateTestSuite(t *testing.T) {
	t.Parallel()
	s := ECSFargateSuite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		e2e.NewTypedPulumiProvisioner("ecsFargateCPUStress", ecsFargateCPUStressProvisioner(), nil),
	),
		e2e.WithDevMode(),
	}

	e2e.Run(t, &s, e2eParams...)
}

func (s *ECSFargateSuite) TestProcessCheck() {
	t := s.T()
	// PROCS-4219
	flake.Mark(t)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "stress-ng-cpu [run]")
	assertContainersCollected(t, payloads, []string{"stress-ng"})
}
