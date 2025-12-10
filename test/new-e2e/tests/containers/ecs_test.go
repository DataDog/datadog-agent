// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containers provides foundational ECS infrastructure tests.
//
// This file contains the base test suite for ECS environments that ensures
// the test infrastructure is ready before running ECS-specific tests.
//
// For comprehensive ECS-specific tests covering APM, logs, configuration,
// resilience, and platform features, see test/new-e2e/tests/ecs/*.
package containers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

type ecsSuite struct {
	baseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSSuite(t *testing.T) {
	e2e.Run(t, &ecsSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithFargateCapacityProvider(),
				scenecs.WithLinuxNodeGroup(),
				scenecs.WithWindowsNodeGroup(),
				scenecs.WithLinuxBottleRocketNodeGroup(),
			),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

func (suite *ecsSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/mnw-tdr-jd8/e2e-tests-containers-ecs?refresh_mode=paused&tpl_var_ecs_cluster_name%%5B0%%5D=%s&tpl_var_fake_intake_task_family%%5B0%%5D=%s-fakeintake-ecs&from_ts=%d&to_ts=%d&live=false",
		suite.ecsClusterName,
		strings.TrimSuffix(suite.ecsClusterName, "-ecs"),
		suite.StartTime().UnixMilli(),
		suite.EndTime().UnixMilli(),
	))
}

// Test00UpAndRunning is a foundation test that ensures all ECS tasks and services
// are in RUNNING state before other tests execute.
//
// Once pulumi has finished creating a stack, it can still take some time for the images to be pulled,
// for the containers to be started, for the agent collectors to collect workload information
// and to feed workload meta and the tagger.
//
// We could increase the timeout of all tests to cope with the agent tagger warmup time.
// But in case of a single bug making a single tag missing from every metric,
// all the tests would time out and that would be a waste of time.
//
// It's better to have the first test having a long timeout to wait for the agent to warmup,
// and to have the following tests with a smaller timeout.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The 00 in Test00UpAndRunning is here to guarantee that this test, waiting for all tasks to be ready
// is run first.
func (suite *ecsSuite) Test00UpAndRunning() {
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
				// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
				if !assert.NoErrorf(c, err, "Failed to describe ECS services") {
					return
				}

				for _, service := range servicesDescription.Services {
					tasksList, err := client.ListTasks(ctx, &awsecs.ListTasksInput{
						Cluster:       service.ClusterArn,
						ServiceName:   service.ServiceName,
						DesiredStatus: awsecstypes.DesiredStatusRunning,
					})
					// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
					if !assert.NoErrorf(c, err, "Failed to list tasks for service %s", *service.ServiceName) {
						return
					}

					tasksDescription, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
						Cluster: service.ClusterArn,
						Tasks:   tasksList.TaskArns,
					})
					// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
					if !assert.NoErrorf(c, err, "Failed to describe tasks for service %s", *service.ServiceName) {
						return
					}

					runningTasks := lo.CountBy(tasksDescription.Tasks, func(task awsecstypes.Task) bool {
						return task.LastStatus != nil && *task.LastStatus == "RUNNING"
					})
					desiredTasks := service.DesiredCount

					if !assert.Equalf(c, int(desiredTasks), runningTasks, "Service %s: expected %d tasks to be running, got %d", *service.ServiceName, desiredTasks, runningTasks) {
						return
					}
				}
			}
		}, 15*time.Minute, 10*time.Second, "All ECS services should be ready")
	})
}
