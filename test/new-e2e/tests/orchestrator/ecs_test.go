// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsecs "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/ecs"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
)

type ecsSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestECSSuite(t *testing.T) {
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsecs.Provisioner(
			awsecs.WithTestingWorkload(),
			awsecs.WithECSOptions(
				ecs.WithFargateCapacityProvider(),
				ecs.WithLinuxNodeGroup(),
			),
		)),
	}
	e2e.Run(t, &ecsSuite{}, options...)
}

func (suite *ecsSuite) TestECSEC2Task() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			commonTest(suite.T(), payload)
			return payload.ECSTask.LaunchType == "ec2"
		},
		message: "receive ecs ec2 task payload",
		timeout: 20 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client())
}

func (suite *ecsSuite) TestECSFargateTask() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			commonTest(suite.T(), payload)
			return payload.ECSTask.LaunchType == "fargate"
		},
		message: "receive ecs fargate task payload",
		timeout: 20 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client())
}

func commonTest(t *testing.T, payload *aggregator.OrchestratorPayload) {
	require.NotNil(t, payload)
	require.NotNil(t, payload.ECSTaskParentCollector)
	require.NotNil(t, payload.ECSTask)

	require.NotEmpty(t, payload.Name)
	require.NotEmpty(t, payload.UID)
	require.NotEmpty(t, payload.ECSTask.Arn)
	require.NotEmpty(t, payload.ECSTask.Family)
	require.NotEmpty(t, payload.ECSTask.Version)
	require.NotEmpty(t, payload.ECSTask.Containers)

	require.Equal(t, process.MessageType(process.TypeCollectorECSTask), payload.Type)

	for _, container := range payload.ECSTask.Containers {
		require.NotEmpty(t, container.DockerID)
		require.NotEmpty(t, container.DockerName)
		require.NotEmpty(t, container.Image)
	}
}
