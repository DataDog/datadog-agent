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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-pipelines/common"
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

	metricsV2Endpoint = "/api/v2/series"
	metricsV3Endpoint = "/api/intake/metrics/v3/series"
)

type dogstatsdUnitSuite struct {
	e2e.BaseSuite[environments.Host]

	adpEnabled bool
	v3Enabled  bool
}

func testDogstatsdMetricUnit(t *testing.T, adpEnabled, v3Enabled bool) {
	t.Parallel()

	agentConfig := `
histogram_aggregates:
  - max
  - avg
  - count
histogram_percentiles:
  - "0.95"
`
	if v3Enabled {
		// The test fakeintake URL is not a Datadog intake URL, so explicitly opt in
		// to V3 instead of relying on the default datadog_only mode.
		agentConfig += `
use_v3_api:
  series:
    enabled: "true"
`
	}

	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(agentConfig),
	}
	if adpEnabled {
		agentOptions = append(agentOptions, common.WithADPEnabled())
	}
	if !v3Enabled {
		// Force V2 explicitly to exercise the V2 wire format.
		agentOptions = append(agentOptions, agentparams.WithV3MetricsDisabled())
	}

	stackName := "dogstatsdmetricunit"
	if adpEnabled {
		stackName += "-adp"
	}
	if v3Enabled {
		stackName += "-v3"
	}

	e2e.Run(t, &dogstatsdUnitSuite{adpEnabled: adpEnabled, v3Enabled: v3Enabled},
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

// TestDogstatsdMetricUnit runs the DogStatsD unit e2e test on Linux with the V2 metrics intake API.
func TestDogstatsdMetricUnit(t *testing.T) {
	testDogstatsdMetricUnit(t, false, false)
}

// TestDogstatsdMetricUnitADP runs the DogStatsD unit e2e test with ADP serving DogStatsD traffic.
func TestDogstatsdMetricUnitADP(t *testing.T) {
	testDogstatsdMetricUnit(t, true, false)
}

// TestDogstatsdMetricUnitV3 runs the DogStatsD unit e2e test with the V3 metrics intake API enabled.
// It verifies that the same unit semantics hold over the V3 wire format, and that payloads are
// routed to /api/intake/metrics/v3/series rather than /api/v2/series.
func TestDogstatsdMetricUnitV3(t *testing.T) {
	testDogstatsdMetricUnit(t, false, true)
}

// sendMetric sends a single DogStatsD metric over UDP to the local Agent.
func (s *dogstatsdUnitSuite) sendMetric(name string, value float32, metricType string) {
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s:%f|%s" > /dev/udp/127.0.0.1/8125'`, name, value, metricType)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestDogstatsdUnitOnlyOnTimingMetrics sends a counter, a histogram, and a timing metric in
// parallel and verifies that only the timing metric carries a unit. The test runs for both V2
// and V3 intake protocols; in both cases it asserts that payloads were routed
// exclusively to the expected endpoint.
func (s *dogstatsdUnitSuite) TestDogstatsdUnitOnlyOnTimingMetrics() {
	if s.adpEnabled {
		common.AssertADPRunning(s.T(), s.Env().RemoteHost)
	}

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

	// Phase 3: verify routing. Each mode must use exactly its intended endpoint and
	// send nothing to the other.
	routeStats, err := s.Env().FakeIntake.Client().RouteStats()
	require.NoError(s.T(), err)
	if s.v3Enabled {
		assert.Greater(s.T(), routeStats[metricsV3Endpoint], 0,
			"expected payloads on %s when V3 is enabled", metricsV3Endpoint)
	} else {
		assert.Greater(s.T(), routeStats[metricsV2Endpoint], 0,
			"expected payloads on %s when V3 is not enabled", metricsV2Endpoint)
		assert.Zero(s.T(), routeStats[metricsV3Endpoint],
			"expected no payloads on %s when V3 is not enabled", metricsV3Endpoint)
	}
}
