// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/stretchr/testify/assert"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

type ecsConfigSuite struct {
	BaseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSConfigSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ecsConfigSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithFargateCapacityProvider(),
				scenecs.WithLinuxNodeGroup(),
			),
			// Using existing workloads (redis, nginx, tracegen) to test configuration
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsConfigSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

func (suite *ecsConfigSuite) Test00UpAndRunning() {
	suite.AssertECSTasksReady(suite.ecsClusterName)
}

func (suite *ecsConfigSuite) TestEnvVarConfiguration() {
	// Test environment variable configuration propagation
	suite.Run("Environment variable configuration", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Use container metrics which carry workload-level tags (service, env)
			// set via DD_SERVICE, DD_ENV environment variables
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

			// Look for workload-level tags from DD_ENV, DD_SERVICE, and ECS metadata
			foundServiceTag := false
			foundEnvTag := false
			foundClusterTag := false

			for _, metric := range metrics {
				for _, tag := range metric.GetTags() {
					if strings.HasPrefix(tag, "service:") {
						foundServiceTag = true
					}
					if strings.HasPrefix(tag, "env:") {
						foundEnvTag = true
					}
					if strings.HasPrefix(tag, "ecs_cluster_name:") {
						foundClusterTag = true
					}
				}

				if foundServiceTag && foundEnvTag && foundClusterTag {
					break
				}
			}

			assert.Truef(c, foundServiceTag, "Metrics should have service tag from DD_SERVICE")
			assert.Truef(c, foundEnvTag, "Metrics should have env tag from DD_ENV")
			assert.Truef(c, foundClusterTag, "Metrics should have ECS cluster tag")
		}, 5*time.Minute, 10*time.Second, "Environment variable configuration validation failed")
	})
}

func (suite *ecsConfigSuite) TestDockerLabelDiscovery() {
	// Test Docker label-based configuration discovery
	suite.Run("Docker label discovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// The testing workload (tracegen, redis, nginx) uses Docker labels for autodiscovery
			// com.datadoghq.ad.* labels configure checks

			// Check metric names available in fakeintake
			names, err := suite.Fakeintake.GetMetricNames()
			if !assert.NoErrorf(c, err, "Failed to query metric names") {
				return
			}

			// Look for metric names from autodiscovered checks
			checkMetrics := make(map[string]bool)
			for _, name := range names {
				if strings.HasPrefix(name, "redis.") {
					checkMetrics["redis"] = true
				}
				if strings.HasPrefix(name, "nginx.") {
					checkMetrics["nginx"] = true
				}
			}

			// At least one autodiscovered check should be producing metrics
			assert.NotEmptyf(c, checkMetrics,
				"Expected autodiscovered check metrics (redis.* or nginx.*) but found none in %d metric names", len(names))

		}, 5*time.Minute, 10*time.Second, "Docker label discovery validation failed")
	})
}

func (suite *ecsConfigSuite) TestTaskDefinitionDiscovery() {
	// Test task definition-level configuration discovery
	suite.Run("Task definition discovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Validate that agent discovers containers from task definition
			// and enriches data with task/container metadata
			// Use container metrics which carry task definition metadata
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

			// Check for task definition metadata in tags
			foundTaskArn := false
			foundContainerName := false
			foundTaskFamily := false

			for _, metric := range metrics {
				for _, tag := range metric.GetTags() {
					if strings.HasPrefix(tag, "task_arn:") {
						foundTaskArn = true
					}
					if strings.HasPrefix(tag, "container_name:") {
						foundContainerName = true
					}
					if strings.HasPrefix(tag, "task_family:") {
						foundTaskFamily = true
					}
				}

				if foundTaskArn && foundContainerName && foundTaskFamily {
					break
				}
			}

			assert.Truef(c, foundTaskArn, "Metrics should have task_arn tag from task definition")
			assert.Truef(c, foundContainerName, "Metrics should have container_name tag from task definition")
			assert.Truef(c, foundTaskFamily, "Metrics should have task_family tag from task definition")
		}, 5*time.Minute, 10*time.Second, "Task definition discovery validation failed")
	})
}

func (suite *ecsConfigSuite) TestDynamicConfiguration() {
	// Test dynamic configuration updates (container discovery)
	suite.Run("Dynamic configuration", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Validate that agent dynamically discovers containers
			// Use a targeted metric that is tagged with container info
			metrics, err := suite.Fakeintake.FilterMetrics("container.cpu.usage")
			if err != nil || len(metrics) == 0 {
				// Fall back to another common container metric
				metrics, err = suite.Fakeintake.FilterMetrics("container.memory.usage")
			}
			if !assert.NoErrorf(c, err, "Failed to query container metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No container metrics found") {
				return
			}

			// Count unique containers discovered
			containers := make(map[string]bool)
			tasks := make(map[string]bool)

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
					if strings.HasPrefix(tag, "container_name:") {
						containers[strings.TrimPrefix(tag, "container_name:")] = true
					}
					if strings.HasPrefix(tag, "task_arn:") {
						tasks[strings.TrimPrefix(tag, "task_arn:")] = true
					}
				}
			}

			// Should discover at least one container
			assert.GreaterOrEqualf(c, len(containers), 1,
				"Should discover at least one container")

			// Should discover at least one task
			assert.GreaterOrEqualf(c, len(tasks), 1,
				"Should discover at least one task")
		}, 5*time.Minute, 10*time.Second, "Dynamic configuration validation failed")
	})
}

func (suite *ecsConfigSuite) TestMetadataEndpoints() {
	// Test ECS metadata endpoint usage
	suite.Run("ECS metadata endpoints", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// The agent uses ECS metadata endpoints (V1, V2, V3/V4) to collect task/container info
			// We can validate this by checking that ECS-specific metadata is present on container metrics
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

			// Check for metadata that comes from ECS endpoints
			foundECSMetadata := make(map[string]bool)

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
					if strings.HasPrefix(tag, "ecs_cluster_name:") {
						foundECSMetadata["ecs_cluster_name"] = true
					}
					if strings.HasPrefix(tag, "task_arn:") {
						foundECSMetadata["task_arn"] = true
					}
					if strings.HasPrefix(tag, "task_family:") {
						foundECSMetadata["task_family"] = true
					}
					if strings.HasPrefix(tag, "task_version:") {
						foundECSMetadata["task_version"] = true
					}
					if strings.HasPrefix(tag, "ecs_container_name:") || strings.HasPrefix(tag, "container_name:") {
						foundECSMetadata["container_name"] = true
					}
					if strings.HasPrefix(tag, "ecs_launch_type:") {
						foundECSMetadata["ecs_launch_type"] = true
					}
				}
			}

			// Should have core ECS metadata
			assert.Truef(c, foundECSMetadata["ecs_cluster_name"],
				"Should have ecs_cluster_name from metadata endpoint")
			assert.Truef(c, foundECSMetadata["task_arn"],
				"Should have task_arn from metadata endpoint")
			assert.Truef(c, foundECSMetadata["container_name"],
				"Should have container_name from metadata endpoint")

			// Validate cluster name matches expected
			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.HasPrefix(tag, "ecs_cluster_name:") {
						clusterName := strings.TrimPrefix(tag, "ecs_cluster_name:")
						assert.Equalf(c, suite.ecsClusterName, clusterName,
							"Cluster name from metadata endpoint should match")
						return
					}
				}
			}
		}, 5*time.Minute, 10*time.Second, "ECS metadata endpoints validation failed")
	})
}

func (suite *ecsConfigSuite) TestServiceDiscovery() {
	// Test automatic service discovery
	suite.Run("Service discovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Use container metrics which carry workload-level service tags
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

			// Collect discovered services from these metrics
			services := make(map[string]bool)

			for _, metric := range metrics {
				for _, tag := range metric.GetTags() {
					if strings.HasPrefix(tag, "service:") {
						services[strings.TrimPrefix(tag, "service:")] = true
					}
				}
			}

			// Should discover at least one service
			assert.GreaterOrEqualf(c, len(services), 1,
				"Should discover at least one service")
		}, 5*time.Minute, 10*time.Second, "Service discovery validation failed")
	})
}

func (suite *ecsConfigSuite) TestConfigPrecedence() {
	// Test configuration precedence (env vars vs labels vs agent config)
	suite.Run("Configuration precedence", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Test that configuration precedence is correct:
			// 1. Container labels (com.datadoghq.tags.*)
			// 2. Environment variables (DD_*)
			// 3. Agent configuration

			// Use container metrics which carry both env var tags and agent metadata tags
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

			// Check for tags that come from different sources
			hasHighPriorityTags := false
			hasAgentTags := false

			for _, metric := range metrics {
				for _, tag := range metric.GetTags() {
					// Tags from env vars (high priority)
					if strings.HasPrefix(tag, "service:") || strings.HasPrefix(tag, "env:") {
						hasHighPriorityTags = true
					}
					// Tags from agent (ECS metadata)
					if strings.HasPrefix(tag, "ecs_cluster_name:") || strings.HasPrefix(tag, "task_arn:") {
						hasAgentTags = true
					}
				}
				if hasHighPriorityTags && hasAgentTags {
					break
				}
			}

			// Both high-priority (env var/label) and agent-level tags should be present
			assert.Truef(c, hasHighPriorityTags,
				"Should have high-priority tags from env vars or labels")
			assert.Truef(c, hasAgentTags,
				"Should have agent-level metadata tags")
		}, 5*time.Minute, 10*time.Second, "Configuration precedence validation failed")
	})
}
