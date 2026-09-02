// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentruntimes

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// internalTelemetryCheckConfig turns on the curated internal telemetry set. It lands at
// conf.d/telemetry.d/conf.yaml, taking precedence over the conf.yaml.default the package ships.
const internalTelemetryCheckConfig = `init_config:

instances:
  - internal_telemetry:
      enabled: true
`

// notCuratedMetric is reported into the regular telemetry registry by the forwarder on every
// submission, but is deliberately absent from the check's curated allowlist. It stands in for
// "everything the curated set is supposed to leave behind".
const notCuratedMetric = "datadog.agent.transactions.input_bytes"

type internalTelemetrySuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestInternalTelemetrySuite covers the telemetry core check's internal_telemetry option, which
// reports the Agent's in-process telemetry registry as datadog.agent.* metrics.
//
// The unit tests in pkg/collector/corechecks/telemetry feed the check a fake registry built from
// the same allowlist they assert on, so they cannot catch the allowlist drifting away from the
// metric names a real Agent registers, nor new instrumentation that is registered but never
// incremented. That is what this suite is for.
func TestInternalTelemetrySuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &internalTelemetrySuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithIntegration("telemetry.d", internalTelemetryCheckConfig),
				),
			),
		)),
	)
}

// TestSchedulerQueueSize covers a gauge added alongside the internal_telemetry option. The
// scheduler always holds checks in its 15s queue, so a value of zero means the gauge is
// registered but never updated.
func (s *internalTelemetrySuite) TestSchedulerQueueSize() {
	s.assertGaugeReported("datadog.agent.scheduler.queue_size",
		[]string{"shadow:false"},
		[]string{"interval", "shadow"},
	)
}

// TestAggregatorFlushTime covers a gauge added alongside the internal_telemetry option, and pins
// one real flush_type value so a change to the tag conversion or to the flush names is caught.
func (s *internalTelemetrySuite) TestAggregatorFlushTime() {
	s.assertGaugeReported("datadog.agent.aggregator.flush_time",
		[]string{"flush_type:MainFlushTime"},
		[]string{"flush_type"},
	)
}

// TestCuratedCountersReported covers the counter path: one metric added with the option and one
// that predates it. Values are not asserted because a monotonic count reports the delta since the
// previous flush, which is legitimately zero on a quiet interval.
func (s *internalTelemetrySuite) TestCuratedCountersReported() {
	for _, tc := range []struct {
		metric  string
		tagKeys []string
	}{
		{metric: "datadog.agent.aggregator.number_of_flush"},
		{metric: "datadog.agent.transactions.success", tagKeys: []string{"domain", "endpoint"}},
	} {
		s.Run(tc.metric, func() {
			s.EventuallyWithT(func(c *assert.CollectT) {
				metrics, err := s.Env().FakeIntake.Client().FilterMetrics(tc.metric)
				require.NoError(c, err)
				require.NotEmpty(c, metrics, "no %s received yet", tc.metric)
				assertEveryMetricHasTagKeys(c, tc.metric, metrics, tc.tagKeys)
			}, 5*time.Minute, 10*time.Second)
		})
	}
}

// TestNonCuratedMetricsStayInternal checks that enabling internal_telemetry reports the curated
// set rather than the whole registry, which is what internal_telemetry.advanced is for.
func (s *internalTelemetrySuite) TestNonCuratedMetricsStayInternal() {
	// Establish a positive signal first: once a curated metric has arrived, the pipeline has
	// flushed at least once, so a non-curated metric would be here too if it were being reported.
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.agent.aggregator.number_of_flush")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics, "no curated metric received yet, cannot conclude anything about the rest")
	}, 5*time.Minute, 10*time.Second)

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(notCuratedMetric)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), metrics, "%s is not on the curated allowlist and must not be reported", notCuratedMetric)
}

// assertGaugeReported waits for a gauge to arrive with a positive value, carrying every tag in
// requiredTags and at least one tag for each key in requiredTagKeys.
func (s *internalTelemetrySuite) assertGaugeReported(metric string, requiredTags []string, requiredTagKeys []string) {
	s.T().Helper()

	s.EventuallyWithT(func(c *assert.CollectT) {
		client := s.Env().FakeIntake.Client()

		metrics, err := client.FilterMetrics(metric,
			fakeintakeclient.WithTags[*aggregator.MetricSeries](requiredTags),
			fakeintakeclient.WithMetricValueHigherThan(0),
		)
		require.NoError(c, err)

		if !assert.NotEmpty(c, metrics, "no %s with tags %v and a value above 0 received yet", metric, requiredTags) {
			// Name the metrics that did arrive, so a rename shows up as a rename rather than as a
			// bare "empty slice" once the infrastructure is gone.
			logReportedTelemetryMetrics(c, s.T().Logf, client)
			return
		}

		assertEveryMetricHasTagKeys(c, metric, metrics, requiredTagKeys)
	}, 5*time.Minute, 10*time.Second)
}

// assertEveryMetricHasTagKeys checks that each series carries a tag for every given key. Keys are
// checked rather than whole tags where the value is environment-dependent.
func assertEveryMetricHasTagKeys(c *assert.CollectT, metric string, metrics []*aggregator.MetricSeries, tagKeys []string) {
	for _, series := range metrics {
		for _, key := range tagKeys {
			assert.True(c, hasTagKey(series.Tags, key), "%s is missing a %q tag, got %v", metric, key, series.Tags)
		}
	}
}

func hasTagKey(tags []string, key string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, key+":") {
			return true
		}
	}
	return false
}

// logReportedTelemetryMetrics lists the datadog.agent.* metrics the intake has seen, to turn a
// name mismatch into a readable failure.
func logReportedTelemetryMetrics(c *assert.CollectT, logf func(string, ...any), client *fakeintakeclient.Client) {
	names, err := client.GetMetricNames()
	if !assert.NoError(c, err) {
		return
	}

	var reported []string
	for _, name := range names {
		if strings.HasPrefix(name, "datadog.agent.") {
			reported = append(reported, name)
		}
	}
	logf("datadog.agent.* metrics received so far: %v", reported)
}
