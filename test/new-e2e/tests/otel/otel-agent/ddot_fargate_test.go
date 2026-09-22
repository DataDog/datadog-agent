// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	otelstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/otel-standalone"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed config/ddot-fargate.yml
var ddotFargateConfig string

type ddotFargateTestSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestDDOTFargateSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ddotFargateTestSuite{},
		e2e.WithProvisioner(ecs.Provisioner(
			ecs.WithRunOptions(
				scenecs.WithECSOptions(scenecs.WithFargateCapacityProvider()),
				scenecs.WithFargateWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake) (*ecsComp.Workload, error) {
					return otelstandalone.FargateAppDefinition(e, clusterArn, apiKeySSMParamName, fakeIntake, ddotFargateConfig)
				}),
			),
		)),
	)
}

func (s *ddotFargateTestSuite) TestDDOTCollectorRunningMetricTaggedWithTaskARN() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	const fargateMetricName = "otel.ddot_collector.metrics.running.fargate"

	var metrics []*aggregator.MetricSeries
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err = s.Env().FakeIntake.Client().FilterMetrics(fargateMetricName)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 10*time.Second)

	require.NotEmpty(s.T(), metrics)
	m := metrics[0]
	require.NotEmpty(s.T(), m.Points)
	assert.Equal(s.T(), 1.0, m.Points[0].Value, "%s should always be 1.0", fargateMetricName)

	var hasTaskARNTag bool
	for _, tag := range m.GetTags() {
		if strings.HasPrefix(tag, "task_arn:arn:aws:ecs:") {
			hasTaskARNTag = true
			break
		}
	}
	assert.True(s.T(), hasTaskARNTag, "expected a task_arn:arn:aws:ecs:... tag, got tags: %v", m.GetTags())

	for _, resource := range m.Resources {
		assert.NotEqual(s.T(), "host", resource.Type, "%s should not carry a host resource, got: %v", fargateMetricName, resource)
	}

	hostlessMetrics, err := s.Env().FakeIntake.Client().FilterMetrics("otel.ddot_collector.metrics.running")
	require.NoError(s.T(), err)
	assert.Empty(s.T(), hostlessMetrics, "otel.ddot_collector.metrics.running should not be emitted on ECS Fargate; only the .fargate variant should be")
}
