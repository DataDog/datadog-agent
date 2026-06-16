// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package seriesv3

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
	gaugeMetric = "e2e.metric.seriesv3.gauge"

	metricsV2Endpoint = "/api/v2/series"
	metricsV3Endpoint = "/api/intake/metrics/v3/series"
)

// v3Mode describes how use_v3_api.series is configured for a given suite run.
type v3Mode int

const (
	// v3ModeDefault: no explicit config, v3 is on by default (PR #52059).
	v3ModeDefault v3Mode = iota
	// v3ModeDisabled: use_v3_api.series.enabled=false, agent must use v2.
	v3ModeDisabled
	// v3ModeDatadogOnly: use_v3_api.series.enabled=datadog_only; fakeintake is not a
	// datadoghq.com host, so the agent must fall back to v2.
	v3ModeDatadogOnly
)

type seriesV3Suite struct {
	e2e.BaseSuite[environments.Host]
	mode v3Mode
}

// sendGauge sends a single DogStatsD gauge over UDP to the local Agent.
func (s *seriesV3Suite) sendGauge(name string, value int) {
	cmd := fmt.Sprintf(`bash -c 'echo -n "%s:%d|g" > /dev/udp/127.0.0.1/8125'`, name, value)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestSeriesV3Routing is the single test method for all three suite variants.
// It sends a gauge, waits for it to appear in fakeintake, then asserts that
// exactly the expected endpoint received series payloads.
func (s *seriesV3Suite) TestSeriesV3Routing() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.sendGauge(gaugeMetric, 1)
		series, err := s.Env().FakeIntake.Client().FilterMetrics(gaugeMetric)
		assert.NoError(c, err)
		assert.NotEmpty(c, series, "metric %q not yet in fakeintake", gaugeMetric)
	}, 2*time.Minute, 5*time.Second, "timed out waiting for %q to reach fakeintake", gaugeMetric)

	routeStats, err := s.Env().FakeIntake.Client().RouteStats()
	require.NoError(s.T(), err)

	switch s.mode {
	case v3ModeDefault:
		// v3 is on by default since #52059; the v3 endpoint must receive payloads
		// and the v2 endpoint must be idle.
		assert.Greater(s.T(), routeStats[metricsV3Endpoint], 0,
			"expected series payloads on %s with default config", metricsV3Endpoint)
		assert.Zero(s.T(), routeStats[metricsV2Endpoint],
			"expected no series payloads on %s with default config", metricsV2Endpoint)

	case v3ModeDisabled:
		// use_v3_api.series.enabled=false must route all series back to v2.
		assert.Greater(s.T(), routeStats[metricsV2Endpoint], 0,
			"expected series payloads on %s when v3 disabled", metricsV2Endpoint)
		assert.Zero(s.T(), routeStats[metricsV3Endpoint],
			"expected no series payloads on %s when v3 disabled", metricsV3Endpoint)

	case v3ModeDatadogOnly:
		// use_v3_api.series.enabled=datadog_only: fakeintake is not a datadoghq.com
		// host, so the agent must fall back to v2.
		assert.Greater(s.T(), routeStats[metricsV2Endpoint], 0,
			"expected series payloads on %s when datadog_only and intake is not a DD domain", metricsV2Endpoint)
		assert.Zero(s.T(), routeStats[metricsV3Endpoint],
			"expected no series payloads on %s when datadog_only and intake is not a DD domain", metricsV3Endpoint)
	}
}

func runSeriesV3Suite(t *testing.T, mode v3Mode, stackName string, agentOptions ...agentparams.Option) {
	t.Parallel()
	e2e.Run(t, &seriesV3Suite{mode: mode},
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

// TestSeriesV3Default verifies that the agent uses the v3 series intake endpoint
// when no explicit use_v3_api configuration is provided (v3 is on by default).
func TestSeriesV3Default(t *testing.T) {
	runSeriesV3Suite(t, v3ModeDefault, "seriesv3-default")
}

// TestSeriesV3Disabled verifies that setting use_v3_api.series.enabled=false
// causes the agent to fall back to the v2 series intake endpoint.
func TestSeriesV3Disabled(t *testing.T) {
	runSeriesV3Suite(t, v3ModeDisabled, "seriesv3-disabled",
		agentparams.WithV3MetricsDisabled(),
	)
}

// TestSeriesV3DatadogOnly verifies that use_v3_api.series.enabled=datadog_only causes
// the agent to fall back to v2 when the intake URL is not a datadoghq.com domain.
// Fakeintake runs on an ephemeral EC2 hostname, so v3 must NOT be selected.
func TestSeriesV3DatadogOnly(t *testing.T) {
	runSeriesV3Suite(t, v3ModeDatadogOnly, "seriesv3-datadogonly",
		agentparams.WithAgentConfig(`
use_v3_api:
  series:
    enabled: "datadog_only"
`),
	)
}
