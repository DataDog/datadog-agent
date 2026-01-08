// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/ecsagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

type ECSEC2Suite struct {
	e2e.BaseSuite[ecsCPUStressEnv]
}

type ecsCPUStressEnv struct {
	environments.ECS
}

func ecsEC2CPUStressProvisioner(runInCoreAgent bool) provisioners.PulumiEnvRunFunc[ecsCPUStressEnv] {
	return func(ctx *pulumi.Context, env *ecsCPUStressEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		runParams := scenecs.GetRunParams(
			scenecs.WithECSOptions(scenecs.WithLinuxNodeGroup()),
			scenecs.WithAgentOptions(
				ecsagentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true"),
				ecsagentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", strconv.FormatBool(runInCoreAgent)),
			),
			scenecs.WithWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput) (*ecsComp.Workload, error) {
				return cpustress.EcsAppDefinition(e, clusterArn)
			}),
		)

		if err := scenecs.RunWithEnv(ctx, awsEnv, &env.ECS, runParams); err != nil {
			return err
		}

		return nil
	}
}

func TestECSEC2TestSuite(t *testing.T) {
	t.Parallel()
	s := ECSEC2Suite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		provisioners.NewTypedPulumiProvisioner("ecsEC2CPUStress", ecsEC2CPUStressProvisioner(false), nil))}

	e2e.Run(t, &s, e2eParams...)
}

func (s *ECSEC2Suite) TestProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		assertProcessCollectedNew(c, payloads, false, "stress-ng-cpu [run]")
		assertContainersCollectedNew(c, payloads, []string{"stress-ng"})
	}, 5*time.Minute, 10*time.Second)
}

// ECSEC2CoreAgentSuite runs the same test as ECSEC2Suite but with the process check running in the core agent
// This is duplicated as the tests have been flaky. This may be due to how pulumi is handling the provisioning of
// ecs tasks.
type ECSEC2CoreAgentSuite struct {
	e2e.BaseSuite[ecsCPUStressEnv]
}

func TestECSEC2CoreAgentSuite(t *testing.T) {
	t.Parallel()
	s := ECSEC2CoreAgentSuite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		provisioners.NewTypedPulumiProvisioner("ecsEC2CoreAgentCPUStress", ecsEC2CPUStressProvisioner(true), nil))}

	e2e.Run(t, &s, e2eParams...)
}

func (s *ECSEC2CoreAgentSuite) TestProcessCheckInCoreAgent() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")
		require.NotEmpty(c, payloads, "no process payloads returned")

		// Check just the last payload as the process-agent should terminate by itself after a while as we are
		// expecting the process checks to run in the core agent.
		payloads = payloads[len(payloads)-1:]
		requireProcessNotCollected(c, payloads, "process-agent")
	}, 5*time.Minute, 10*time.Second)

	// Flush the server to ensure payloads are received from the process checks that are running on the core agent
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		assertProcessCollectedNew(c, payloads, false, "stress-ng-cpu [run]")
		assertContainersCollectedNew(c, payloads, []string{"stress-ng"})
	}, 5*time.Minute, 10*time.Second)
}
