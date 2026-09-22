// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

func TestCheck(t *testing.T) {
	reg := prometheus.NewRegistry()

	func() {
		gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Subsystem: "test", Name: "_gauge"}, []string{"foo"})
		gauge.WithLabelValues("bar").Set(1.0)
		gauge.WithLabelValues("baz").Set(2.0)
		reg.MustRegister(gauge)

		count := prometheus.NewCounter(prometheus.CounterOpts{Subsystem: "test", Name: "_counter"})
		count.Add(4.0)
		reg.MustRegister(count)
	}()

	sm := mocksender.CreateDefaultDemultiplexer(t)

	c := &checkImpl{CheckBase: corechecks.NewCheckBase(CheckName)}
	c.Configure(sm, integration.FakeConfigHash, nil, nil, "test", "provider")

	s := mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
	s.On("Gauge", "datadog.agent.test.gauge", 1.0, "", []string{"foo:bar"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.test.gauge", 2.0, "", []string{"foo:baz"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return().Times(1)
	s.On("Commit").Return().Times(1)

	mfs, err := reg.Gather()
	require.Nil(t, err)

	c.handleMetricFamilies(mfs, s)
	s.AssertExpectations(t)
}

// TestRunSendsOnlyDefaultMetrics guards the premise of the single telemetry registry: the check sees
// all of the Agent's internal telemetry, and must submit only what defaultMetrics lists.
func TestRunSendsOnlyDefaultMetrics(t *testing.T) {
	tel := telemetrymock.New(t)

	pointSent := tel.NewGauge("points", "sent", []string{"domain"}, "Number of points successfully sent to the intake")
	pointSent.Set(7, "https://api.datadoghq.com")

	pointDropped := tel.NewGauge("points", "dropped", []string{"domain"}, "Number of points dropped before reaching the intake")
	pointDropped.Set(2, "https://api.datadoghq.com")

	haAgentRuns := tel.NewCounter("checks", "ha_agent_integration_runs", []string{"integration", "config_id"}, "Tracks number of HA integrations runs.")
	haAgentRuns.Add(3, "snmp", "abc")

	// Not in defaultMetrics: must stay on the /telemetry endpoint only.
	notDefault := tel.NewGauge("checks", "execution_time", []string{"check_name"}, "Check execution time")
	notDefault.Set(123, "cpu")

	sm := mocksender.CreateDefaultDemultiplexer(t)

	c := &checkImpl{CheckBase: corechecks.NewCheckBase(CheckName), telemetry: tel, metrics: defaultMetrics}
	require.NoError(t, c.Configure(sm, integration.FakeConfigHash, nil, nil, "test", "provider"))

	s := mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
	s.On("SetNoIndex", true).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.sent", 7.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.dropped", 2.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.ha_agent.integration_runs", 3.0, "",
		[]string{"config_id:abc", "integration:snmp"}, true).Return().Times(1)
	s.On("Commit").Return().Times(1)

	require.NoError(t, c.Run())

	s.AssertExpectations(t)
	s.AssertNotCalled(t, "Gauge", "datadog.agent.checks.execution_time", 123.0, "", []string{"check_name:cpu"})
}

// TestRunRemapsAllowlistedMetrics covers the allowlist's ability to send a metric under a name other than the
// one it is registered with in the codebase.
func TestRunRemapsAllowlistedMetrics(t *testing.T) {
	tel := telemetrymock.New(t)

	remapped := tel.NewGauge("bad_namespace", "calls", []string{}, "Number of calls to feature A")
	remapped.Set(7)

	asIs := tel.NewCounter("feature_b", "calls", []string{}, "Number of calls to feature B")
	asIs.Add(3)

	sm := mocksender.CreateDefaultDemultiplexer(t)

	c := &checkImpl{
		CheckBase: corechecks.NewCheckBase(CheckName),
		telemetry: tel,
		metrics: []allowlistedMetric{
			{name: "bad_namespace__request_count", sendAs: "feature_a__request_count"},
			{name: "feature_b__call_count"},
		},
	}
	require.NoError(t, c.Configure(sm, integration.FakeConfigHash, nil, nil, "test", "provider"))

	s := mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
	s.On("SetNoIndex", true).Return().Times(1)
	s.On("Gauge", "datadog.agent.feature_a.calls", 7.0, "", []string{}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.feature_b.calls", 3.0, "", []string{}, true).Return().Times(1)
	s.On("Commit").Return().Times(1)

	require.NoError(t, c.Run())

	s.AssertExpectations(t)
	s.AssertNotCalled(t, "Gauge", "datadog.agent.bad_namespace.calls", 7.0, "", []string{})
}
