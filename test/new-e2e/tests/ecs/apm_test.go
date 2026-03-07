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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/stretchr/testify/assert"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	scenfi "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
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
			scenecs.WithFakeIntakeOptions(
				scenfi.WithRetentionPeriod("31m"),
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
// The bundled _dd.tags.container value is a comma-separated string of key:value pairs
// containing ECS metadata, image metadata, and git metadata.
func (suite *ecsAPMSuite) testTrace(taskName string) {
	// Build validation patterns for the bundled _dd.tags.container value
	patterns := []*regexp.Regexp{
		// Core ECS metadata
		regexp.MustCompile(`ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName)),
		regexp.MustCompile(`task_arn:`),
		regexp.MustCompile(`container_name:`),
		regexp.MustCompile(`ecs_container_name:tracegen`),
		regexp.MustCompile(`task_family:.*-` + regexp.QuoteMeta(taskName) + `-ec2`),
		regexp.MustCompile(`task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2`),
		regexp.MustCompile(`task_version:[[:digit:]]+`),

		// Image metadata
		regexp.MustCompile(`docker_image:ghcr\.io/datadog/apps-tracegen:` + regexp.QuoteMeta(apps.Version)),
		regexp.MustCompile(`image_name:ghcr\.io/datadog/apps-tracegen`),
		regexp.MustCompile(`image_tag:` + regexp.QuoteMeta(apps.Version)),
		regexp.MustCompile(`short_image:apps-tracegen`),

		// Git metadata
		regexp.MustCompile(`git\.commit\.sha:[[:xdigit:]]{40}`),
		regexp.MustCompile(`git.repository_url:https://github.com/DataDog/test-infra-definitions`),
	}

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

				// Validate all patterns match the bundled tag value
				allMatch := true
				for _, pattern := range patterns {
					if !pattern.MatchString(containerTags) {
						allMatch = false
						break
					}
				}
				if allMatch {
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
