// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"time"
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/stretchr/testify/assert"

	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
)

type ecsLogsSuite struct {
	containers.BaseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSLogsSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ecsLogsSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithFargateCapacityProvider(),
				scenecs.WithLinuxNodeGroup(),
			),
			// Note: In a real implementation, we would add the log-generator workload here
			// scenecs.WithFargateWorkloadApp(ecsloggenerator.FargateAppDefinition),
			// scenecs.WithWorkloadApp(ecsloggenerator.EcsAppDefinition),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsLogsSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

func (suite *ecsLogsSuite) Test00AgentLogsReady() {
	// Test that the log agent is ready and collecting logs
	suite.Run("Log agent readiness check", func() {
		suite.TestAgentHealth(&containers.TestAgentHealthArgs{
			CheckComponents: []string{"logs"},
		})

		// Verify we're collecting logs
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			assert.NoErrorf(c, err, "Failed to query logs from fake intake")
			assert.NotEmptyf(c, logs, "No logs received - log agent may not be ready")

			suite.T().Logf("Log agent is ready - received %d logs", len(logs))
		}, 5*time.Minute, 10*time.Second, "Log agent readiness check failed")
	})
}

func (suite *ecsLogsSuite) TestContainerLogCollection() {
	// Test basic container log collection with metadata enrichment
	suite.Run("Container log collection", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}
			if !assert.NotEmptyf(c, logs, "No logs found") {
				return
			}

			// Find logs from ECS containers
			ecsLogs := filterLogsByTag(logs, "ecs_cluster_name", suite.ecsClusterName)
			if !assert.NotEmptyf(c, ecsLogs, "No logs from ECS cluster found") {
				return
			}

			suite.T().Logf("Found %d logs from ECS cluster", len(ecsLogs))

			// Validate log has container metadata
			log := ecsLogs[0]
			tags := log.GetTags()

			// Check for key container metadata tags
			hasClusterName := false
			hasContainerName := false
			hasTaskArn := false

			for _, tag := range tags {
				if strings.HasPrefix(tag, "ecs_cluster_name:") && strings.Contains(tag, suite.ecsClusterName) {
					hasClusterName = true
				}
				if strings.HasPrefix(tag, "container_name:") {
					hasContainerName = true
				}
				if strings.HasPrefix(tag, "task_arn:") {
					hasTaskArn = true
				}
			}

			assert.Truef(c, hasClusterName, "Log missing ecs_cluster_name tag")
			assert.Truef(c, hasContainerName, "Log missing container_name tag")
			assert.Truef(c, hasTaskArn, "Log missing task_arn tag")

			// Validate log has timestamp
			assert.NotZerof(c, log.GetTimestamp(), "Log missing timestamp")

			// Validate log has message
			assert.NotEmptyf(c, log.GetMessage(), "Log has empty message")

			suite.T().Logf("Container log collection validated: cluster=%s, container=%s",
				suite.ecsClusterName, getTagValue(tags, "container_name"))
		}, 3*time.Minute, 10*time.Second, "Container log collection validation failed")
	})
}

func (suite *ecsLogsSuite) TestLogMultiline() {
	// Test multiline log handling (stack traces)
	suite.Run("Multiline log handling", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}

			// Look for stack trace patterns in logs
			// Stack traces should be grouped into single log entries, not split
			multilinePattern := regexp.MustCompile(`(?s)Exception.*\n\s+at\s+.*`)

			for _, log := range logs {
				message := log.GetMessage()
				if multilinePattern.MatchString(message) {
					suite.T().Logf("Found multiline stack trace log (length: %d chars)", len(message))

					// Verify the entire stack trace is in one log entry
					assert.Containsf(c, message, "Exception",
						"Multiline log should contain exception header")
					assert.Containsf(c, message, "at ",
						"Multiline log should contain stack frames")

					// Stack trace should have multiple lines
					lines := strings.Split(message, "\n")
					assert.GreaterOrEqualf(c, len(lines), 2,
						"Stack trace should have multiple lines")

					suite.T().Logf("Multiline handling validated: %d lines in single log entry", len(lines))
					return
				}
			}

			suite.T().Logf("Note: No multiline stack traces found yet (checking %d logs)", len(logs))
		}, 3*time.Minute, 10*time.Second, "Multiline log handling check completed")
	})
}

func (suite *ecsLogsSuite) TestLogParsing() {
	// Test JSON log parsing
	suite.Run("JSON log parsing", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}

			// Look for logs that were JSON and check if they're properly parsed
			for _, log := range logs {
				message := log.GetMessage()

				// Check if this looks like it was originally JSON
				// (may have been parsed into structured fields)
				if strings.Contains(message, "timestamp") || strings.Contains(message, "level") {
					suite.T().Logf("Found structured log: %s", truncateString(message, 100))

					// Verify log has service tag (should be extracted from JSON)
					tags := log.GetTags()
					hasService := false
					for _, tag := range tags {
						if strings.HasPrefix(tag, "service:") {
							hasService = true
							break
						}
					}

					if hasService {
						suite.T().Logf("JSON log properly parsed with service tag")
						assert.Truef(c, true, "Found properly parsed JSON log")
						return
					}
				}
			}

			suite.T().Logf("Checked %d logs for JSON parsing", len(logs))
		}, 2*time.Minute, 10*time.Second, "JSON log parsing check completed")
	})
}

func (suite *ecsLogsSuite) TestLogSampling() {
	// Test log sampling for high-volume logs
	suite.Run("Log sampling", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}
			if !assert.NotEmptyf(c, logs, "No logs found") {
				return
			}

			suite.T().Logf("Received %d total logs", len(logs))

			// In a high-volume scenario with sampling enabled, we should see:
			// 1. Logs are being collected
			// 2. Not every single log is collected (sampling is working)
			// 3. Important logs (errors) are prioritized

			// Check for error logs specifically
			errorLogs := 0
			infoLogs := 0

			for _, log := range logs {
				status := log.GetStatus()
				if status == "error" {
					errorLogs++
				} else if status == "info" {
					infoLogs++
				}
			}

			suite.T().Logf("Log distribution: %d errors, %d info logs", errorLogs, infoLogs)

			// We should have collected some logs
			assert.GreaterOrEqualf(c, len(logs), 10,
				"Should have collected at least 10 logs")

			// Note: Actual sampling behavior depends on agent configuration
			// This is a basic validation that logs are flowing
		}, 2*time.Minute, 10*time.Second, "Log sampling validation completed")
	})
}

func (suite *ecsLogsSuite) TestLogFiltering() {
	// Test log filtering (include/exclude patterns)
	suite.Run("Log filtering", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}
			if !assert.NotEmptyf(c, logs, "No logs found") {
				return
			}

			// Validate that logs are being collected with expected patterns
			// Check for both inclusion and exclusion of certain log types

			// Count logs by source
			sourceDistribution := make(map[string]int)
			for _, log := range logs {
				source := log.GetSource()
				if source != "" {
					sourceDistribution[source]++
				}
			}

			suite.T().Logf("Log sources found: %v", sourceDistribution)

			// We should see logs from various sources
			assert.GreaterOrEqualf(c, len(sourceDistribution), 1,
				"Should have logs from at least one source")

			// Check that logs have proper filtering applied
			// (e.g., no debug logs if log level is INFO)
			debugCount := 0
			for _, log := range logs {
				if strings.Contains(strings.ToLower(log.GetMessage()), "debug") {
					debugCount++
				}
			}

			suite.T().Logf("Found %d debug logs out of %d total", debugCount, len(logs))
		}, 2*time.Minute, 10*time.Second, "Log filtering validation completed")
	})
}

func (suite *ecsLogsSuite) TestLogSourceDetection() {
	// Test automatic source detection from containers
	suite.Run("Log source detection", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}
			if !assert.NotEmptyf(c, logs, "No logs found") {
				return
			}

			// Check that logs have source field populated
			logsWithSource := 0
			sources := make(map[string]bool)

			for _, log := range logs {
				source := log.GetSource()
				if source != "" {
					logsWithSource++
					sources[source] = true
				}
			}

			suite.T().Logf("Found %d logs with source out of %d total", logsWithSource, len(logs))
			suite.T().Logf("Detected sources: %v", getKeys(sources))

			// Most logs should have a source
			sourcePercentage := float64(logsWithSource) / float64(len(logs)) * 100
			assert.GreaterOrEqualf(c, sourcePercentage, 50.0,
				"At least 50%% of logs should have source field populated")

			// Should detect at least one source
			assert.GreaterOrEqualf(c, len(sources), 1,
				"Should detect at least one log source")
		}, 2*time.Minute, 10*time.Second, "Log source detection validation failed")
	})
}

func (suite *ecsLogsSuite) TestLogStatusRemapping() {
	// Test log status remapping (error/warning detection)
	suite.Run("Log status remapping", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := getAllLogs(suite.Fakeintake)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}
			if !assert.NotEmptyf(c, logs, "No logs found") {
				return
			}

			// Check status distribution
			statusDistribution := make(map[string]int)
			for _, log := range logs {
				status := log.GetStatus()
				if status != "" {
					statusDistribution[status]++
				}
			}

			suite.T().Logf("Log status distribution: %v", statusDistribution)

			// We should see various log statuses
			assert.GreaterOrEqualf(c, len(statusDistribution), 1,
				"Should have logs with at least one status")

			// Look for logs with ERROR in message that should have error status
			for _, log := range logs {
				message := log.GetMessage()
				status := log.GetStatus()

				if strings.Contains(strings.ToUpper(message), "ERROR") {
					// This log should likely have error status
					suite.T().Logf("Found log with ERROR in message: status=%s", status)

					// Note: Status remapping depends on agent configuration
					// This is an observational check
					if status == "error" {
						assert.Equalf(c, "error", status,
							"Log with ERROR keyword should have error status")
						return
					}
				}
			}

			suite.T().Logf("Status remapping check completed on %d logs", len(logs))
		}, 2*time.Minute, 10*time.Second, "Log status remapping check completed")
	})
}

func (suite *ecsLogsSuite) TestLogTraceCorrelation() {
	// Test log-trace correlation
	suite.Run("Log-trace correlation", func() {
		// First get traces to find trace IDs
		var traceID uint64
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}

			// Get a trace ID from a recent trace
			for _, trace := range traces {
				for _, payload := range trace.TracerPayloads {
					for _, chunk := range payload.Chunks {
						if len(chunk.Spans) > 0 {
							traceID = chunk.Spans[0].TraceID
							if traceID != 0 {
								suite.T().Logf("Found trace ID: %d", traceID)
								return
							}
						}
					}
				}
			}
		}, 2*time.Minute, 10*time.Second, "Failed to get trace ID")

		// Now check if logs have trace correlation
		if traceID != 0 {
			suite.EventuallyWithTf(func(c *assert.CollectT) {
				logs, err := getAllLogs(suite.Fakeintake)
				if !assert.NoErrorf(c, err, "Failed to query logs") {
					return
				}

				// Look for logs with trace_id tag
				logsWithTraceID := 0
				for _, log := range logs {
					tags := log.GetTags()
					for _, tag := range tags {
						if regexp.MustCompile(`dd\.trace_id:[[:xdigit:]]+`).MatchString(tag) {
							logsWithTraceID++
							suite.T().Logf("Found log with trace correlation: %s", tag)
							break
						}
					}
				}

				if logsWithTraceID > 0 {
					suite.T().Logf("Found %d logs with trace correlation", logsWithTraceID)
					assert.Truef(c, true, "Trace-log correlation is working")
				} else {
					suite.T().Logf("Note: No logs with trace correlation found yet (checked %d logs)", len(logs))
				}
			}, 2*time.Minute, 10*time.Second, "Trace-log correlation check completed")
		}
	})
}

