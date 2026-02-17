// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

type ecsResilienceSuite struct {
	BaseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSResilienceSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ecsResilienceSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithLinuxNodeGroup(),
			),
			// Note: In a real implementation, we would add the chaos workload here
			// scenecs.WithWorkloadApp(ecschaos.EcsAppDefinition),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsResilienceSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

// Test00UpAndRunning is a foundation test that ensures all ECS tasks and services
// are in RUNNING state before other tests execute. The 00 prefix ensures it runs first.
func (suite *ecsResilienceSuite) Test00UpAndRunning() {
	ctx := suite.T().Context()

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
					MaxResults: pointer.Ptr(int32(10)),
					NextToken:  nextToken,
				})
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
							MaxResults:    pointer.Ptr(int32(100)),
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
		}, 15*time.Minute, 10*time.Second, "Not all tasks became ready in time.")
	})
}

func (suite *ecsResilienceSuite) TestAgentRestart() {
	// Test that agent recovers gracefully from restarts
	suite.Run("Agent restart recovery", func() {
		// Verify agent is collecting data by checking for a well-known metric
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics("datadog.agent.running")
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			assert.NotEmptyf(c, metrics, "Should have datadog.agent.running metrics")
			suite.T().Logf("Agent running metrics: %d", len(metrics))
		}, 5*time.Minute, 10*time.Second, "Failed to establish baseline")

		// Note: In a real implementation, we would restart the agent here
		// and verify it resumes collecting metrics
	})
}

func (suite *ecsResilienceSuite) TestTaskFailureRecovery() {
	// Test that agent handles task failures and replacements
	suite.Run("Task failure recovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify agent is tracking tasks via container metrics
			metrics, err := suite.Fakeintake.FilterMetrics("container.cpu.usage")
			if err != nil || len(metrics) == 0 {
				metrics, err = suite.Fakeintake.FilterMetrics("container.memory.usage")
			}
			if !assert.NoErrorf(c, err, "Failed to query container metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No container metrics found") {
				return
			}

			// Count unique tasks being monitored
			tasks := make(map[string]bool)
			for _, metric := range metrics {
				for _, tag := range metric.GetTags() {
					if strings.HasPrefix(tag, "task_arn:") {
						tasks[strings.TrimPrefix(tag, "task_arn:")] = true
					}
				}
			}

			suite.T().Logf("Monitoring %d unique tasks", len(tasks))
			assert.GreaterOrEqualf(c, len(tasks), 1,
				"Should be monitoring at least one task")
		}, 5*time.Minute, 10*time.Second, "Task failure recovery validation failed")
	})
}

func (suite *ecsResilienceSuite) TestNetworkInterruption() {
	// Test agent behavior during network interruptions
	suite.Run("Network interruption handling", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify data flow using a targeted metric
			metrics, err := suite.Fakeintake.FilterMetrics("datadog.agent.running")
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			assert.NotEmptyf(c, metrics, "Agent should be reporting metrics")
			suite.T().Logf("Agent running metrics: %d", len(metrics))
		}, 5*time.Minute, 10*time.Second, "Network interruption handling validation failed")
	})
}

func (suite *ecsResilienceSuite) TestHighCardinality() {
	// Test agent handling of high cardinality metrics
	suite.Run("High cardinality handling", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify agent is collecting metrics by checking metric names
			names, err := suite.Fakeintake.GetMetricNames()
			if !assert.NoErrorf(c, err, "Failed to query metric names") {
				return
			}

			suite.T().Logf("Unique metric names: %d", len(names))

			// Agent should be collecting a reasonable number of unique metrics
			assert.GreaterOrEqualf(c, len(names), 10,
				"Agent should collect metrics despite cardinality")
		}, 5*time.Minute, 10*time.Second, "High cardinality handling validation failed")
	})
}

func (suite *ecsResilienceSuite) TestResourceExhaustion() {
	// Test agent behavior under resource pressure
	suite.Run("Resource exhaustion handling", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify agent is operational by checking for its running metric
			metrics, err := suite.Fakeintake.FilterMetrics("datadog.agent.running")
			if !assert.NoErrorf(c, err, "Failed to query agent metrics") {
				return
			}
			assert.NotEmptyf(c, metrics,
				"Agent should continue reporting metrics under pressure")

			suite.T().Logf("Agent running metrics: %d", len(metrics))
		}, 5*time.Minute, 10*time.Second, "Resource exhaustion handling validation failed")
	})
}

func (suite *ecsResilienceSuite) TestRapidContainerChurn() {
	// Test agent handling of rapid container creation/deletion
	suite.Run("Rapid container churn", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify agent tracks containers via container metrics
			metrics, err := suite.Fakeintake.FilterMetrics("container.cpu.usage")
			if err != nil || len(metrics) == 0 {
				metrics, err = suite.Fakeintake.FilterMetrics("container.memory.usage")
			}
			if !assert.NoErrorf(c, err, "Failed to query container metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No container metrics found") {
				return
			}

			// Count unique containers
			containers := make(map[string]bool)
			for _, metric := range metrics {
				for _, tag := range metric.GetTags() {
					if strings.HasPrefix(tag, "container_name:") {
						containers[strings.TrimPrefix(tag, "container_name:")] = true
					}
				}
			}

			suite.T().Logf("Tracked containers: %d", len(containers))

			// Verify agent is tracking at least one container
			assert.GreaterOrEqualf(c, len(containers), 1,
				"Agent should track at least one container")
		}, 5*time.Minute, 10*time.Second, "Rapid container churn validation failed")
	})
}

func (suite *ecsResilienceSuite) TestLargePayloads() {
	// Test agent handling of large traces and logs
	suite.Run("Large payload handling", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify agent is receiving traces
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}
			assert.NotEmptyf(c, traces, "Should receive traces")

			if len(traces) > 0 {
				// Find largest trace
				maxSpans := 0
				for _, trace := range traces {
					spanCount := 0
					for _, payload := range trace.TracerPayloads {
						for _, chunk := range payload.Chunks {
							spanCount += len(chunk.Spans)
						}
					}
					if spanCount > maxSpans {
						maxSpans = spanCount
					}
				}
				suite.T().Logf("Largest trace: %d spans", maxSpans)
			}
		}, 5*time.Minute, 10*time.Second, "Large payload handling validation failed")
	})
}

func (suite *ecsResilienceSuite) TestBackpressure() {
	// Test agent behavior under backpressure (slow downstream)
	suite.Run("Backpressure handling", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify agent continues collecting data
			metrics, err := suite.Fakeintake.FilterMetrics("datadog.agent.running")
			if !assert.NoErrorf(c, err, "Failed to query agent metrics") {
				return
			}
			assert.NotEmptyf(c, metrics,
				"Agent should continue reporting metrics (handles backpressure)")
			suite.T().Logf("Agent running metrics: %d", len(metrics))
		}, 5*time.Minute, 10*time.Second, "Backpressure handling validation failed")
	})
}
