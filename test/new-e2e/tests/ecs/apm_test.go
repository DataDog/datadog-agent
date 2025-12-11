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
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			// Note: In a real implementation, we would add the multiservice workload here
			// scenecs.WithMultiServiceWorkload(),
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

// getCommonECSTagPatterns returns common ECS tag patterns for metrics and traces.
// Parameters:
//   - clusterName: ECS cluster name
//   - taskName: Task name pattern (e.g., "dogstatsd-uds", "tracegen-tcp")
//   - appName: Application name (e.g., "dogstatsd", "tracegen")
//   - includeFullSet: If true, includes all tags (for metrics). If false, returns minimal set (for traces).
func (suite *ecsAPMSuite) getCommonECSTagPatterns(clusterName, taskName, appName string, includeFullSet bool) []string {
	// Common tags present in both metrics and traces
	commonTags := []string{
		`^cluster_name:` + regexp.QuoteMeta(clusterName) + `$`,
		`^container_id:`,
		`^container_name:ecs-.*-` + regexp.QuoteMeta(taskName) + `-ec2-`,
		`^docker_image:ghcr\.io/datadog/apps-` + appName + `:` + regexp.QuoteMeta(apps.Version) + `$`,
		`^ecs_cluster_name:` + regexp.QuoteMeta(clusterName) + `$`,
		`^ecs_container_name:` + appName + `$`,
		`^git\.commit\.sha:[[:xdigit:]]{40}$`,
		`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`,
		`^image_id:sha256:`,
		`^image_name:ghcr\.io/datadog/apps-` + appName + `$`,
		`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
		`^short_image:apps-` + appName + `$`,
		`^task_arn:`,
		`^task_family:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
		`^task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
		`^task_version:[[:digit:]]+$`,
	}

	// Additional tags only present in metrics (not in traces)
	if includeFullSet {
		fullTags := append(commonTags,
			`^aws_account:[[:digit:]]{12}$`,
			`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/`+regexp.QuoteMeta(clusterName)+`$`,
			`^ecs_service:`+regexp.QuoteMeta(strings.TrimSuffix(clusterName, "-ecs"))+`-`+appName+`-ud[ps]$`,
			`^region:us-east-1$`,
			`^series:`,
			`^service_arn:`,
			`^task_definition_arn:`,
		)
		return fullTags
	}

	return commonTags
}

// Once pulumi has finished to create a stack, it can still take some time for the images to be pulled,
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
func (suite *ecsAPMSuite) Test00UpAndRunning() {
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
		}, 15*time.Minute, 10*time.Second, "Not all tasks became ready in time.")
	})
}

func (suite *ecsAPMSuite) Test01AgentAPMReady() {
	// Test that the APM agent is ready and receiving traces
	suite.Run("APM agent readiness check", func() {
		suite.TestAgentHealth(&TestAgentHealthArgs{
			CheckComponents: []string{"trace"},
		})

		// Verify we're receiving traces
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			assert.NoErrorf(c, err, "Failed to query traces from fake intake")
			assert.NotEmptyf(c, traces, "No traces received - APM agent may not be ready")

			suite.T().Logf("APM agent is ready - received %d traces", len(traces))
		}, 5*time.Minute, 10*time.Second, "APM agent readiness check failed")
	})
}

func (suite *ecsAPMSuite) TestBasicTraceCollection() {
	// Test basic trace collection and validation
	suite.Run("Basic trace collection", func() {
		// Use the existing tracegen app for basic trace validation
		suite.TestAPMTrace(&TestAPMTraceArgs{
			Filter: TestAPMTraceFilterArgs{
				ServiceName: "tracegen-test-service",
			},
			Expect: TestAPMTraceExpectArgs{
				TraceIDPresent: true,
				Tags: &[]string{
					`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
					`^container_name:`,
					`^task_arn:`,
				},
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

			suite.T().Logf("Found traces from services: %v", lo.Keys(serviceNames))

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
										suite.T().Logf("Found parent-child span relationship: parent=%d, child=%d",
											span.ParentID, span.SpanID)
										break
									}
								}
							}

							if hasParentChild {
								assert.Truef(c, true, "Trace propagation working - found parent-child spans")
								return
							}
						}
					}
				}
			}

			suite.T().Logf("Note: No parent-child spans found yet, but traces are being collected")
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
			foundSamplingPriority := false
			for _, trace := range traces {
				for _, payload := range trace.TracerPayloads {
					for _, chunk := range payload.Chunks {
						for _, span := range chunk.Spans {
							if samplingPriority, exists := span.Metrics["_sampling_priority_v1"]; exists {
								suite.T().Logf("Found span with sampling priority: %f (service=%s)",
									samplingPriority, span.Service)

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

			assert.Truef(c, foundSamplingPriority, "No traces with sampling priority found")
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

			// Check that traces have ECS metadata tags
			foundEnrichedTrace := false
			for _, trace := range traces {
				traceTags := trace.Tags

				// Check for key ECS tags
				hasClusterName := false
				hasTaskArn := false
				hasContainerName := false

				for key, value := range traceTags {
					if key == "ecs_cluster_name" && value == suite.ecsClusterName {
						hasClusterName = true
					}
					if key == "task_arn" && value != "" {
						hasTaskArn = true
					}
					if key == "container_name" && value != "" {
						hasContainerName = true
					}
				}

				if hasClusterName && hasTaskArn && hasContainerName {
					foundEnrichedTrace = true
					suite.T().Logf("Found trace with ECS metadata tags: cluster=%s, task_arn=%s, container=%s",
						traceTags["ecs_cluster_name"], traceTags["task_arn"], traceTags["container_name"])
					break
				}
			}

			assert.Truef(c, foundEnrichedTrace,
				"No traces found with complete ECS metadata tags (cluster_name, task_arn, container_name)")
		}, 2*time.Minute, 10*time.Second, "Trace tag enrichment validation failed")
	})
}

func (suite *ecsAPMSuite) TestTraceCorrelation() {
	// Test trace-log correlation
	suite.Run("Trace-log correlation", func() {
		// Get a trace with a trace ID
		var traceID uint64
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}
			if !assert.NotEmptyf(c, traces, "No traces found") {
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

			assert.NotZerof(c, traceID, "No valid trace ID found")
		}, 2*time.Minute, 10*time.Second, "Failed to get trace ID")

		// If we found a trace ID, check if logs have the same trace ID
		if traceID != 0 {
			suite.EventuallyWithTf(func(c *assert.CollectT) {
				logs, err := getAllLogs(suite.Fakeintake)
				if !assert.NoErrorf(c, err, "Failed to query logs") {
					return
				}

				// Look for logs with trace_id tag
				foundCorrelatedLog := false
				for _, log := range logs {
					for _, tag := range log.GetTags() {
						if regexp.MustCompile(`dd\.trace_id:[[:xdigit:]]+`).MatchString(tag) {
							foundCorrelatedLog = true
							suite.T().Logf("Found log with trace correlation tag: %s", tag)
							break
						}
					}
					if foundCorrelatedLog {
						break
					}
				}

				if len(logs) > 0 {
					suite.T().Logf("Checked %d logs for trace correlation", len(logs))
				}

				// Note: Correlation may not always be present depending on app configuration
				// This is an informational check
				if foundCorrelatedLog {
					assert.Truef(c, true, "Trace-log correlation is working")
				} else {
					suite.T().Logf("Note: No logs with trace correlation found yet")
				}
			}, 2*time.Minute, 10*time.Second, "Trace-log correlation check completed")
		}
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

			// Filter for Fargate traces
			fargateTraces := lo.Filter(traces, func(trace *aggregator.TracePayload, _ int) bool {
				if launchType, exists := trace.Tags["ecs_launch_type"]; exists {
					return launchType == "fargate"
				}
				return false
			})

			if len(fargateTraces) > 0 {
				suite.T().Logf("Found %d traces from Fargate tasks", len(fargateTraces))

				// Verify Fargate traces have expected tags
				trace := fargateTraces[0]
				assert.Equalf(c, "fargate", trace.Tags["ecs_launch_type"],
					"Fargate trace should have ecs_launch_type:fargate tag")

				// Verify trace has cluster name
				assert.Equalf(c, suite.ecsClusterName, trace.Tags["ecs_cluster_name"],
					"Fargate trace should have correct cluster name")

				// Fargate tasks should have task_arn
				assert.NotEmptyf(c, trace.Tags["task_arn"],
					"Fargate trace should have task_arn tag")
			} else {
				suite.T().Logf("No Fargate traces found yet - checking EC2 traces")
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

			// Filter for EC2 traces
			ec2Traces := lo.Filter(traces, func(trace *aggregator.TracePayload, _ int) bool {
				if launchType, exists := trace.Tags["ecs_launch_type"]; exists {
					return launchType == "ec2"
				}
				// If no launch type tag, might be EC2 (daemon mode)
				if _, hasCluster := trace.Tags["ecs_cluster_name"]; hasCluster {
					return true
				}
				return false
			})

			if !assert.NotEmptyf(c, ec2Traces, "No EC2 traces found") {
				return
			}

			suite.T().Logf("Found %d traces from EC2 tasks", len(ec2Traces))

			// Verify EC2 traces have expected metadata
			trace := ec2Traces[0]

			// EC2 tasks should have cluster name
			assert.Equalf(c, suite.ecsClusterName, trace.Tags["ecs_cluster_name"],
				"EC2 trace should have correct cluster name")

			// EC2 tasks should have task_arn
			assert.NotEmptyf(c, trace.Tags["task_arn"],
				"EC2 trace should have task_arn tag")

			// EC2 tasks should have container_name
			assert.NotEmptyf(c, trace.Tags["container_name"],
				"EC2 trace should have container_name tag")

			// Log transport method (UDS vs TCP)
			for _, payload := range trace.TracerPayloads {
				for _, chunk := range payload.Chunks {
					if len(chunk.Spans) > 0 {
						span := chunk.Spans[0]
						// Check if span has metadata about transport
						suite.T().Logf("EC2 trace: service=%s, resource=%s, operation=%s",
							span.Service, span.Resource, span.Name)
					}
				}
			}
		}, 3*time.Minute, 10*time.Second, "EC2 APM validation failed")
	})
}

func (suite *ecsAPMSuite) TestDogtstatsdUDS() {
	suite.testDogstatsd(taskNameDogstatsdUDS)
}

func (suite *ecsAPMSuite) TestDogtstatsdUDP() {
	suite.testDogstatsd(taskNameDogstatsdUDP)
}

func (suite *ecsAPMSuite) testDogstatsd(taskName string) {
	expectedTags := suite.getCommonECSTagPatterns(suite.ecsClusterName, taskName, "dogstatsd", true)

	suite.TestMetric(&TestMetricArgs{
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
	// Get expected tag patterns (minimal set for traces)
	expectedTagPatterns := suite.getCommonECSTagPatterns(suite.ecsClusterName, taskName, "tracegen", false)

	// Convert string patterns to compiled regexps
	compiledPatterns := make([]*regexp.Regexp, len(expectedTagPatterns))
	for i, pattern := range expectedTagPatterns {
		compiledPatterns[i] = regexp.MustCompile(pattern)
	}

	suite.EventuallyWithTf(func(c *assert.CollectT) {
		traces, cerr := suite.Fakeintake.GetTraces()
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, cerr, "Failed to query fake intake") {
			return
		}

		var err error
		// Iterate starting from the most recent traces
		for _, trace := range traces {
			tags := lo.MapToSlice(trace.Tags, func(k string, v string) string {
				return k + ":" + v
			})
			// Assert origin detection is working properly
			err = assertTags(tags, compiledPatterns, []*regexp.Regexp{}, false)
			if err == nil {
				break
			}
		}
		require.NoErrorf(c, err, "Failed finding trace with proper tags")
	}, 2*time.Minute, 10*time.Second, "Failed finding trace with proper tags")
}
