// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricSampleCopy(t *testing.T) {
	src := &MetricSample{}
	src.Host = "foo"
	src.Mtype = HistogramType
	src.Name = "metric.name"
	src.RawValue = "0.1"
	src.SampleRate = 1
	src.Tags = []string{"a:b", "c:d"}
	src.Timestamp = 1234
	src.Value = 0.1
	dst := src.Copy()

	assert.False(t, src == dst)
	assert.True(t, reflect.DeepEqual(&src, &dst))
}

func TestMetricSampleGetName(t *testing.T) {
	sample := &MetricSample{Name: "test.metric.name"}
	assert.Equal(t, "test.metric.name", sample.GetName())
}

func TestMetricSampleGetHost(t *testing.T) {
	sample := &MetricSample{Host: "test-hostname"}
	assert.Equal(t, "test-hostname", sample.GetHost())
}

func TestMetricSampleGetMetricType(t *testing.T) {
	tests := []struct {
		name       string
		metricType MetricType
	}{
		{"gauge", GaugeType},
		{"rate", RateType},
		{"count", CountType},
		{"histogram", HistogramType},
		{"distribution", DistributionType},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sample := &MetricSample{Mtype: tc.metricType}
			assert.Equal(t, tc.metricType, sample.GetMetricType())
		})
	}
}

func TestMetricSampleIsNoIndex(t *testing.T) {
	t.Run("no index true", func(t *testing.T) {
		sample := &MetricSample{NoIndex: true}
		assert.True(t, sample.IsNoIndex())
	})

	t.Run("no index false", func(t *testing.T) {
		sample := &MetricSample{NoIndex: false}
		assert.False(t, sample.IsNoIndex())
	})
}

func TestMetricSampleGetSource(t *testing.T) {
	sample := &MetricSample{Source: MetricSourceDogstatsd}
	assert.Equal(t, MetricSourceDogstatsd, sample.GetSource())
}

func TestMetricTypeString(t *testing.T) {
	tests := []struct {
		metricType MetricType
		expected   string
	}{
		{GaugeType, "Gauge"},
		{RateType, "Rate"},
		{CountType, "Count"},
		{MonotonicCountType, "MonotonicCount"},
		{CounterType, "Counter"},
		{HistogramType, "Histogram"},
		{HistorateType, "Historate"},
		{SetType, "Set"},
		{DistributionType, "Distribution"},
		{GaugeWithTimestampType, "GaugeWithTimestamp"},
		{CountWithTimestampType, "CountWithTimestamp"},
		{MetricType(999), ""},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.metricType.String())
		})
	}
}

func TestDistributionMetricTypes(t *testing.T) {
	_, ok := DistributionMetricTypes[DistributionType]
	assert.True(t, ok, "DistributionType should be in DistributionMetricTypes")

	_, ok = DistributionMetricTypes[GaugeType]
	assert.False(t, ok, "GaugeType should not be in DistributionMetricTypes")
}
