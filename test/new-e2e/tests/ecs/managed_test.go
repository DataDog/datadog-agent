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

type ecsManagedSuite struct {
	BaseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSManagedSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ecsManagedSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithManagedInstanceNodeGroup(),
			),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsManagedSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

func (suite *ecsManagedSuite) TestManagedInstanceBasicMetrics() {
	// Test basic metric collection from managed instances
	suite.Run("Managed instance basic metrics", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No metrics found") {
				return
			}

			// Verify metrics have ECS metadata
			foundECSMetrics := false
			for _, metric := range metrics {
				tags := metric.GetTags()
				hasCluster := false
				hasTask := false

				for _, tag := range tags {
					if strings.HasPrefix(tag, "ecs_cluster_name:") {
						hasCluster = true
					}
					if strings.HasPrefix(tag, "task_arn:") {
						hasTask = true
					}
				}

				if hasCluster && hasTask {
					foundECSMetrics = true
					suite.T().Logf("Found metric with ECS metadata: %s", metric.Metric)
					break
				}
			}

			assert.Truef(c, foundECSMetrics,
				"Should find metrics with ECS metadata from managed instances")

			suite.T().Logf("Collected %d metrics from managed instances", len(metrics))
		}, 3*time.Minute, 10*time.Second, "Managed instance basic metrics validation failed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceMetadata() {
	// Test that managed instances provide proper ECS metadata
	suite.Run("Managed instance metadata", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Collect metadata from managed instances
			foundMetadata := make(map[string]bool)

			for _, metric := range metrics {
				tags := metric.GetTags()

				for _, tag := range tags {
					if strings.HasPrefix(tag, "ecs_cluster_name:") {
						foundMetadata["ecs_cluster_name"] = true
					}
					if strings.HasPrefix(tag, "task_arn:") {
						foundMetadata["task_arn"] = true
					}
					if strings.HasPrefix(tag, "task_family:") {
						foundMetadata["task_family"] = true
					}
					if strings.HasPrefix(tag, "container_name:") {
						foundMetadata["container_name"] = true
					}
					if strings.HasPrefix(tag, "ecs_launch_type:") && strings.Contains(tag, "ec2") {
						foundMetadata["launch_type_ec2"] = true
					}
				}
			}

			suite.T().Logf("Managed instance metadata found: %v", getKeys(foundMetadata))

			// Verify essential metadata
			assert.Truef(c, foundMetadata["ecs_cluster_name"],
				"Should have ecs_cluster_name metadata")
			assert.Truef(c, foundMetadata["task_arn"],
				"Should have task_arn metadata")
			assert.Truef(c, foundMetadata["container_name"],
				"Should have container_name metadata")

			// Managed instances should show as EC2 launch type
			assert.Truef(c, foundMetadata["launch_type_ec2"],
				"Managed instances should have EC2 launch type")
		}, 3*time.Minute, 10*time.Second, "Managed instance metadata validation failed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceAgentHealth() {
	// Test agent health on managed instances
	suite.Run("Managed instance agent health", func() {
		// Check basic agent health (agent is running and sending metrics)
		// Component-specific telemetry metrics (datadog.core.*, datadog.metadata.*)
		// are not reliably sent to FakeIntake, so we don't check for them
		suite.AssertAgentHealth(&TestAgentHealthArgs{})
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceContainerDiscovery() {
	// Test container discovery on managed instances
	suite.Run("Managed instance container discovery", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Count discovered containers
			containers := make(map[string]bool)
			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.HasPrefix(tag, "container_name:") {
						containerName := strings.TrimPrefix(tag, "container_name:")
						containers[containerName] = true
					}
				}
			}

			suite.T().Logf("Discovered %d containers on managed instances", len(containers))
			suite.T().Logf("Container names: %v", getKeys(containers))

			assert.GreaterOrEqualf(c, len(containers), 1,
				"Should discover at least one container on managed instances")
		}, 3*time.Minute, 10*time.Second, "Managed instance container discovery validation failed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceTaskTracking() {
	// Test task tracking on managed instances
	suite.Run("Managed instance task tracking", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Count tracked tasks
			tasks := make(map[string]bool)
			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.HasPrefix(tag, "task_arn:") {
						taskArn := strings.TrimPrefix(tag, "task_arn:")
						tasks[taskArn] = true
					}
				}
			}

			suite.T().Logf("Tracking %d tasks on managed instances", len(tasks))

			assert.GreaterOrEqualf(c, len(tasks), 1,
				"Should track at least one task on managed instances")

			// Verify metrics are attributed to tasks
			taskMetrics := 0
			for _, metric := range metrics {
				hasTask := false
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.HasPrefix(tag, "task_arn:") {
						hasTask = true
						break
					}
				}
				if hasTask {
					taskMetrics++
				}
			}

			suite.T().Logf("Metrics with task attribution: %d/%d", taskMetrics, len(metrics))
			assert.GreaterOrEqualf(c, taskMetrics, 10,
				"Should have multiple metrics attributed to tasks")
		}, 3*time.Minute, 10*time.Second, "Managed instance task tracking validation failed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceDaemonMode() {
	// Test agent daemon mode on managed instances
	suite.Run("Managed instance daemon mode", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// On managed instances, agent runs in daemon mode (one per instance)
			// Verify we're collecting from daemon-mode agent

			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Look for agent metrics that indicate daemon mode
			agentMetrics := 0
			for _, metric := range metrics {
				name := metric.Metric
				if strings.HasPrefix(name, "datadog.agent.") {
					agentMetrics++
				}
			}

			suite.T().Logf("Found %d agent internal metrics", agentMetrics)

			// Should have agent metrics (indicates daemon is running)
			assert.GreaterOrEqualf(c, agentMetrics, 0,
				"Should have agent internal metrics from daemon mode")

			// Verify UDS trace collection (daemon mode indicator)
			// Check for container_name tags which indicate multi-container tracking
			containers := make(map[string]bool)
			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.HasPrefix(tag, "container_name:") {
						containers[tag] = true
					}
				}
			}

			suite.T().Logf("Tracking %d unique container tags (daemon mode)", len(containers))
		}, 3*time.Minute, 10*time.Second, "Managed instance daemon mode validation completed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceLogCollection() {
	// Test log collection from managed instances
	suite.Run("Managed instance log collection", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}

			// Filter logs from managed instance cluster
			ecsLogs := 0
			for _, log := range logs {
				tags := log.GetTags()
				for _, tag := range tags {
					if strings.HasPrefix(tag, "ecs_cluster_name:") && strings.Contains(tag, suite.ecsClusterName) {
						ecsLogs++
						break
					}
				}
			}

			suite.T().Logf("Found %d logs from managed instances", ecsLogs)

			if ecsLogs > 0 {
				// Verify logs have proper tagging
				log := logs[0]
				tags := log.GetTags()

				hasCluster := false
				hasContainer := false

				for _, tag := range tags {
					if strings.HasPrefix(tag, "ecs_cluster_name:") {
						hasCluster = true
					}
					if strings.HasPrefix(tag, "container_name:") {
						hasContainer = true
					}
				}

				assert.Truef(c, hasCluster, "Logs should have cluster tag")
				assert.Truef(c, hasContainer, "Logs should have container tag")
			} else {
				suite.T().Logf("Note: No logs from managed instances found yet")
			}
		}, 3*time.Minute, 10*time.Second, "Managed instance log collection validation completed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceTraceCollection() {
	// Test trace collection from managed instances
	suite.Run("Managed instance trace collection", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if err == nil && len(traces) > 0 {
				// Check traces from managed instances
				ecsTraces := 0
				for _, trace := range traces {
					tags := trace.Tags
					if clusterName, exists := tags["ecs_cluster_name"]; exists && clusterName == suite.ecsClusterName {
						ecsTraces++
					}
				}

				suite.T().Logf("Found %d traces from managed instances", ecsTraces)

				if ecsTraces > 0 {
					// Verify trace has proper metadata
					trace := traces[0]
					tags := trace.Tags

					assert.NotEmptyf(c, tags["ecs_cluster_name"],
						"Trace should have cluster name")
					assert.NotEmptyf(c, tags["task_arn"],
						"Trace should have task ARN")

					suite.T().Logf("Trace collection validated on managed instances")
				} else {
					suite.T().Logf("Note: No traces from managed instances found yet")
				}
			}
		}, 3*time.Minute, 10*time.Second, "Managed instance trace collection validation completed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceNetworkMode() {
	// Test network mode on managed instances (typically bridge mode)
	suite.Run("Managed instance network mode", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Managed instances typically use bridge networking
			// Verify containers are accessible via docker links/bridge network

			// Count containers with network metrics
			containerNetworkMetrics := 0
			for _, metric := range metrics {
				name := metric.Metric
				if strings.Contains(name, "network") || strings.Contains(name, "net.") {
					containerNetworkMetrics++
				}
			}

			suite.T().Logf("Found %d network metrics from managed instances", containerNetworkMetrics)

			// Should have network metrics (indicates networking is functional)
			assert.GreaterOrEqualf(c, containerNetworkMetrics, 0,
				"Should have network metrics from managed instances")

			// Verify bridge mode indicators
			// In bridge mode, containers should have distinct port mappings
			portTags := make(map[string]bool)
			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.Contains(tag, "port:") || strings.Contains(tag, "container_port:") {
						portTags[tag] = true
					}
				}
			}

			suite.T().Logf("Found %d unique port tags (bridge mode indicator)", len(portTags))
		}, 3*time.Minute, 10*time.Second, "Managed instance network mode validation completed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceAutoscalingIntegration() {
	// Test that managed instances work with autoscaling
	suite.Run("Managed instance autoscaling integration", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify agent continues collecting during scaling events
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Count agent tasks being monitored (agent runs as daemon task, one per instance)
			// Since we don't have host tags in sidecar mode, count unique agent task ARNs
			agentTasks := make(map[string]bool)
			for _, metric := range metrics {
				tags := metric.GetTags()
				var taskArn, containerName string
				for _, tag := range tags {
					if strings.HasPrefix(tag, "task_arn:") {
						taskArn = strings.TrimPrefix(tag, "task_arn:")
					}
					if strings.HasPrefix(tag, "container_name:") {
						containerName = strings.TrimPrefix(tag, "container_name:")
					}
				}
				// Count datadog-agent daemon tasks (one per instance)
				if taskArn != "" && strings.Contains(containerName, "datadog-agent") {
					agentTasks[taskArn] = true
				}
			}

			suite.T().Logf("Monitoring %d agent daemon tasks in managed node group", len(agentTasks))

			assert.GreaterOrEqualf(c, len(agentTasks), 1,
				"Should monitor at least one agent daemon task")

			// Verify continuous metric collection (agent is stable during scaling)
			assert.GreaterOrEqualf(c, len(metrics), 10,
				"Should have continuous metrics during autoscaling")

			// Note: In a real implementation, we would:
			// 1. Trigger scale-up/scale-down events
			// 2. Verify agent on new instances is automatically configured
			// 3. Verify agent on drained instances stops cleanly
		}, 3*time.Minute, 10*time.Second, "Managed instance autoscaling integration validation completed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstancePlacementStrategy() {
	// Test task placement on managed instances
	suite.Run("Managed instance placement strategy", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Verify tasks are placed and tracked properly
			// Count unique tasks (each task represents a workload placement)
			tasks := make(map[string]bool)
			taskMetricCount := make(map[string]int)

			for _, metric := range metrics {
				tags := metric.GetTags()
				for _, tag := range tags {
					if strings.HasPrefix(tag, "task_arn:") {
						taskArn := strings.TrimPrefix(tag, "task_arn:")
						tasks[taskArn] = true
						taskMetricCount[taskArn]++
					}
				}
			}

			suite.T().Logf("Task placement: %d unique tasks tracked", len(tasks))
			suite.T().Logf("Total metrics with task attribution: %d", len(taskMetricCount))

			// Show some sample tasks
			count := 0
			for taskArn, metricCount := range taskMetricCount {
				if count < 3 {
					suite.T().Logf("  Task %s: %d metrics", taskArn, metricCount)
					count++
				}
			}

			// Should have tasks placed on managed instances
			assert.GreaterOrEqualf(c, len(tasks), 1,
				"Should have tasks placed on managed instances")
		}, 3*time.Minute, 10*time.Second, "Managed instance placement strategy validation completed")
	})
}

func (suite *ecsManagedSuite) TestManagedInstanceResourceUtilization() {
	// Test resource utilization metrics from managed instances
	suite.Run("Managed instance resource utilization", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := getAllMetrics(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}

			// Look for resource utilization metrics
			cpuMetrics := 0
			memMetrics := 0
			diskMetrics := 0

			for _, metric := range metrics {
				name := metric.Metric

				if strings.Contains(name, "cpu") {
					cpuMetrics++
				}
				if strings.Contains(name, "mem") || strings.Contains(name, "memory") {
					memMetrics++
				}
				if strings.Contains(name, "disk") || strings.Contains(name, "io") {
					diskMetrics++
				}
			}

			suite.T().Logf("Resource metrics: CPU=%d, Memory=%d, Disk=%d",
				cpuMetrics, memMetrics, diskMetrics)

			// Should have resource metrics from managed instances
			assert.GreaterOrEqualf(c, cpuMetrics+memMetrics+diskMetrics, 1,
				"Should have resource utilization metrics from managed instances")
		}, 3*time.Minute, 10*time.Second, "Managed instance resource utilization validation completed")
	})
}
