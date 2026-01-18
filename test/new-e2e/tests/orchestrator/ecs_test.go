// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

type ecsSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestECSSuite(t *testing.T) {
	t.Parallel()
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsecs.Provisioner(
			awsecs.WithRunOptions(
				ecs.WithWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput) (*ecsComp.Workload, error) {
					return cpustress.EcsAppDefinition(e, clusterArn)
				}),
				ecs.WithFargateWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake) (*ecsComp.Workload, error) {
					return cpustress.FargateAppDefinition(e, clusterArn, apiKeySSMParamName, fakeIntake)
				}),
				ecs.WithECSOptions(
					ecs.WithFargateCapacityProvider(),
					ecs.WithLinuxNodeGroup(),
				),
			))),
	}
	e2e.Run(t, &ecsSuite{}, options...)
}

func (suite *ecsSuite) TestECSEC2Task() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return commonTest(payload) && payload.ECSTask.LaunchType == "ec2"
		},
		message: "receive ecs ec2 task payload",
		timeout: 20 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client())
}

func (suite *ecsSuite) TestECSFargateTask() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return commonTest(payload) && payload.ECSTask.LaunchType == "fargate"
		},
		message: "receive ecs fargate task payload",
		timeout: 20 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client())
}

func (suite *ecsSuite) TestAgentVersion() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return payload.ECSTask.LaunchType == "fargate" && payload.ECSTaskParentCollector.AgentVersion != nil
		},
		message: "receive ecs fargate task payload with agent version",
		timeout: 20 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client())

	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return payload.ECSTask.LaunchType == "ec2" && payload.ECSTaskParentCollector.AgentVersion != nil
		},
		message: "receive ecs ec2 task payload with agent version",
		timeout: 20 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client())
}

func commonTest(payload *aggregator.OrchestratorPayload) bool {
	if payload == nil || payload.ECSTaskParentCollector == nil || payload.ECSTask == nil {
		return false
	}

	if payload.Name == "" || payload.UID == "" ||
		payload.ECSTask.Arn == "" || payload.ECSTask.Family == "" ||
		payload.ECSTask.Version == "" || len(payload.ECSTask.Containers) == 0 {
		return false
	}

	if payload.Type != process.MessageType(process.TypeCollectorECSTask) {
		return false
	}

	for _, container := range payload.ECSTask.Containers {
		if container.DockerID == "" || container.DockerName == "" || container.Image == "" {
			return false
		}
	}

	return true
}
