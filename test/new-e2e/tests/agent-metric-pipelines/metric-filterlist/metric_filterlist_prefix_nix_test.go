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
)

const (
	prefixListPrefix          = "e2e.metric.filterlist.prefixed."
	prefixListBlockedMetric   = prefixListPrefix + "blocked"
	prefixListExceptedMetric  = prefixListPrefix + "excepted"
	prefixListUnrelatedMetric = "e2e.metric.filterlist.unrelated"
)

type metricFilterListPrefixSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestMetricFilterListPrefix runs the metric_filterlist_prefix e2e test on
// Linux. metric_filterlist_prefix is not exercised with ADP enabled: ADP only
// reads the flat metric_filterlist / metric_filterlist_match_prefix format
// (see TestMetricFilterListADP), and is unaffected by this setting.
func TestMetricFilterListPrefix(t *testing.T) {
	t.Parallel()

	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(fmt.Sprintf(`
metric_filterlist_prefix:
  - prefix: "%s"
    except_exact:
      - "%s"
`, prefixListPrefix, prefixListExceptedMetric)),
	}

	e2e.Run(t, &metricFilterListPrefixSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(agentOptions...),
				),
			),
		),
		e2e.WithStackName("metricfilterlist-prefix"),
	)
}

// sendStatsdGauge sends a DogStatsD gauge metric to the agent via UDP on the remote host.
func (s *metricFilterListPrefixSuite) sendStatsdGauge(name string, value int) {
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s:%d|g" > /dev/udp/127.0.0.1/8125'`, name, value)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestMetricFilterListPrefixBlocksMatchingMetrics verifies that:
//   - a metric matching the metric_filterlist_prefix prefix is NOT forwarded
//     to the intake
//   - a metric matching the prefix but listed in its except_exact IS
//     forwarded normally
//   - a metric NOT matching the prefix at all IS forwarded normally
func (s *metricFilterListPrefixSuite) TestMetricFilterListPrefixBlocksMatchingMetrics() {
	// Send all three metrics on each retry so metrics keep flowing until the
	// pipeline confirms a flush.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.sendStatsdGauge(prefixListUnrelatedMetric, 1)
		s.sendStatsdGauge(prefixListExceptedMetric, 1)
		s.sendStatsdGauge(prefixListBlockedMetric, 1)

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(prefixListUnrelatedMetric)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "unrelated metric should be forwarded to fakeintake")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for unrelated metric to reach fakeintake")

	// At this point the aggregation pipeline has flushed at least once.
	// Verify the excepted metric was forwarded, and the blocked metric never
	// reached fakeintake.
	excepted, err := s.Env().FakeIntake.Client().FilterMetrics(prefixListExceptedMetric)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), excepted, "except_exact metric should still be forwarded to fakeintake")

	blocked, err := s.Env().FakeIntake.Client().FilterMetrics(prefixListBlockedMetric)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), blocked, "metric matching the prefix should not have been forwarded to fakeintake")
}
