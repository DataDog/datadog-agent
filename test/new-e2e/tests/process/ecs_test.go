// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/ecsagentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"
)

type ECSSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestECSTestSuite(t *testing.T) {
	agentOpts := []ecsagentparams.Option{
		ecsagentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true"),
	}

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(
			ecs.Provisioner(
				ecs.WithECSLinuxECSOptimizedNodeGroup(),
				ecs.WithAgentOptions(agentOpts...)),
		),
	}

	var ecsEnv ECSSuite
	e2e.Run(t, &ecsEnv, options...)

	// TODO: Get the CPU workload working
	_, err := cpustress.EcsAppDefinition(*ecsEnv.Env().AwsEnvironment, ecsEnv.Env().ClusterArn)
	require.NoError(t, err)
}

func (s *ECSSuite) TestECSProcessCheck() {
	t := s.T()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "dd")
	assertContainersCollected(t, payloads, []string{"fake-process"})
}
