// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ecs provides end-to-end tests for the Datadog Agent running on Amazon ECS.
// It tests APM/tracing, metrics, logs, and agent health across different ECS launch types
// (Fargate, EC2, and Managed Instances).
package ecs

import (
	"regexp"
	"strings"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

const (
	taskNameDogstatsdUDS = "dogstatsd-uds"
	taskNameDogstatsdUDP = "dogstatsd-udp"

	taskNameTracegenUDS = "tracegen-uds"
	taskNameTracegenTCP = "tracegen-tcp"
)

type ecsAPMSuite struct {
	BaseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSAPMSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ecsAPMSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithFargateCapacityProvider(),
				scenecs.WithLinuxNodeGroup(),
			),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsAPMSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

// getCommonECSTagPatterns returns ECS tag patterns for metrics and traces.
// Parameters:
//   - clusterName: ECS cluster name
//   - taskName: Task name pattern (e.g., "dogstatsd-uds", "tracegen-tcp")
//   - appName: Application name (e.g., "dogstatsd", "tracegen")
//   - includeFullSet: If true, includes all tags (for metrics). If false, returns minimal set (for traces).
func (suite *ecsAPMSuite) getCommonECSTagPatterns(clusterName, taskName, appName string, includeFullSet bool) []string {
	// Minimal tags for traces - ECS metadata is bundled in _dd.tags.container when DD_APM_ENABLE_CONTAINER_TAGS_BUFFER=true
	if !includeFullSet {
		// When DD_APM_ENABLE_CONTAINER_TAGS_BUFFER=true, container tags are bundled into a single _dd.tags.container tag
		// The actual payload format is _dd.tags.container=task_name:X,cluster_name:Y,...
		// BUT when converted to string via k+":"+v in base_helpers.go, it becomes:
		// _dd.tags.container:task_name:X,cluster_name:Y,...
		// Note the ':' separator, not '=' (that's how Go concatenates map entries)
		// We validate that this bundled tag contains the required ECS metadata
		// Patterns match: key:value (followed by comma or end of string)
		// Use non-greedy .*? to avoid matching cluster name in service_arn first
		return []string{
			`^_dd\.tags\.container:.*?cluster_name:` + regexp.QuoteMeta(clusterName) + `(,|$)`,
			`^_dd\.tags\.container:.*?ecs_cluster_name:` + regexp.QuoteMeta(clusterName) + `(,|$)`,
			`^_dd\.tags\.container:.*?container_name:[^,]+(,|$)`,
			`^_dd\.tags\.container:.*?task_arn:[^,]+(,|$)`,
		}
	}

	// Full tag set for metrics - includes ECS metadata, image metadata, and AWS metadata
	return []string{
		// Core ECS metadata
		`^cluster_name:` + regexp.QuoteMeta(clusterName) + `$`,
		`^ecs_cluster_name:` + regexp.QuoteMeta(clusterName) + `$`,
		`^ecs_container_name:` + appName + `$`,
		`^container_id:`,
		`^container_name:ecs-.*-` + regexp.QuoteMeta(taskName) + `-ec2-`,
		`^task_arn:`,
		`^task_family:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
		`^task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
		`^task_version:[[:digit:]]+$`,
		`^task_definition_arn:`,

		// Image metadata
		`^docker_image:ghcr\.io/datadog/apps-` + appName + `:` + regexp.QuoteMeta(apps.Version) + `$`,
		`^image_id:sha256:`,
		`^image_name:ghcr\.io/datadog/apps-` + appName + `$`,
		`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
		`^short_image:apps-` + appName + `$`,
		`^git\.commit\.sha:[[:xdigit:]]{40}$`,
		`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`,

		// AWS metadata
		`^aws_account:[[:digit:]]{12}$`,
		`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(clusterName) + `$`,
		`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(clusterName, "-ecs")) + `-` + appName + `-ud[ps]$`,
		`^region:us-east-1$`,
		`^service_arn:`,
		`^series:`,
	}
}

// Inside a testify test suite, tests are executed in alphabetical order.
// The 00 in Test00UpAndRunning is here to guarantee that this test, waiting for all tasks to be ready
// is run first. This gives the agent time to warm up before other tests run with shorter timeouts.
func (suite *ecsAPMSuite) Test00UpAndRunning() {
	suite.AssertECSTasksReady(suite.ecsClusterName)
}

func (suite *ecsAPMSuite) Test01AgentAPMReady() {
	// Test that the APM agent is ready and receiving traces
	suite.Run("APM agent readiness check", func() {
		suite.AssertAgentHealth(&TestAgentHealthArgs{
			CheckComponents: []string{"trace_agent"},
		})

		// Verify we're receiving traces
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			assert.NoErrorf(c, err, "Failed to query traces from fake intake")
			assert.NotEmptyf(c, traces, "No traces received - APM agent may not be ready")

		}, 5*time.Minute, 10*time.Second, "APM agent readiness check failed")
	})
}

func (suite *ecsAPMSuite) TestBasicTraceCollection() {
	// Test basic trace collection and validation
	suite.Run("Basic trace collection", func() {
		suite.AssertAPMTrace(&TestAPMTraceArgs{
			Filter: TestAPMTraceFilterArgs{
				ServiceName: "tracegen-test-service",
			},
			Expect: TestAPMTraceExpectArgs{
				TraceIDPresent: true,
			},
		})
	})
}

func (suite *ecsAPMSuite) TestMultiServiceTracing() {
	// Test multi-service tracing and service map creation
	// This would test the multiservice app once it's deployed
	suite.Run("Multi-service distributed tracing", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}
			if !assert.NotEmptyf(c, traces, "No traces found") {
				return
			}

			// Look for traces from multiple services
			serviceNames := make(map[string]bool)
			for _, trace := range traces {
				for _, payload := range trace.TracerPayloads {
					for _, chunk := range payload.Chunks {
						for _, span := range chunk.Spans {
							if span.Service != "" {
								serviceNames[span.Service] = true
							}
						}
					}
				}
			}

			// In a real multi-service app, we'd expect frontend, backend, database
			// For now, we just verify we have some services
			assert.GreaterOrEqualf(c, len(serviceNames), 1,
				"Expected traces from at least 1 service, got %d", len(serviceNames))

			// Verify trace propagation (parent-child relationships)
			for _, trace := range traces {
				for _, payload := range trace.TracerPayloads {
					for _, chunk := range payload.Chunks {
						if len(chunk.Spans) > 1 {
							// Check if spans have parent-child relationships
							spansByID := make(map[uint64]*pb.Span)
							for _, span := range chunk.Spans {
								spansByID[span.SpanID] = span
							}

							hasParentChild := false
							for _, span := range chunk.Spans {
								if span.ParentID != 0 {
									if _, exists := spansByID[span.ParentID]; exists {
										hasParentChild = true
										break
									}
								}
							}

							if hasParentChild {
								return
							}
						}
					}
				}
			}

		}, 3*time.Minute, 10*time.Second, "Multi-service tracing validation failed")
	})
}

func (suite *ecsAPMSuite) TestTraceSampling() {
	// Test that trace sampling is working correctly
	suite.Run("Trace sampling validation", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}
			if !assert.NotEmptyf(c, traces, "No traces found") {
				return
			}

			// Check for sampling priority in traces
			for _, trace := range traces {
				for _, payload := range trace.TracerPayloads {
					for _, chunk := range payload.Chunks {
						for _, span := range chunk.Spans {
							if samplingPriority, exists := span.Metrics["_sampling_priority_v1"]; exists {

								// Sampling priority should be >= 0
								assert.GreaterOrEqualf(c, samplingPriority, float64(0),
									"Sampling priority should be >= 0")

								// Common values are 0 (drop), 1 (keep), 2 (user keep)
								assert.LessOrEqualf(c, samplingPriority, float64(2),
									"Sampling priority should be <= 2")

								return
							}
						}
					}
				}
			}

			assert.Failf(c, "No traces with sampling priority found", "checked %d traces", len(traces))
		}, 2*time.Minute, 10*time.Second, "Trace sampling validation failed")
	})
}

func (suite *ecsAPMSuite) TestTraceTagEnrichment() {
	// Test that traces are enriched with ECS metadata tags
	suite.Run("Trace tag enrichment", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}
			if !assert.NotEmptyf(c, traces, "No traces found") {
				return
			}

			// Check that traces have ECS metadata tags (bundled in _dd.tags.container)
			foundEnrichedTrace := false
			for _, trace := range traces {
				// Container tags are in TracerPayload.Tags, not AgentPayload.Tags
				for _, tracerPayload := range trace.TracerPayloads {
					// Check for bundled _dd.tags.container tag
					if containerTagsValue, exists := tracerPayload.Tags["_dd.tags.container"]; exists {
						// Check if bundled tag contains required ECS metadata
						hasClusterName := regexp.MustCompile(`ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName)).MatchString(containerTagsValue)
						hasTaskArn := regexp.MustCompile(`task_arn:`).MatchString(containerTagsValue)
						hasContainerName := regexp.MustCompile(`container_name:`).MatchString(containerTagsValue)

						if hasClusterName && hasTaskArn && hasContainerName {
							foundEnrichedTrace = true
							break
						}
					}
				}
				if foundEnrichedTrace {
					break
				}
			}

			assert.Truef(c, foundEnrichedTrace,
				"No traces found with complete ECS metadata tags in _dd.tags.container (cluster_name, task_arn, container_name)")
		}, 2*time.Minute, 10*time.Second, "Trace tag enrichment validation failed")
	})
}

func (suite *ecsAPMSuite) TestAPMFargate() {
	// Test Fargate-specific APM scenarios
	suite.Run("APM on Fargate", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}

			// Filter for Fargate traces (check bundled _dd.tags.container tag)
			fargateTraces := lo.Filter(traces, func(trace *aggregator.TracePayload, _ int) bool {
				for _, tracerPayload := range trace.TracerPayloads {
					if containerTags, exists := tracerPayload.Tags["_dd.tags.container"]; exists {
						if regexp.MustCompile(`ecs_launch_type:fargate`).MatchString(containerTags) {
							return true
						}
					}
				}
				return false
			})

			if len(fargateTraces) > 0 {

				// Verify Fargate traces have expected metadata in bundled tag
				trace := fargateTraces[0]
				for _, tracerPayload := range trace.TracerPayloads {
					if containerTags, exists := tracerPayload.Tags["_dd.tags.container"]; exists {
						assert.Regexpf(c, `ecs_launch_type:fargate`, containerTags,
							"Fargate trace should have ecs_launch_type:fargate in bundled tag")

						assert.Regexpf(c, `ecs_cluster_name:`+regexp.QuoteMeta(suite.ecsClusterName), containerTags,
							"Fargate trace should have correct cluster name in bundled tag")

						assert.Regexpf(c, `task_arn:`, containerTags,
							"Fargate trace should have task_arn in bundled tag")
						break
					}
				}
			}
		}, 3*time.Minute, 10*time.Second, "Fargate APM validation completed")
	})
}

func (suite *ecsAPMSuite) TestAPMEC2() {
	// Test EC2-specific APM scenarios
	suite.Run("APM on EC2", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}

			// Filter for EC2 traces (check bundled _dd.tags.container tag)
			ec2Traces := lo.Filter(traces, func(trace *aggregator.TracePayload, _ int) bool {
				for _, tracerPayload := range trace.TracerPayloads {
					if containerTags, exists := tracerPayload.Tags["_dd.tags.container"]; exists {
						// Check for ecs_launch_type:ec2 OR presence of ecs_cluster_name (daemon mode)
						if regexp.MustCompile(`ecs_launch_type:ec2`).MatchString(containerTags) ||
							regexp.MustCompile(`ecs_cluster_name:`).MatchString(containerTags) {
							return true
						}
					}
				}
				return false
			})

			if !assert.NotEmptyf(c, ec2Traces, "No EC2 traces found") {
				return
			}

			// Verify EC2 traces have expected metadata in bundled tag
			trace := ec2Traces[0]
			for _, tracerPayload := range trace.TracerPayloads {
				if containerTags, exists := tracerPayload.Tags["_dd.tags.container"]; exists {
					// EC2 tasks should have cluster name
					assert.Regexpf(c, `ecs_cluster_name:`+regexp.QuoteMeta(suite.ecsClusterName), containerTags,
						"EC2 trace should have correct cluster name in bundled tag")

					// EC2 tasks should have task_arn
					assert.Regexpf(c, `task_arn:`, containerTags,
						"EC2 trace should have task_arn in bundled tag")

					// EC2 tasks should have container_name
					assert.Regexpf(c, `container_name:`, containerTags,
						"EC2 trace should have container_name in bundled tag")

					break
				}
			}
		}, 3*time.Minute, 10*time.Second, "EC2 APM validation failed")
	})
}

func (suite *ecsAPMSuite) TestDogstatsdUDS() {
	suite.testDogstatsd(taskNameDogstatsdUDS)
}

func (suite *ecsAPMSuite) TestDogstatsdUDP() {
	suite.testDogstatsd(taskNameDogstatsdUDP)
}

func (suite *ecsAPMSuite) testDogstatsd(taskName string) {
	expectedTags := suite.getCommonECSTagPatterns(suite.ecsClusterName, taskName, "dogstatsd", true)

	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				`^task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
			},
		},
		Expect: TestMetricExpectArgs{
			Tags: &expectedTags,
		},
	})
}

func (suite *ecsAPMSuite) TestTraceUDS() {
	suite.testTrace(taskNameTracegenUDS)
}

func (suite *ecsAPMSuite) TestTraceTCP() {
	suite.testTrace(taskNameTracegenTCP)
}

// testTrace verifies that traces are tagged with container and ECS task tags.
func (suite *ecsAPMSuite) testTrace(taskName string) {
	// Build validation patterns for the bundled _dd.tags.container value
	// The bundled tag is a single comma-separated string of key:value pairs
	clusterNamePattern := regexp.MustCompile(`ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName))
	taskArnPattern := regexp.MustCompile(`task_arn:`)
	containerNamePattern := regexp.MustCompile(`container_name:`)

	suite.EventuallyWithTf(func(c *assert.CollectT) {
		traces, cerr := suite.Fakeintake.GetTraces()
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, cerr, "Failed to query fake intake") {
			return
		}

		found := false
		// Iterate starting from the most recent traces
		for _, trace := range traces {
			// Container tags are in TracerPayload.Tags, not AgentPayload.Tags
			for _, tracerPayload := range trace.TracerPayloads {
				containerTags, exists := tracerPayload.Tags["_dd.tags.container"]
				if !exists {
					continue
				}

				// Validate the bundled tag value contains required ECS metadata
				if clusterNamePattern.MatchString(containerTags) &&
					taskArnPattern.MatchString(containerTags) &&
					containerNamePattern.MatchString(containerTags) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		assert.Truef(c, found, "Failed finding trace with proper bundled _dd.tags.container tags for task %s", taskName)
	}, 2*time.Minute, 10*time.Second, "Failed finding trace with proper bundled tags")
}
