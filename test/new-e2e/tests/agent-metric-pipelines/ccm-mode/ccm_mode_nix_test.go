// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ccmmode

import (
	"fmt"
	"strings"
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
	dogstatsdJMXMetric         = "e2e.ccm.jmx.metric"
	dogstatsdFilterListAllowed = "e2e.ccm.dogstatsd.allowed"
	dogstatsdFilterListBlocked = "e2e.ccm.blocked.by.filterlist"
	jmxCheckNameTag            = "dd.internal.jmx_check_name:custom_e2e"
)

type ccmModeSuiteBase struct {
	e2e.BaseSuite[environments.Host]
}

type ccmModeDefaultTaggedSuite struct {
	ccmModeSuiteBase
}

type ccmModeConfiguredTaggedSuite struct {
	ccmModeSuiteBase
}

func ccmAgentConfig(taggedChecks []string) string {
	cfg := `
infrastructure_mode: cloud_cost_only
metric_filterlist:
  - e2e.ccm.blocked.by.filterlist
`
	if len(taggedChecks) > 0 {
		cfg += `integration:
  cloud_cost_only:
    tagged:
`
		for _, check := range taggedChecks {
			cfg += fmt.Sprintf("      - %s\n", check)
		}
	}
	return cfg
}

func runCCMModeSuite[T e2e.Suite[environments.Host]](t *testing.T, stackName string, agentConfig string, suite T) {
	t.Helper()
	t.Parallel()
	e2e.Run(t, suite, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
				),
			),
		),
	), e2e.WithStackName(stackName))
}

// TestCCMModeLinuxDefaultTagged runs CCM e2e checks with the default empty
// integration.cloud_cost_only.tagged list (all checks receive infra_mode).
func TestCCMModeLinuxDefaultTagged(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-default-tagged", ccmAgentConfig(nil), &ccmModeDefaultTaggedSuite{})
}

// TestCCMModeLinuxConfiguredTagged runs CCM e2e checks with an explicit tagged list.
func TestCCMModeLinuxConfiguredTagged(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-configured-tagged", ccmAgentConfig([]string{"cpu"}), &ccmModeConfiguredTaggedSuite{})
}

func (s *ccmModeSuiteBase) assertMetricHasInfraModeTag(c *assert.CollectT, metricName, checkName string) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
		metricName,
		client.WithTags[*aggregator.MetricSeries]([]string{infraModeTag}),
		client.WithMetricValueHigherThan(0),
	)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics, "%s from %s check should be forwarded with %s", metricName, checkName, infraModeTag)
}

func (s *ccmModeSuiteBase) assertMetricLacksInfraModeTag(c *assert.CollectT, metricName, checkName string) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
		metricName,
		client.WithMetricValueHigherThan(0),
	)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics, "%s from %s check should be forwarded", metricName, checkName)
	for _, m := range metrics {
		assert.NotContains(c, m.GetTags(), infraModeTag, "%s from %s check should not carry %s", metricName, checkName, infraModeTag)
	}
}

// TestDefaultTaggedAllChecksReceiveInfraModeTag verifies the default empty tagged list
// tags metrics from multiple integrations, including checks outside a typical tagged: [cpu] config.
func (s *ccmModeDefaultTaggedSuite) TestDefaultTaggedAllChecksReceiveInfraModeTag() {
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
			s.assertMetricHasInfraModeTag(c, tc.metric, tc.check)
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for default-tagged metrics")
}

// TestConfiguredTaggedAppliesSelectively verifies integration.cloud_cost_only.tagged limits
// which checks receive infra_mode when the list is non-empty.
func (s *ccmModeConfiguredTaggedSuite) TestConfiguredTaggedAppliesSelectively() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.assertMetricHasInfraModeTag(c, "system.cpu.user", "cpu")
		s.assertMetricLacksInfraModeTag(c, "system.net.bytes_rcvd", "network")
	}, 3*time.Minute, 10*time.Second, "timed out waiting for configured selective tagging")
}

// TestAllowlistedIntegrationMetricForwarded verifies default-allowlist system.* metrics
// are still forwarded on awshost (see defaultCloudCostAllowlistedMetrics).
func (s *ccmModeSuiteBase) TestAllowlistedIntegrationMetricForwarded() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for _, metricName := range ec2CloudCostAllowlistedMetrics() {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
				metricName,
				client.WithMetricValueHigherThan(0),
			)
			assert.NoError(c, err)
			assert.NotEmpty(c, metrics, "%s should be forwarded on the cloud_cost_only allowlist", metricName)
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for allowlisted system metrics on fakeintake")
}

// TestMetricFilterListAppliesInCloudCostMode verifies metric_filterlist still drops DogStatsD
// metrics in cloud_cost_only mode even though DogStatsD bypasses the cloud_cost allowlist.
func (s *ccmModeSuiteBase) TestMetricFilterListAppliesInCloudCostMode() {
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
func (s *ccmModeSuiteBase) sendStatsdGauge(name string, value int) {
	s.sendStatsdGaugeWithTags(name, value, nil)
}

// sendStatsdGaugeWithTags sends a DogStatsD gauge with optional tags (DogStatsD #tag format).
func (s *ccmModeSuiteBase) sendStatsdGaugeWithTags(name string, value int, tags []string) {
	payload := fmt.Sprintf("%s:%d|g", name, value)
	if len(tags) > 0 {
		payload += "|#" + strings.Join(tags, ",")
	}
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s" > /dev/udp/127.0.0.1/8125'`, payload)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestDogstatsdJMXTaggedMetricForwarded verifies JMX-tagged DogStatsD metrics are forwarded in
// cloud_cost_only mode via FromDogstatsd even when the name is not on the integration allowlist.
func (s *ccmModeSuiteBase) TestDogstatsdJMXTaggedMetricForwarded() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.sendStatsdGaugeWithTags(dogstatsdJMXMetric, 1, []string{jmxCheckNameTag})

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			dogstatsdJMXMetric,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "%s should be forwarded via JMX-tagged DogStatsD in cloud_cost_only mode", dogstatsdJMXMetric)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for JMX-tagged DogStatsD metric on fakeintake")
}

// TestDogstatsdCustomMetricForwarded verifies DogStatsD metrics are forwarded in cloud_cost_only
// mode even when the metric name is not on integration.cloud_cost_only.metrics.
func (s *ccmModeSuiteBase) TestDogstatsdCustomMetricForwarded() {
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
func (s *ccmModeSuiteBase) TestNonAllowlistedIntegrationMetricDropped() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.disk.free",
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.Empty(c, metrics, "system.disk.free should be dropped by the cloud_cost_only metric allowlist")
	}, 3*time.Minute, 10*time.Second, "timed out waiting for allowlist to drop disk metrics")
}
