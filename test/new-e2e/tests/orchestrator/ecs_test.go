// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
)

type ecsSuite struct {
	suite.Suite
	FakeIntake     *fakeintake.Client
	ecsClusterName string
}

func (suite *ecsSuite) GetFakeIntakeClient() *fakeintake.Client {
	return suite.FakeIntake
}

func TestECSSuite(t *testing.T) {
	suite.Run(t, &ecsSuite{})
}

func (suite *ecsSuite) SetupSuite() {
	ctx := context.Background()

	// Creating the stack
	// https://github.com/DataDog/test-infra-definitions/blob/main/resources/aws/environment.go#L13-L58
	stackConfig := runner.ConfigMap{
		"ddinfra:aws/ecs/linuxECSOptimizedNodeGroup": auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/fargateCapacityProvider":    auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/linuxBottlerocketNodeGroup": auto.ConfigValue{Value: "true"},

		"ddagent:deploy":     auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake": auto.ConfigValue{Value: "true"},

		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
	}

	if runner.GetProfile().AllowDevMode() && *replaceStacks {
		fmt.Fprintln(os.Stderr, "Destroying existing stack")
		err := infra.GetStackManager().DeleteStack(ctx, "ecs-cluster", nil)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
		}
	}

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "orch-ecs-cluster", ecs.Run, infra.WithConfigMap(stackConfig))
	if !suite.Assert().NoError(err) {
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, "ecs-cluster", nil)
		}
		suite.T().FailNow()
	}

	intake := &components.FakeIntake{}
	fiSerialized, err := json.Marshal(stackOutput.Outputs["dd-Fakeintake-aws-ecs"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(intake.Import(fiSerialized, intake))
	suite.Require().NoError(intake.Init(suite))
	suite.FakeIntake = intake.Client()

	suite.ecsClusterName = stackOutput.Outputs["ecs-cluster-name"].Value.(string)
}

func (suite *ecsSuite) TearDownSuite() {
	summarizeResources(suite.FakeIntake)
}

func (suite *ecsSuite) TestECSEC2Task() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			suite.commonTest(payload)
			return payload.ECSTask.LaunchType == "ec2"
		},
		message: "receive ecs ec2 task payload",
		timeout: 20 * time.Minute,
	}.Assert(suite, suite.T())
}

func (suite *ecsSuite) TestECSFargateTask() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			suite.commonTest(payload)
			return payload.ECSTask.LaunchType == "fargate"
		},
		message: "receive ecs fargate task payload",
		timeout: 20 * time.Minute,
	}.Assert(suite, suite.T())
}

func (suite *ecsSuite) commonTest(payload *aggregator.OrchestratorPayload) {
	suite.Require().NotNil(payload)
	suite.Require().Equal(process.MessageType(process.TypeCollectorECSTask), payload.Type)
	suite.Require().NotEmpty(payload.Name)
	suite.Require().NotEmpty(payload.UID)
	suite.Require().NotNil(payload.ECSTaskParentCollector)

	suite.Require().NotNil(payload.ECSTask)
	suite.Require().NotEmpty(payload.ECSTask.Arn)
	suite.Require().NotEmpty(payload.ECSTask.Family)
	suite.Require().NotEmpty(payload.ECSTask.Version)
	suite.Require().Contains([]string{"ec2", "fargate"}, payload.ECSTask.LaunchType)
	suite.Require().NotEmpty(payload.ECSTask.Containers)

	for _, container := range payload.ECSTask.Containers {
		require.NotEmpty(suite.T(), container.DockerID)
		require.NotEmpty(suite.T(), container.DockerName)
		require.NotEmpty(suite.T(), container.Image)
	}

	switch payload.ECSTask.LaunchType {
	case "ec2":
		// service name is only present for EC2 tasks
		suite.Require().NotEmpty(payload.ECSTask.ServiceName)
	case "fargate":
		// check if datadog-agent container is present
		containerNames := make([]string, 0, len(payload.ECSTask.Containers))
		for _, container := range payload.ECSTask.Containers {
			containerNames = append(containerNames, container.Name)
		}
		suite.Require().Contains(containerNames, "datadog-agent")
	default:
		suite.Failf("unexpected launch type", "unexpected launch type %s", payload.ECSTask.LaunchType)
	}
}
