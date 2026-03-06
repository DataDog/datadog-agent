// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

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

func TestBuildName(t *testing.T) {
	c := &checkImpl{}
	assert.Equal(t, "datadog.agent.test.metric", c.buildName("test__metric"))
	assert.Equal(t, "datadog.agent.simple", c.buildName("simple"))
	assert.Equal(t, "datadog.agent.a.b.c", c.buildName("a__b__c"))
}

func TestBuildTags(t *testing.T) {
	c := &checkImpl{}

	t.Run("normal labels", func(t *testing.T) {
		name := "env"
		value := "prod"
		labels := []*dto.LabelPair{
			{Name: &name, Value: &value},
		}
		tags := c.buildTags(labels)
		assert.Equal(t, []string{"env:prod"}, tags)
	})

	t.Run("nil label name is skipped", func(t *testing.T) {
		value := "prod"
		labels := []*dto.LabelPair{
			{Name: nil, Value: &value},
		}
		tags := c.buildTags(labels)
		assert.Empty(t, tags)
	})

	t.Run("nil label value uses name only", func(t *testing.T) {
		name := "flag"
		labels := []*dto.LabelPair{
			{Name: &name, Value: nil},
		}
		tags := c.buildTags(labels)
		assert.Equal(t, []string{"flag"}, tags)
	})

	t.Run("empty labels", func(t *testing.T) {
		tags := c.buildTags(nil)
		assert.Empty(t, tags)
	})
}

func TestHandleMetricFamiliesEdgeCases(t *testing.T) {
	sm := mocksender.CreateDefaultDemultiplexer()
	c := &checkImpl{CheckBase: corechecks.NewCheckBase(CheckName)}
	c.Configure(sm, integration.FakeConfigHash, nil, nil, "test")

	s := mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
	s.On("Commit").Return().Times(1)

	// Metric family with nil name should be skipped
	// Metric family with nil type should be skipped
	// Metric family with no metrics should be skipped
	// Nil metric should be skipped
	// Gauge with nil gauge field should be skipped
	// Counter with nil counter field should be skipped
	gaugeType := dto.MetricType_GAUGE
	counterType := dto.MetricType_COUNTER
	unknownType := dto.MetricType_SUMMARY
	name := "test__metric"
	mfs := []*dto.MetricFamily{
		{Name: nil, Type: &gaugeType, Metric: []*dto.Metric{{}}},
		{Name: &name, Type: nil, Metric: []*dto.Metric{{}}},
		{Name: &name, Type: &gaugeType, Metric: []*dto.Metric{}},
		{Name: &name, Type: &gaugeType, Metric: []*dto.Metric{nil}},
		{Name: &name, Type: &gaugeType, Metric: []*dto.Metric{{Gauge: nil}}},
		{Name: &name, Type: &counterType, Metric: []*dto.Metric{{Counter: nil}}},
		{Name: &name, Type: &unknownType, Metric: []*dto.Metric{{Gauge: &dto.Gauge{Value: proto.Float64(1.0)}}}},
	}

	// None of the above should trigger any Gauge/MonotonicCount calls
	c.handleMetricFamilies(mfs, s)
	s.AssertExpectations(t)
}
