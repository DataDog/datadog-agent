// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/stretchr/testify/assert"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	scenfi "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
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
			scenecs.WithFakeIntakeOptions(
				scenfi.WithRetentionPeriod("31m"),
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

func (suite *ecsManagedSuite) Test00UpAndRunning() {
	suite.AssertECSTasksReady(suite.ecsClusterName)
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

func (suite *ecsManagedSuite) TestManagedInstanceTraceCollection() {
	// Test trace collection from managed instances
	suite.Run("Managed instance trace collection", func() {
		// ECS metadata on traces is bundled in _dd.tags.container within TracerPayload.Tags.
		// Patterns use ^ and $ anchors and are matched against individual comma-separated tags.
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^task_arn:`),
			regexp.MustCompile(`^container_name:`),
		}

		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}
			if !assert.NotEmptyf(c, traces, "No traces received yet") {
				return
			}

			// Check traces from managed instances via bundled _dd.tags.container tag
			found := false
			for _, trace := range traces {
				for _, tracerPayload := range trace.TracerPayloads {
					containerTags, exists := tracerPayload.Tags["_dd.tags.container"]
					if !exists {
						continue
					}
					tags := strings.Split(containerTags, ",")
					allMatch := true
					for _, pattern := range patterns {
						matched := false
						for _, tag := range tags {
							if pattern.MatchString(tag) {
								matched = true
								break
							}
						}
						if !matched {
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

			assert.Truef(c, found, "No traces with ECS metadata (cluster_name, task_arn, container_name) found in _dd.tags.container")
		}, 3*time.Minute, 10*time.Second, "Managed instance trace collection validation failed")
	})
}
