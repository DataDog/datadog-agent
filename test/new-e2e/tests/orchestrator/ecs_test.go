package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
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

func extraAgentEnv() string {
	envs := []string{
		"DD_ECS_TASK_COLLECTION_ENABLED=true",
		"DD_ORCHESTRATOR_EXPLORER_ENABLED=true",
		"DD_ORCHESTRATOR_EXPLORER_ECS_COLLECTION_ENABLED=true",
	}
	return strings.Join(envs, ",")
}

func (suite *ecsSuite) SetupSuite() {
	ctx := context.Background()

	// Creating the stack
	// https://github.com/DataDog/test-infra-definitions/blob/main/resources/aws/environment.go#L13-L58
	stackConfig := runner.ConfigMap{
		"ddinfra:aws/ecs/linuxECSOptimizedNodeGroup": auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/fargateCapacityProvider":    auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/linuxBottlerocketNodeGroup": auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/windowsLTSCNodeGroup":       auto.ConfigValue{Value: "false"},

		"ddagent:deploy":       auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":   auto.ConfigValue{Value: "true"},
		"ddagent:extraEnvVars": auto.ConfigValue{Value: extraAgentEnv()},

		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
	}

	if runner.GetProfile().AllowDevMode() && *replaceStacks {
		fmt.Fprintln(os.Stderr, "Destroying existing stack")
		err := infra.GetStackManager().DeleteStack(ctx, "ecs-cluster", nil)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
		}
	}

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "ecs-cluster", stackConfig, ecs.Run, false, nil)
	if !suite.Assert().NoError(err) {
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, "ecs-cluster", nil)
		}
		suite.T().FailNow()
	}

	fakeintake := &components.FakeIntake{}
	fiSerialized, err := json.Marshal(stackOutput.Outputs["dd-Fakeintake-aws-ecs"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(fakeintake.Import(fiSerialized, fakeintake))
	suite.Require().NoError(fakeintake.Init(suite))
	suite.FakeIntake = fakeintake.Client()

	suite.ecsClusterName = stackOutput.Outputs["ecs-cluster-name"].Value.(string)

	suite.waitForAllTasksReady()

}

func (suite *ecsSuite) waitForAllTasksReady() {
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	suite.Require().NoErrorf(err, "Failed to load AWS config")

	client := awsecs.NewFromConfig(cfg)

	suite.Run("ECS tasks are ready", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			var initToken string
			for nextToken := &initToken; nextToken != nil; {
				if nextToken == &initToken {
					nextToken = nil
				}

				servicesList, err := client.ListServices(ctx, &awsecs.ListServicesInput{
					Cluster:    &suite.ecsClusterName,
					MaxResults: pointer.Ptr(int32(10)), // Because `DescribeServices` takes at most 10 services in input
					NextToken:  nextToken,
				})
				// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
				if !assert.NoErrorf(c, err, "Failed to list ECS services") {
					return
				}

				nextToken = servicesList.NextToken

				servicesDescription, err := client.DescribeServices(ctx, &awsecs.DescribeServicesInput{
					Cluster:  &suite.ecsClusterName,
					Services: servicesList.ServiceArns,
				})
				if !assert.NoErrorf(c, err, "Failed to describe ECS services %v", servicesList.ServiceArns) {
					continue
				}

				for _, serviceDescription := range servicesDescription.Services {
					assert.NotZerof(c, serviceDescription.DesiredCount, "ECS service %s has no task", *serviceDescription.ServiceName)

					for nextToken := &initToken; nextToken != nil; {
						if nextToken == &initToken {
							nextToken = nil
						}

						tasksList, err := client.ListTasks(ctx, &awsecs.ListTasksInput{
							Cluster:       &suite.ecsClusterName,
							ServiceName:   serviceDescription.ServiceName,
							DesiredStatus: awsecstypes.DesiredStatusRunning,
							MaxResults:    pointer.Ptr(int32(100)), // Because `DescribeTasks` takes at most 100 tasks in input
							NextToken:     nextToken,
						})
						if !assert.NoErrorf(c, err, "Failed to list ECS tasks for service %s", *serviceDescription.ServiceName) {
							break
						}

						nextToken = tasksList.NextToken

						tasksDescription, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
							Cluster: &suite.ecsClusterName,
							Tasks:   tasksList.TaskArns,
						})
						if !assert.NoErrorf(c, err, "Failed to describe ECS tasks %v", tasksList.TaskArns) {
							continue
						}

						for _, taskDescription := range tasksDescription.Tasks {
							assert.Equalf(c, string(awsecstypes.DesiredStatusRunning), *taskDescription.LastStatus,
								"Task %s of service %s is not running", *taskDescription.TaskArn, *serviceDescription.ServiceName)
							assert.NotEqualf(c, awsecstypes.HealthStatusUnhealthy, taskDescription.HealthStatus,
								"Task %s of service %s is unhealthy", *taskDescription.TaskArn, *serviceDescription.ServiceName)
						}
					}
				}
			}
		}, 5*time.Minute, 10*time.Second, "Not all tasks became ready in time.")
	})
}

func (suite *ecsSuite) TearDownSuite() {
	summarizeResources(suite.FakeIntake)
}

func (suite *ecsSuite) TestECSEC2Task() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			suite.commonTest(payload)
			if payload.ECSTask.LaunchType == "ec2" {
				return true
			}
			return false
		},
		message: "receive ecs ec2 task payload",
		timeout: defaultTimeout,
	}.Assert(suite)
}

func (suite *ecsSuite) TestECSFargateTask() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: process.TypeCollectorECSTask},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			suite.commonTest(payload)
			if payload.ECSTask.LaunchType == "fargate" {
				return true
			}
			return false
		},
		message: "receive ecs fargate task payload",
		timeout: defaultTimeout,
	}.Assert(suite)
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
	suite.Require().NotEmpty(payload.ECSTask.Containers)
}
