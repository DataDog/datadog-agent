// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/ecsagentparams"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	ecsComp "github.com/DataDog/test-infra-definitions/components/ecs"
	tifEcs "github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"
)

type ECSEC2Suite struct {
	e2e.BaseSuite[ecsCPUStressEnv]
}

type ecsCPUStressEnv struct {
	environments.ECS
}

func ecsEC2CPUStressProvisioner(runInCoreAgent bool) e2e.PulumiEnvRunFunc[ecsCPUStressEnv] {
	return func(ctx *pulumi.Context, env *ecsCPUStressEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		params := ecs.GetProvisionerParams(
			ecs.WithAwsEnv(&awsEnv),
			ecs.WithECSOptions(tifEcs.WithLinuxNodeGroup()),
			ecs.WithAgentOptions(
				ecsagentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true"),
				ecsagentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", fmt.Sprintf("%t", runInCoreAgent)),
			),
			ecs.WithWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput) (*ecsComp.Workload, error) {
				return cpustress.EcsAppDefinition(e, clusterArn)
			}),
		)

		if err := ecs.Run(ctx, &env.ECS, params); err != nil {
			return err
		}

		return nil
	}
}

func TestECSEC2TestSuite(t *testing.T) {
	t.Parallel()
	s := ECSEC2Suite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		e2e.NewTypedPulumiProvisioner("ecsEC2CPUStress", ecsEC2CPUStressProvisioner(false), nil))}

	e2e.Run(t, &s, e2eParams...)
}

func (s *ECSEC2Suite) TestProcessCheck() {
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

func (s *ECSEC2Suite) TestProcessCheckInCoreAgent() {
	t := s.T()
	// PROCS-4219
	flake.Mark(t)

	s.UpdateEnv(e2e.NewTypedPulumiProvisioner("ecsEC2CPUStress", ecsEC2CPUStressProvisioner(true), nil))

	// Flush fake intake to remove any payloads which may have
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "stress-ng-cpu [run]")
	requireProcessNotCollected(t, payloads, "process-agent")
	assertContainersCollected(t, payloads, []string{"stress-ng"})
}
