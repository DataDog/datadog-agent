// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricfilterlist contains e2e tests for the metric_filterlist feature.
package metricfilterlist

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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-pipelines/common"
)

const (
	allowedMetric = "e2e.metric.filterlist.allowed"
	blockedMetric = "e2e.metric.filterlist.blocked"

	// rcPrefixPattern is delivered through Remote Config's metric_filterlist_prefix
	// key (see the RFC "metric prefix filtering configuration"): it blocks every
	// name under the prefix except rcExceptedMetric.
	rcPrefixPattern  = "e2e.metric.filterlist.rcprefix.*"
	rcBlockedMetric  = "e2e.metric.filterlist.rcprefix.blocked"
	rcExceptedMetric = "e2e.metric.filterlist.rcprefix.excepted"
)

type metricFilterListSuite struct {
	e2e.BaseSuite[environments.Host]

	adpEnabled bool
}

func testMetricFilterList(t *testing.T, adpEnabled bool) {
	t.Parallel()

	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(fmt.Sprintf(`
metric_filterlist:
  - "%s"
`, blockedMetric)),
	}
	if adpEnabled {
		agentOptions = append(agentOptions, common.WithADPEnabled())
	}

	stackName := "metricfilterlist"
	if adpEnabled {
		stackName += "-adp"
	}

	e2e.Run(t, &metricFilterListSuite{adpEnabled: adpEnabled},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(agentOptions...),
				),
			),
		),
		e2e.WithStackName(stackName),
	)
}

// TestMetricFilterList runs the metric_filterlist e2e test on Linux.
func TestMetricFilterList(t *testing.T) {
	testMetricFilterList(t, false)
}

// TestMetricFilterListADP runs the metric_filterlist e2e test with ADP serving DogStatsD traffic.
func TestMetricFilterListADP(t *testing.T) {
	testMetricFilterList(t, true)
}

// sendStatsdGauge sends a DogStatsD gauge metric to the agent via UDP on the remote host.
func (s *metricFilterListSuite) sendStatsdGauge(name string, value int) {
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s:%d|g" > /dev/udp/127.0.0.1/8125'`, name, value)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestMetricFilterListBlocksMetric verifies that:
//   - a metric listed in metric_filterlist is NOT forwarded to the intake
//   - a metric NOT in metric_filterlist IS forwarded normally
func (s *metricFilterListSuite) TestMetricFilterListBlocksMetric() {
	if s.adpEnabled {
		common.AssertADPRunning(s.T(), s.Env().RemoteHost)
	}

	// Send both metrics on each retry so metrics keep flowing until the pipeline confirms a flush.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.sendStatsdGauge(allowedMetric, 1)
		s.sendStatsdGauge(blockedMetric, 1)

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(allowedMetric)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "allowed metric should be forwarded to fakeintake")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for allowed metric to reach fakeintake")

	// At this point the aggregation pipeline has flushed at least once.
	// Verify the blocked metric never reached fakeintake.
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(blockedMetric)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), metrics, "filtered metric should not have been forwarded to fakeintake")
}

// TestMetricFilterListPrefixExceptionsViaRC verifies that a metric_filterlist_prefix
// entry delivered through Remote Config -- the schema described by the RFC
// "metric prefix filtering configuration" -- drops every metric name matching
// its prefix except the ones listed in its exception.
//
// metric_filterlist_prefix is a Go Agent-only key: Agent Data Plane's own
// config parser only understands the flat metric_filterlist list, so this
// test does not run against the ADP entry point.
func (s *metricFilterListSuite) TestMetricFilterListPrefixExceptionsViaRC() {
	if s.adpEnabled {
		s.T().Skip("metric_filterlist_prefix is not read by Agent Data Plane, only by the Go Agent")
	}

	fi := s.Env().FakeIntake.Client()

	// Push a metric_filterlist_prefix entry through Remote Config: block
	// everything under rcPrefixPattern except rcExceptedMetric.
	err := fi.RCAddConfig("", "METRIC_CONTROL", "prefix-exceptions", "prefix-exceptions", []byte(fmt.Sprintf(`{
		"metric_filterlist_prefix": {
			"by_name": {
				"values": [
					{"name": %q, "except_exact": [{"name": %q}]}
				]
			}
		}
	}`, rcPrefixPattern, rcExceptedMetric)))
	require.NoError(s.T(), err)

	// Wait until the Agent has actually applied the RC update before sending
	// any metric: `agent config get` mirrors the value Remote Config set, so
	// the pushed prefix shows up there once polled
	// (remote_configuration.refresh_interval is 5s). Confirming this first --
	// rather than racing metric delivery against the RC poll -- is what makes
	// the "never reached fakeintake" assertion below safe: without it, a
	// metric sent before the rule is applied could reach fakeintake and be
	// mistaken for a bug.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		cfg, err := s.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"get", "metric_filterlist_prefix"}))
		assert.NoError(c, err)
		assert.Contains(c, cfg, rcPrefixPattern, "metric_filterlist_prefix not yet updated by RC")
	}, 2*time.Minute, 5*time.Second, "the RC prefix rule was never applied")

	// The RC rule is confirmed active: start from a clean intake so the
	// assertions below only see traffic sent after this point.
	require.NoError(s.T(), fi.FlushServerAndResetAggregators())

	// Send both metrics on each retry so metrics keep flowing until the pipeline confirms a flush.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.sendStatsdGauge(rcExceptedMetric, 1)
		s.sendStatsdGauge(rcBlockedMetric, 1)

		metrics, err := fi.FilterMetrics(rcExceptedMetric)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "excepted metric should be forwarded to fakeintake despite matching the RC prefix rule")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for the excepted metric to reach fakeintake")

	// At this point the aggregation pipeline has flushed at least once since
	// the reset above. Verify the metric matching only the prefix (and not
	// the exception) never reached fakeintake.
	metrics, err := fi.FilterMetrics(rcBlockedMetric)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), metrics, "metric matching the RC prefix rule should not have been forwarded to fakeintake")
}
