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
	allowedMetric = "e2e.metric.filterlist.allowed"
	blockedMetric = "e2e.metric.filterlist.blocked"
)

type metricFilterListSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestMetricFilterList runs the metric_filterlist e2e test on Linux.
func TestMetricFilterList(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &metricFilterListSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(fmt.Sprintf(`
metric_filterlist:
  - "%s"
`, blockedMetric)),
				),
			),
		),
	))
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
