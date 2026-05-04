// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsdunit

import (
	"fmt"
	"sync"
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
	// countMetric is a DogStatsD counter (|c|). Flushed as a rate; no unit expected.
	countMetric = "e2e.metric.unit.count"

	// histogramMetric is a DogStatsD histogram (|h|). Flushed as multiple aggregated
	// series (e.g. .max, .avg); no unit expected.
	histogramMetric    = "e2e.metric.unit.histogram"
	histogramMaxSuffix = ".max"

	// timingMetric is a DogStatsD timing (|ms|). Processed identically to a histogram
	// but the Agent attaches unit "millisecond" to every flushed serie.
	timingMetric    = "e2e.metric.unit.timing"
	timingMaxSuffix = ".max"

	// expectedTimingUnit is the Datadog API unit name for millisecond timing metrics.
	expectedTimingUnit = "millisecond"
)

type dogstatsdUnitSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestDogstatsdMetricUnit runs the DogStatsD unit e2e test on Linux.
func TestDogstatsdMetricUnit(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dogstatsdUnitSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(`
histogram_aggregates:
  - max
  - avg
  - count
histogram_percentiles:
  - "0.95"
`),
				),
			),
		),
	))
}

// sendMetric sends a single DogStatsD metric over UDP to the local Agent.
func (s *dogstatsdUnitSuite) sendMetric(name string, value float32, metricType string) {
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s:%f|%s" > /dev/udp/127.0.0.1/8125'`, name, value, metricType)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestDogstatsdUnitOnlyOnTimingMetrics sends a counter, a histogram, and a timing
// metric in parallel and verifies that only the timing metric carries a unit.
func (s *dogstatsdUnitSuite) TestDogstatsdUnitOnlyOnTimingMetrics() {
	// Phase 1: keep sending all three metrics until at least one flushed serie for
	// each has reached fakeintake.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); s.sendMetric(countMetric, 1, "c") }()
		go func() { defer wg.Done(); s.sendMetric(histogramMetric, 100, "h") }()
		go func() {
			defer wg.Done()
			s.sendMetric(timingMetric, 100, "ms")
			s.sendMetric(timingMetric, 0.2, "ms")
			s.sendMetric(timingMetric, 3000, "ms")
		}()
		wg.Wait()

		_, err := s.Env().FakeIntake.Client().FilterMetrics(countMetric)
		assert.NoError(c, err)

		histoSeries, err := s.Env().FakeIntake.Client().FilterMetrics(histogramMetric + histogramMaxSuffix)
		assert.NoError(c, err)
		assert.NotEmpty(c, histoSeries, "histogram .max serie must reach fakeintake")

		timingSeries, err := s.Env().FakeIntake.Client().FilterMetrics(timingMetric + timingMaxSuffix)
		assert.NoError(c, err)
		assert.NotEmpty(c, timingSeries, "timing .max serie must reach fakeintake")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for all metric types to reach fakeintake")

	// Phase 2: assert unit fields on each metric type.

	// Counter: unit must be empty.
	countSeries, err := s.Env().FakeIntake.Client().FilterMetrics(countMetric)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), countSeries)
	for _, m := range countSeries {
		assert.Empty(s.T(), m.Unit,
			"counter metric %q must not carry a unit, got %q", m.Metric, m.Unit)
		fmt.Printf("metric %q does not carry a unit\n", m.Metric)
	}

	// Histogram .max: unit must be empty.
	histoSeries, err := s.Env().FakeIntake.Client().FilterMetrics(histogramMetric + histogramMaxSuffix)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), histoSeries)
	for _, m := range histoSeries {
		assert.Empty(s.T(), m.Unit,
			"histogram metric %q must not carry a unit, got %q", m.Metric, m.Unit)
		fmt.Printf("metric %q does not carry a unit\n", m.Metric)
	}

	// Timing .max: unit must be "millisecond".
	timingSeries, err := s.Env().FakeIntake.Client().FilterMetrics(timingMetric + timingMaxSuffix)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), timingSeries)
	for _, m := range timingSeries {
		assert.Equal(s.T(), expectedTimingUnit, m.Unit,
			"timing metric %q must carry unit %q, got %q", m.Metric, expectedTimingUnit, m.Unit)
		fmt.Printf("metric %q carries unit %q\n", m.Metric, m.Unit)
	}
}
