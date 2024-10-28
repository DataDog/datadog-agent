// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricGroup(t *testing.T) {
	Clear()

	assert := assert.New(t)

	// these metrics will be namespace under foo and will have share tag:abc
	metricGroup := NewMetricGroup("foo", "tag:abc")
	metricGroup.NewGauge("m1", "tag:foo").Set(10)
	metricGroup.NewGauge("m2", "tag:bar").Set(20)

	// since we're here using the full (namespaced) name and the full tag set,
	// we should get the previously created metrics
	assert.Equal(int64(10), NewGauge("foo.m1", "tag:foo", "tag:abc").Get())
}

func TestMetricGroupSummary(t *testing.T) {
	Clear()

	metricGroup := NewMetricGroup("foo", "common_tag:whatever")
	metricGroup.NewCounter("m1", "tag:foo").Add(10)
	metricGroup.NewCounter("m2", "tag:bar").Add(20)

	assert.Regexp(t,
		regexp.MustCompile(`m1\[tag:foo\]=10\([0-9.]+/s\) m2\[tag:bar\]=20\([0-9.]+/s\)`),
		metricGroup.Summary(),
	)
}

func TestGaugeSummaryRegression(t *testing.T) {
	Clear()

	metricGroup := NewMetricGroup("foo")
	gauge := metricGroup.NewGauge("cache_size")
	gauge.Set(50)

	assert.Equal(t, "cache_size=50", metricGroup.Summary())

	// Assert a second time that the value hasn't changed
	// (for gauge types we don't want to print the delta, just the actual number)
	assert.Equal(t, "cache_size=50", metricGroup.Summary())
}
