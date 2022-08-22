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
