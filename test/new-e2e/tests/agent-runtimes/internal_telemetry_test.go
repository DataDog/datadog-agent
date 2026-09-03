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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// internalTelemetryAgentConfig turns on the curated internal telemetry set. Both settings live in
// datadog.yaml rather than in the check's own configuration, and both are runtime settings.
const internalTelemetryAgentConfig = `telemetry:
  internal:
    enabled: true
`

// notCuratedMetric is reported into the regular telemetry registry by the forwarder on every
// submission, but is deliberately absent from the check's curated allowlist. It stands in for
// "everything the curated set is supposed to leave behind".
const notCuratedMetric = "datadog.agent.transactions.input_bytes"

// histogramMetric is a Prometheus histogram the check reports as a native distribution. The Go
// runtime collector registers it in every Agent and the scheduler accumulates latencies
// continuously, so unlike most internal histograms it is reliably non-empty on an idle host, and
// its per-interval delta is reliably non-zero.
const histogramMetric = "datadog.agent.go_sched_latencies_seconds"

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
					agentparams.WithAgentConfig(internalTelemetryAgentConfig),
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

// TestCurationAndAdvancedAtRuntime covers the boundary between the curated set and the whole
// registry, and that telemetry.internal.advanced takes effect at runtime.
//
// The two halves are subtests because they must run in order: the first proves a non-curated
// metric is absent, and the second makes that same metric start arriving. The intake keeps
// everything it has received, so running them the other way round would leave the absence check
// with no way to pass.
func (s *internalTelemetrySuite) TestCurationAndAdvancedAtRuntime() {
	s.T().Run("curated set excludes the rest of the registry", func(t *testing.T) {
		// Establish a positive signal first: once a curated metric has arrived, the pipeline has
		// flushed, so a non-curated metric would be here too if it were being reported.
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.agent.aggregator.number_of_flush")
			require.NoError(c, err)
			assert.NotEmpty(c, metrics, "no curated metric received yet, cannot conclude anything about the rest")
		}, 5*time.Minute, 10*time.Second)

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(notCuratedMetric)
		require.NoError(t, err)
		assert.Empty(t, metrics, "%s is not on the curated allowlist and must not be reported", notCuratedMetric)
	})

	s.T().Run("advanced widens it without a restart", func(t *testing.T) {
		// telemetry.internal.advanced is a runtime setting and the check reads it on every run, so
		// this takes effect on the next check run with no restart and no reconfiguration.
		s.setRuntimeSetting(t, "telemetry.internal.advanced", "true")
		t.Cleanup(func() { s.setRuntimeSetting(t, "telemetry.internal.advanced", "false") })

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics(notCuratedMetric)
			require.NoError(c, err)
			assert.NotEmpty(c, metrics, "%s should be reported once advanced mode is on", notCuratedMetric)
		}, 2*time.Minute, 10*time.Second)

		// Histograms go out as native distributions rather than as metrics, so they arrive as
		// sketches. The first observation of each bucket is skipped to avoid reporting everything
		// since Agent start in one interval, so this needs a second check run to appear.
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			sketches, err := s.Env().FakeIntake.Client().FilterSketches(histogramMetric)
			require.NoError(c, err)
			assert.NotEmpty(c, sketches, "%s should be reported as a distribution", histogramMetric)
		}, 2*time.Minute, 10*time.Second)
	})
}

// setRuntimeSetting changes a runtime setting through the Agent CLI and confirms it took, which
// is also what proves the setting is registered as runtime-capable in the first place.
func (s *internalTelemetrySuite) setRuntimeSetting(t *testing.T, setting, value string) {
	t.Helper()

	s.Env().Agent.Client.Config(agentclient.WithArgs([]string{"set", setting, value}))

	got := s.Env().Agent.Client.Config(agentclient.WithArgs([]string{"get", setting}))
	require.Contains(t, got, value, "%s did not take the value %q, got: %s", setting, value, got)
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
