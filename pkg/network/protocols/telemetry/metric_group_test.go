// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricGroup(t *testing.T) {
	Clear()

	assert := assert.New(t)

	// these metrics will be namespace under foo and will have share tag:abc
	metricGroup := NewMetricGroup("foo", "tag:abc")
	metricGroup.NewMetric("m1", "tag:foo").Set(10)
	metricGroup.NewMetric("m2", "tag:bar").Set(20)

	// since we're here using the full (namespaced) name and the full tag set,
	// we should get the previously created metrics
	assert.Equal(int64(10), NewMetric("foo.m1", "tag:foo", "tag:abc").Get())
	assert.Equal(int64(20), NewMetric("foo.m2", "tag:bar", "tag:abc").Get())

	summary := metricGroup.Summary()
	expected := map[string]int64{
		"m1,tag:abc,tag:foo": int64(10),
		"m2,tag:abc,tag:bar": int64(20),
	}
	assert.Equal(expected, summary)
}

func TestMetricGroupWithoutPrefix(t *testing.T) {
	Clear()

	assert := assert.New(t)
	metricGroup := NewMetricGroup("")
	metricGroup.NewMetric("m1").Set(10)
	metricGroup.NewMetric("m2").Set(20)

	summary := metricGroup.Summary()
	expected := map[string]int64{
		"m1": int64(10),
		"m2": int64(20),
	}

	assert.Equal(expected, summary)
}
