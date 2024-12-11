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

	sm := mocksender.CreateDefaultDemultiplexer()

	c := &checkImpl{CheckBase: corechecks.NewCheckBase(CheckName)}
	c.Configure(sm, integration.FakeConfigHash, nil, nil, "test")

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
