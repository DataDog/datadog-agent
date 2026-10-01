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

func TestUnknownMetricType(t *testing.T) {
	assert.Equal(t, "Unknown", UnknownType.String())
	assert.Equal(t, MetricType(0), GaugeType)
}

func TestParseMetricType(t *testing.T) {
	for metricType := GaugeType; metricType < NumMetricTypes; metricType++ {
		assert.Equal(t, metricType, ParseMetricType(metricType.String()))
	}
	for _, name := range []string{"", "Unknown", "future-type"} {
		assert.Equal(t, UnknownType, ParseMetricType(name))
	}
}
