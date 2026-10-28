// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inframode

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const infrastructureModeTag = "infra_mode:cloud_cost_only"

const ccmAgentConfig = `
infrastructure_mode: cloud_cost_only
`

type ccmModeSuiteBase struct {
	e2e.BaseSuite[environments.Host]
}

type ccmModeLinuxSuite struct {
	ccmModeSuiteBase
}

func runCCMModeSuite[T e2e.Suite[environments.Host]](t *testing.T, stackName string, suite T) {
	t.Helper()
	t.Parallel()

	agentOptions := []agentparams.Option{agentparams.WithAgentConfig(ccmAgentConfig)}

	e2e.Run(t, suite, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(agentOptions...),
			),
		),
	), e2e.WithStackName(stackName))
}

func (s *ccmModeSuiteBase) assertMetricHasInfrastructureModeTag(c *assert.CollectT, metricName, checkName string) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
		metricName,
		client.WithTags[*aggregator.MetricSeries]([]string{infrastructureModeTag}),
		client.WithMetricValueHigherThan(0),
	)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics, "%s from %s check should be forwarded with %s", metricName, checkName, infrastructureModeTag)
}

// TestCCMModeLinux runs CCM e2e checks in cloud_cost_only infrastructure mode.
func TestCCMModeLinux(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-linux", &ccmModeLinuxSuite{})
}

// TestHostTags verifies that the infra_mode:cloud_cost_only marker tag is attached to
// the agent's host-tags payload when running in cloud_cost_only infrastructure mode.
func (s *ccmModeLinuxSuite) TestHostTags() {
	fakeintake := s.Env().FakeIntake.Client()
	hostname := s.Env().Agent.Client.Hostname()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		payloads, err := fakeintake.GetHostTags(hostname)
		if !assert.NoError(c, err, "failed to fetch host-tags for %s", hostname) {
			return
		}
		if !assert.NotEmpty(c, payloads, "no host-tags payloads for %s", hostname) {
			return
		}

		// Latest payload: host_tags are eventually consistent.
		tags := payloads[len(payloads)-1].HostTags

		assert.Contains(c, tags, infrastructureModeTag,
			"expected infra_mode marker on host %s; got %v", hostname, tags)
	}, 5*time.Minute, 15*time.Second, "cloud_cost_only host tags did not appear in fakeintake host-tags payload")
}

// TestMetricsReceiveInfrastructureModeTag verifies cloud_cost_only tags metrics from
// multiple integrations.
func (s *ccmModeLinuxSuite) TestMetricsReceiveInfrastructureModeTag() {
	metricsByCheck := []struct {
		metric string
		check  string
	}{
		{"system.cpu.user", "cpu"},
		{"system.net.bytes_rcvd", "network"},
		{"system.mem.pct_usable", "memory"},
	}

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for _, tc := range metricsByCheck {
			s.assertMetricHasInfrastructureModeTag(c, tc.metric, tc.check)
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for cloud_cost_only-tagged metrics")
}
