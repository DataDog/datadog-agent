// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ccmmode

import (
	"fmt"
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

const (
	infraModeTag               = "infra_mode:cloud_cost_only"
	dogstatsdCustomMetric      = "e2e.ccm.dogstatsd.custom"
	dogstatsdFilterListAllowed = "e2e.ccm.dogstatsd.allowed"
	dogstatsdFilterListBlocked = "e2e.ccm.blocked.by.filterlist"
)

type ccmModeSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestCCMModeLinux runs cloud-cost-only infra tagging e2e checks on a Linux EC2 host with fakeintake.
func TestCCMModeLinux(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ccmModeSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(`
infrastructure_mode: cloud_cost_only
metric_filterlist:
  - e2e.ccm.blocked.by.filterlist
integration:
  cloud_cost_only:
    tagged:
      - cpu
`),
				),
			),
		),
	))
}

// TestTaggedCoreCheckMetricsIncludeCCMModeTag verifies metrics from the cpu core check
// carry the infra_mode tag when that integration is listed under integration.cloud_cost_only.tagged.
func (s *ccmModeSuite) TestTaggedCoreCheckMetricsIncludeCCMModeTag() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.cpu.user",
			client.WithTags[*aggregator.MetricSeries]([]string{infraModeTag}),
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "expected system.cpu.user with %s on fakeintake", infraModeTag)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for tagged cpu metrics")
}

// TestAllowlistedIntegrationMetricForwarded verifies integration metrics on the default
// cloud_cost_only allowlist are still forwarded.
func (s *ccmModeSuite) TestAllowlistedIntegrationMetricForwarded() {
	const metricName = "system.mem.pct_usable"

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			metricName,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "%s should be forwarded on the cloud_cost_only allowlist", metricName)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for allowlisted memory metric on fakeintake")
}

// TestMetricFilterListAppliesInCloudCostMode verifies metric_filterlist still drops DogStatsD
// metrics in cloud_cost_only mode even though DogStatsD bypasses the cloud_cost allowlist.
func (s *ccmModeSuite) TestMetricFilterListAppliesInCloudCostMode() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.sendStatsdGauge(dogstatsdFilterListAllowed, 1)
		s.sendStatsdGauge(dogstatsdFilterListBlocked, 1)

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			dogstatsdFilterListAllowed,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "%s should be forwarded via DogStatsD", dogstatsdFilterListAllowed)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for allowed DogStatsD metric on fakeintake")

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
		dogstatsdFilterListBlocked,
		client.WithMetricValueHigherThan(0),
	)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), metrics, "%s should be dropped by metric_filterlist", dogstatsdFilterListBlocked)
}

// sendStatsdGauge sends a DogStatsD gauge metric to the agent via UDP on the remote host.
func (s *ccmModeSuite) sendStatsdGauge(name string, value int) {
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s:%d|g" > /dev/udp/127.0.0.1/8125'`, name, value)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestDogstatsdCustomMetricForwarded verifies DogStatsD metrics are forwarded in cloud_cost_only
// mode even when the metric name is not on integration.cloud_cost_only.metrics.
func (s *ccmModeSuite) TestDogstatsdCustomMetricForwarded() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.sendStatsdGauge(dogstatsdCustomMetric, 1)

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			dogstatsdCustomMetric,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "%s should be forwarded via DogStatsD in cloud_cost_only mode", dogstatsdCustomMetric)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for DogStatsD custom metric on fakeintake")
}

// TestNonAllowlistedIntegrationMetricDropped verifies integration metrics outside
// integration.cloud_cost_only.metrics are not forwarded when infrastructure_mode is cloud_cost_only.
func (s *ccmModeSuite) TestNonAllowlistedIntegrationMetricDropped() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.disk.free",
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.Empty(c, metrics, "system.disk.free should be dropped by the cloud_cost_only metric allowlist")
	}, 3*time.Minute, 10*time.Second, "timed out waiting for allowlist to drop disk metrics")
}
