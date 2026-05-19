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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-pipelines/common"
)

// ec2DefaultCloudCostAllowlistedMetrics are system.* metrics on the built-in
// cloud_cost_only allowlist that core checks emit on awshost e2e stacks.
var ec2DefaultCloudCostAllowlistedMetrics = []string{
	"system.cpu.user",
	"system.mem.pct_usable",
	"system.net.bytes_rcvd",
	"system.net.bytes_sent",
}

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

	adpEnabled bool
}

type ccmModeDefaultTaggedSuite struct {
	ccmModeSuiteBase
}

type ccmModeConfiguredTaggedSuite struct {
	ccmModeSuiteBase
}

// ccmModeADPSuite exercises CCM DogStatsD paths with ADP serving port 8125.
// It embeds ccmModeSuiteBase only (not ccmModeDefaultTaggedSuite) so integration
// tagging tests are not run here — ADP does not affect check-sourced metrics.
type ccmModeADPSuite struct {
	ccmModeSuiteBase
}

func ccmAgentConfig(taggedChecks, metricsBlocked []string) string {
	cfg := `
infrastructure_mode: cloud_cost_only
metric_filterlist:
  - e2e.ccm.blocked.by.filterlist
`
	if len(taggedChecks) > 0 || len(metricsBlocked) > 0 {
		cfg += `integration:
  cloud_cost_only:
`
		if len(taggedChecks) > 0 {
			cfg += `    tagged:
`
			for _, check := range taggedChecks {
				cfg += fmt.Sprintf("      - %s\n", check)
			}
		}
		if len(metricsBlocked) > 0 {
			cfg += `    metrics_blocked:
`
			for _, metric := range metricsBlocked {
				cfg += fmt.Sprintf("      - %s\n", metric)
			}
		}
	}
	return cfg
}

func runCCMModeSuite[T e2e.Suite[environments.Host]](t *testing.T, stackName string, agentConfig string, suite T, adpEnabled bool) {
	t.Helper()
	t.Parallel()

	agentOptions := []agentparams.Option{agentparams.WithAgentConfig(agentConfig)}
	if adpEnabled {
		agentOptions = append(agentOptions, common.WithADPEnabled())
		stackName += "-adp"
	}

	e2e.Run(t, suite, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(agentOptions...),
			),
		),
	), e2e.WithStackName(stackName))
}

func (s *ccmModeSuiteBase) assertADPRunningIfEnabled() {
	s.T().Helper()
	if s.adpEnabled {
		common.AssertADPRunning(s.T(), s.Env().RemoteHost)
	}
}

// TestCCMModeLinuxDefaultTagged runs CCM e2e checks with the default empty
// integration.cloud_cost_only.tagged list (all checks receive infra_mode).
func TestCCMModeLinuxDefaultTagged(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-default-tagged", ccmAgentConfig(nil, nil), &ccmModeDefaultTaggedSuite{}, false)
}

// TestCCMModeLinuxADP runs CCM DogStatsD e2e checks with ADP serving port 8125 traffic.
func TestCCMModeLinuxADP(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-adp", ccmAgentConfig(nil, nil), &ccmModeADPSuite{
		ccmModeSuiteBase: ccmModeSuiteBase{adpEnabled: true},
	}, true)
}

// TestCCMModeLinuxConfiguredTagged runs CCM e2e checks with an explicit tagged list.
func TestCCMModeLinuxConfiguredTagged(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-configured-tagged", ccmAgentConfig([]string{"cpu"}, nil), &ccmModeConfiguredTaggedSuite{}, false)
}

type ccmModeMetricsBlockedSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestCCMModeLinuxMetricsBlocked runs CCM e2e checks with integration.cloud_cost_only.metrics_blocked set.
func TestCCMModeLinuxMetricsBlocked(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-metrics-blocked", ccmAgentConfig(nil, []string{"system.cpu"}), &ccmModeMetricsBlockedSuite{}, false)
}

// TestMetricsBlockedOverridesAllowlist verifies metrics_blocked drops allowlisted metrics
// when integration.cloud_cost_only.metrics_match_prefix is true (default).
func (s *ccmModeMetricsBlockedSuite) TestMetricsBlockedOverridesAllowlist() {
	const blockedMetric = "system.cpu.user"
	const allowedMetric = "system.net.bytes_rcvd"

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		blocked, err := s.Env().FakeIntake.Client().FilterMetrics(
			blockedMetric,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.Empty(c, blocked, "%s should be dropped by metrics_blocked despite the default allowlist", blockedMetric)

		allowed, err := s.Env().FakeIntake.Client().FilterMetrics(
			allowedMetric,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, allowed, "%s should still be forwarded when not on metrics_blocked", allowedMetric)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for metrics_blocked to override allowlist")
}

func ccmAgentConfigEmptyAllowlist() string {
	return `
infrastructure_mode: cloud_cost_only
integration:
  cloud_cost_only:
    metrics: []
`
}

type ccmModeEmptyAllowlistSuite struct {
	ccmModeSuiteBase
}

// TestCCMModeLinuxEmptyAllowlist runs CCM e2e checks with an explicit empty metrics allowlist.
func TestCCMModeLinuxEmptyAllowlist(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-empty-allowlist", ccmAgentConfigEmptyAllowlist(), &ccmModeEmptyAllowlistSuite{}, false)
}

// TestEmptyAllowlistDeniesIntegrationMetrics verifies integration.cloud_cost_only.metrics: []
// forwards no integration metric names (bypass paths such as DogStatsD still apply).
func (s *ccmModeEmptyAllowlistSuite) TestEmptyAllowlistDeniesIntegrationMetrics() {
	const (
		onDefaultAllowlist = "system.mem.pct_usable"
		offAllowlist       = "system.disk.free"
	)

	s.assertADPRunningIfEnabled()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		onList, err := s.Env().FakeIntake.Client().FilterMetrics(
			onDefaultAllowlist,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.Empty(c, onList, "%s should be dropped when metrics is explicitly empty", onDefaultAllowlist)

		offList, err := s.Env().FakeIntake.Client().FilterMetrics(
			offAllowlist,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.Empty(c, offList, "%s should be dropped when metrics is explicitly empty", offAllowlist)

		s.sendStatsdGauge(dogstatsdCustomMetric, 1)
		dogstatsd, err := s.Env().FakeIntake.Client().FilterMetrics(
			dogstatsdCustomMetric,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, dogstatsd, "%s should still bypass via DogStatsD when metrics is empty", dogstatsdCustomMetric)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for empty allowlist to deny integration metrics")
}

func ccmAgentConfigCustomAllowlist(allowlist []string, matchPrefix bool) string {
	cfg := `
infrastructure_mode: cloud_cost_only
integration:
  cloud_cost_only:
    metrics:
`
	for _, metric := range allowlist {
		cfg += fmt.Sprintf("      - %s\n", metric)
	}
	cfg += fmt.Sprintf("    metrics_match_prefix: %t\n", matchPrefix)
	return cfg
}

type ccmModeCustomAllowlistSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestCCMModeLinuxCustomAllowlist runs CCM e2e checks with a user-defined metrics allowlist.
func TestCCMModeLinuxCustomAllowlist(t *testing.T) {
	runCCMModeSuite(
		t,
		"ccmmode-custom-allowlist",
		ccmAgentConfigCustomAllowlist([]string{"system.mem.pct_usable"}, false),
		&ccmModeCustomAllowlistSuite{},
		false,
	)
}

// TestCustomAllowlistApplies verifies integration.cloud_cost_only.metrics overrides defaults:
// listed metrics are forwarded, unlisted integration metrics are dropped, DogStatsD still bypasses.
func (s *ccmModeCustomAllowlistSuite) TestCustomAllowlistApplies() {
	const (
		onCustomAllowlist  = "system.mem.pct_usable"
		offCustomAllowlist = "system.cpu.user"
	)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		onList, err := s.Env().FakeIntake.Client().FilterMetrics(
			onCustomAllowlist,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, onList, "%s should be forwarded when on the custom allowlist", onCustomAllowlist)

		offList, err := s.Env().FakeIntake.Client().FilterMetrics(
			offCustomAllowlist,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.Empty(c, offList, "%s should be dropped when not on the custom allowlist", offCustomAllowlist)

		s.sendStatsdGauge(dogstatsdCustomMetric, 1)
		dogstatsd, err := s.Env().FakeIntake.Client().FilterMetrics(
			dogstatsdCustomMetric,
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, dogstatsd, "%s should bypass the custom allowlist via DogStatsD", dogstatsdCustomMetric)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for custom allowlist filtering")
}

func (s *ccmModeCustomAllowlistSuite) sendStatsdGauge(name string, value int) {
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s:%d|g" > /dev/udp/127.0.0.1/8125'`, name, value)
	s.Env().RemoteHost.MustExecute(cmd)
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
// are still forwarded on awshost.
func (s *ccmModeSuiteBase) TestAllowlistedIntegrationMetricForwarded() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for _, metricName := range ec2DefaultCloudCostAllowlistedMetrics {
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
	s.assertADPRunningIfEnabled()

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
	s.assertADPRunningIfEnabled()

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
	s.assertADPRunningIfEnabled()

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
