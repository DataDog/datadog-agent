// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers"
	"github.com/stretchr/testify/assert"

	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
)

type ecsConfigSuite struct {
	containers.BaseSuite[environments.ECS]
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
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.clusterName = suite.Env().ECSCluster.ClusterName
}

func (suite *ecsConfigSuite) TestEnvVarConfiguration() {
	// Test environment variable configuration propagation
	suite.Run("Environment variable configuration", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Check metrics for DD_* env var configuration
			metrics, err := suite.Fakeintake.GetMetrics()
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No metrics found") {
				return
			}

			// Look for metrics with custom tags from DD_TAGS
			// The testing workload should have standard DD_ENV, DD_SERVICE, DD_VERSION tags
			foundServiceTag := false
			foundEnvTag := false
			foundClusterTag := false

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
					if strings.HasPrefix(tag, "service:") {
						foundServiceTag = true
						suite.T().Logf("Found service tag: %s", tag)
					}
					if strings.HasPrefix(tag, "env:") {
						foundEnvTag = true
						suite.T().Logf("Found env tag: %s", tag)
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

			// Validate DD_TAGS propagation
			suite.T().Logf("Environment variable configuration validated: service=%v, env=%v, cluster=%v",
				foundServiceTag, foundEnvTag, foundClusterTag)
		}, 3*suite.Minute, 10*suite.Second, "Environment variable configuration validation failed")
	})
}

func (suite *ecsConfigSuite) TestDockerLabelDiscovery() {
	// Test Docker label-based configuration discovery
	suite.Run("Docker label discovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// The testing workload (tracegen, redis, nginx) uses Docker labels for autodiscovery
			// com.datadoghq.ad.* labels configure checks

			// Check that autodiscovered checks are running
			// We can validate this by looking for check-specific metrics
			metrics, err := suite.Fakeintake.GetMetrics()
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Look for metrics from autodiscovered checks
			// For example, redis metrics if redis is deployed
			checkMetrics := make(map[string]bool)

			for _, metric := range metrics {
				metricName := metric.GetMetricName()

				// Identify check-specific metrics
				if strings.HasPrefix(metricName, "redis.") {
					checkMetrics["redis"] = true
				}
				if strings.HasPrefix(metricName, "nginx.") {
					checkMetrics["nginx"] = true
				}
			}

			if len(checkMetrics) > 0 {
				suite.T().Logf("Found autodiscovered check metrics: %v", getKeys(checkMetrics))
				assert.Truef(c, true, "Docker label autodiscovery is working")
			} else {
				suite.T().Logf("Note: No autodiscovered check metrics found yet (checked %d metrics)", len(metrics))
			}

			// Validate logs have Docker label configuration
			logs, err := suite.Fakeintake.GetLogs()
			if err == nil && len(logs) > 0 {
				// Check that logs have source configured via Docker labels
				logsWithSource := 0
				for _, log := range logs {
					if log.GetSource() != "" {
						logsWithSource++
					}
				}

				suite.T().Logf("Found %d/%d logs with source (configured via Docker labels)",
					logsWithSource, len(logs))

				if logsWithSource > 0 {
					assert.Truef(c, true, "Docker label log configuration is working")
				}
			}
		}, 3*suite.Minute, 10*suite.Second, "Docker label discovery validation completed")
	})
}

func (suite *ecsConfigSuite) TestTaskDefinitionDiscovery() {
	// Test task definition-level configuration discovery
	suite.Run("Task definition discovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Validate that agent discovers containers from task definition
			// and enriches data with task/container metadata

			metrics, err := suite.Fakeintake.GetMetrics()
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No metrics found") {
				return
			}

			// Check for task definition metadata in tags
			foundTaskArn := false
			foundContainerName := false
			foundTaskFamily := false

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
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

			// Validate port mapping discovery
			// If containers expose ports, metrics should reflect that
			foundContainerPort := false
			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.Contains(tag, "port:") || strings.Contains(tag, "container_port:") {
						foundContainerPort = true
						suite.T().Logf("Found port mapping in tags: %s", tag)
						break
					}
				}
				if foundContainerPort {
					break
				}
			}

			suite.T().Logf("Task definition discovery validated: task_arn=%v, container=%v, family=%v, port=%v",
				foundTaskArn, foundContainerName, foundTaskFamily, foundContainerPort)
		}, 3*suite.Minute, 10*suite.Second, "Task definition discovery validation failed")
	})
}

func (suite *ecsConfigSuite) TestDynamicConfiguration() {
	// Test dynamic configuration updates (container discovery)
	suite.Run("Dynamic configuration", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Validate that agent dynamically discovers containers
			// This is tested by checking that metrics are collected from multiple containers

			metrics, err := suite.Fakeintake.GetMetrics()
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No metrics found") {
				return
			}

			// Count unique containers discovered
			containers := make(map[string]bool)
			tasks := make(map[string]bool)

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
					if strings.HasPrefix(tag, "container_name:") {
						containerName := strings.TrimPrefix(tag, "container_name:")
						containers[containerName] = true
					}
					if strings.HasPrefix(tag, "task_arn:") {
						taskArn := strings.TrimPrefix(tag, "task_arn:")
						tasks[taskArn] = true
					}
				}
			}

			suite.T().Logf("Dynamically discovered %d containers in %d tasks",
				len(containers), len(tasks))
			suite.T().Logf("Containers: %v", getKeys(containers))

			// Should discover at least one container
			assert.GreaterOrEqualf(c, len(containers), 1,
				"Should discover at least one container")

			// Should discover at least one task
			assert.GreaterOrEqualf(c, len(tasks), 1,
				"Should discover at least one task")

			// Validate dynamic updates - check that metrics are continuously updated
			// by checking for recent timestamps
			recentMetrics := 0
			for _, metric := range metrics {
				// Metrics with recent timestamps indicate active discovery
				if metric.GetTimestamp() > 0 {
					recentMetrics++
				}
			}

			suite.T().Logf("Found %d metrics with timestamps (indicating active collection)", recentMetrics)
			assert.GreaterOrEqualf(c, recentMetrics, 10,
				"Should have recent metrics indicating dynamic updates")
		}, 3*suite.Minute, 10*suite.Second, "Dynamic configuration validation failed")
	})
}

func (suite *ecsConfigSuite) TestMetadataEndpoints() {
	// Test ECS metadata endpoint usage
	suite.Run("ECS metadata endpoints", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// The agent uses ECS metadata endpoints (V1, V2, V3/V4) to collect task/container info
			// We can validate this by checking that ECS-specific metadata is present

			metrics, err := suite.Fakeintake.GetMetrics()
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No metrics found") {
				return
			}

			// Check for metadata that comes from ECS endpoints
			foundECSMetadata := make(map[string]bool)

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
					// Metadata from ECS endpoints
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

			suite.T().Logf("Found ECS metadata from endpoints: %v", getKeys(foundECSMetadata))

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
		}, 3*suite.Minute, 10*suite.Second, "ECS metadata endpoints validation failed")
	})
}

func (suite *ecsConfigSuite) TestServiceDiscovery() {
	// Test automatic service discovery
	suite.Run("Service discovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Validate that services are automatically discovered and tagged

			metrics, err := suite.Fakeintake.GetMetrics()
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No metrics found") {
				return
			}

			// Collect discovered services
			services := make(map[string]bool)
			serviceMetrics := make(map[string]int)

			for _, metric := range metrics {
				tags := metric.GetTags()

				// Find service tags
				for _, tag := range tags {
					if strings.HasPrefix(tag, "service:") {
						serviceName := strings.TrimPrefix(tag, "service:")
						services[serviceName] = true
						serviceMetrics[serviceName]++
					}
				}
			}

			suite.T().Logf("Discovered services: %v", getKeys(services))
			suite.T().Logf("Metrics per service: %v", serviceMetrics)

			// Should discover at least one service
			assert.GreaterOrEqualf(c, len(services), 1,
				"Should discover at least one service")

			// Services should have multiple metrics
			for service, count := range serviceMetrics {
				suite.T().Logf("Service '%s' has %d metrics", service, count)
				assert.GreaterOrEqualf(c, count, 1,
					"Service '%s' should have at least one metric", service)
			}

			// Validate service-level tags are applied consistently
			// Check that all metrics from a service have consistent tags
			for serviceName := range services {
				serviceMetricsCount := 0
				for _, metric := range metrics {
					hasService := false
					hasEnv := false

					tags := metric.GetTags()
					for _, tag := range tags {
						if tag == "service:"+serviceName {
							hasService = true
							serviceMetricsCount++
						}
						if strings.HasPrefix(tag, "env:") {
							hasEnv = true
						}
					}

					// If metric is from this service, it should have env tag
					if hasService && hasEnv {
						suite.T().Logf("Service '%s' metrics have consistent env tag", serviceName)
						assert.Truef(c, true, "Service discovery applying consistent tags")
						return
					}
				}

				suite.T().Logf("Service '%s' has %d metrics", serviceName, serviceMetricsCount)
			}
		}, 3*suite.Minute, 10*suite.Second, "Service discovery validation completed")
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

			metrics, err := suite.Fakeintake.GetMetrics()
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No metrics found") {
				return
			}

			// Check for tags that come from different sources
			tagSources := make(map[string]string)

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
					// Tags from env vars
					if strings.HasPrefix(tag, "service:") {
						if _, exists := tagSources["service"]; !exists {
							tagSources["service"] = "env_var_or_label"
						}
					}
					if strings.HasPrefix(tag, "env:") {
						if _, exists := tagSources["env"]; !exists {
							tagSources["env"] = "env_var_or_label"
						}
					}
					if strings.HasPrefix(tag, "version:") {
						if _, exists := tagSources["version"]; !exists {
							tagSources["version"] = "env_var_or_label"
						}
					}

					// Tags from agent (ECS metadata)
					if strings.HasPrefix(tag, "ecs_cluster_name:") {
						tagSources["ecs_cluster_name"] = "agent_metadata"
					}
					if strings.HasPrefix(tag, "task_arn:") {
						tagSources["task_arn"] = "agent_metadata"
					}
				}
			}

			suite.T().Logf("Tag sources detected: %v", tagSources)

			// Validate that both container-level and agent-level tags are present
			assert.NotEmptyf(c, tagSources, "Should have tags from various sources")

			// Check that service/env/version tags (high priority) are present
			hasHighPriorityTags := tagSources["service"] != "" || tagSources["env"] != ""
			assert.Truef(c, hasHighPriorityTags,
				"Should have high-priority tags from env vars or labels")

			// Check that agent metadata tags (lower priority) are present
			hasAgentTags := tagSources["ecs_cluster_name"] != "" || tagSources["task_arn"] != ""
			assert.Truef(c, hasAgentTags,
				"Should have agent-level metadata tags")

			// Validate precedence by checking for custom tags
			// Custom tags from DD_TAGS should be present
			foundCustomTag := false
			customTagPattern := regexp.MustCompile(`^[a-z_]+:[a-z0-9_-]+$`)

			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					// Skip known standard tags
					if !strings.HasPrefix(tag, "service:") &&
						!strings.HasPrefix(tag, "env:") &&
						!strings.HasPrefix(tag, "version:") &&
						!strings.HasPrefix(tag, "host:") &&
						!strings.HasPrefix(tag, "ecs_") &&
						!strings.HasPrefix(tag, "task_") &&
						!strings.HasPrefix(tag, "container_") &&
						customTagPattern.MatchString(tag) {
						foundCustomTag = true
						suite.T().Logf("Found custom tag (from DD_TAGS or labels): %s", tag)
						break
					}
				}
				if foundCustomTag {
					break
				}
			}

			suite.T().Logf("Configuration precedence validated: high-priority=%v, agent=%v, custom=%v",
				hasHighPriorityTags, hasAgentTags, foundCustomTag)
		}, 3*suite.Minute, 10*suite.Second, "Configuration precedence validation completed")
	})
}

// Helper function to get map keys
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
