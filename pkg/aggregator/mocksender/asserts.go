// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

//nolint:revive // TODO(AML) Fix revive linter
package mocksender

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

// AssertServiceCheck allows to assert a ServiceCheck was exclusively emitted with given parameters.
// Additional tags over the ones specified don't make it fail
// Assert the ServiceCheck wasn't called with any other possible status
func (m *MockSender) AssertServiceCheck(t *testing.T, checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) bool {
	okCall := m.Mock.AssertCalled(t, "ServiceCheck", checkName, status, hostname, MatchTagsContains(tags), message)
	notOkCalls := m.Mock.AssertNotCalled(t, "ServiceCheck", checkName, AnythingBut(status), hostname, MatchTagsContains(tags), mock.AnythingOfType("string"))
	return okCall && notOkCalls
}

// AssertMetric allows to assert a metric was emitted with given parameters.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetric(t *testing.T, method string, metric string, value float64, hostname string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, value, hostname, MatchTagsContains(tags))
}

// AssertMetric that the metric is emitted only once
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetricOnce(t *testing.T, method string, metric string, value float64, hostname string, tags []string) bool {
	okCall := m.Mock.AssertCalled(t, method, metric, value, hostname, MatchTagsContains(tags))
	notOkCalls := m.Mock.AssertNotCalled(t, method, metric, AnythingBut(value), hostname, MatchTagsContains(tags))
	return okCall && notOkCalls
}

// AssertMonotonicCount allows to assert a monotonic count was emitted with given parameters.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMonotonicCount(t *testing.T, method string, metric string, value float64, hostname string, tags []string, flushFirstValue bool) bool {
	return m.Mock.AssertCalled(t, method, metric, value, hostname, MatchTagsContains(tags), flushFirstValue)
}

// AssertHistogramBucket allows to assert a histogram bucket was emitted with given parameters.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertHistogramBucket(t *testing.T, method string, metric string, value int64, lowerBound float64, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) bool {
	return m.Mock.AssertCalled(t, method, metric, value, lowerBound, upperBound, monotonic, hostname, tags, flushFirstValue)
}

// AssertMetricInRange allows to assert a metric was emitted with given parameters, with a value in a given range.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetricInRange(t *testing.T, method string, metric string, min float64, max float64, hostname string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, AssertFloatInRange(min, max), hostname, MatchTagsContains(tags))
}

// AssertMetricTaggedWith allows to assert a metric was emitted with given tags, value and hostname not tested.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetricTaggedWith(t *testing.T, method string, metric string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, mock.AnythingOfType("float64"), mock.AnythingOfType("string"), MatchTagsContains(tags))
}

// AssertMetricNotTaggedWith allows to assert tags were never emitted for a metric.
func (m *MockSender) AssertMetricNotTaggedWith(t *testing.T, method string, metric string, tags []string) bool {
	return m.Mock.AssertNotCalled(t, method, metric, mock.AnythingOfType("float64"), mock.AnythingOfType("string"), MatchTagsContains(tags))
}

// AssertMetricWithTimestamp allows to assert a metric was emitted with given parameters and timestamp.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetricWithTimestamp(t *testing.T, method string, metric string, value float64, hostname string, tags []string, ts float64) bool {
	return m.Mock.AssertCalled(t, method, metric, value, hostname, MatchTagsContains(tags), ts)
}

// AssertEvent assert the expectedEvent was emitted with the following values:
// AggregationKey, Priority, SourceTypeName, EventType, Host and a Ts range weighted with the parameter allowedDelta
func (m *MockSender) AssertEvent(t *testing.T, expectedEvent event.Event, allowedDelta time.Duration) bool {
	return m.Mock.AssertCalled(t, "Event", MatchEventLike(expectedEvent, allowedDelta))
}

// AssertEventPlatformEvent assert the expected event was emitted with the following values
func (m *MockSender) AssertEventPlatformEvent(t *testing.T, expectedRawEvent []byte, expectedEventType string) bool {
	return m.Mock.AssertCalled(t, "EventPlatformEvent", expectedRawEvent, expectedEventType)
}

// AssertEventMissing assert the expectedEvent was never emitted with the following values:
// AggregationKey, Priority, SourceTypeName, EventType, Host and a Ts range weighted with the parameter allowedDelta
func (m *MockSender) AssertEventMissing(t *testing.T, expectedEvent event.Event, allowedDelta time.Duration) bool {
	return m.Mock.AssertNotCalled(t, "Event", MatchEventLike(expectedEvent, allowedDelta))
}

// AnythingBut match everything except the argument
// It builds a mock.argumentMatcher
func AnythingBut(expected interface{}) interface{} {
	return mock.MatchedBy(func(actual interface{}) bool {
		return !assert.ObjectsAreEqualValues(expected, actual)
	})
}

// MatchTagsContains is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if tags are emitted, ignoring unexpected ones and order.
func MatchTagsContains(expected []string) interface{} {
	return mock.MatchedBy(func(actual []string) bool {
		return expectedInActual(expected, actual)
	})
}

// IsGreaterOrEqual is a mock.argumentMatcher builder to be used in asserts.
// actual have to be greater or equal expectedMin
func IsGreaterOrEqual(expectedMin float64) interface{} {
	return mock.MatchedBy(func(actual float64) bool {
		return expectedMin <= actual
	})
}

// AssertFloatInRange is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if a metric value is in a given range instead of matching exactly.
func AssertFloatInRange(min float64, max float64) interface{} {
	return mock.MatchedBy(func(actual float64) bool {
		return actual >= min && actual <= max
	})
}

// MatchEventLike is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if an event is Equal on the following Event elements:
// AggregationKey, Priority, SourceTypeName, EventType, Host and Tag list
// Also do a timestamp comparison with a tolerance defined by allowedDelta
func MatchEventLike(expected event.Event, allowedDelta time.Duration) interface{} {
	return mock.MatchedBy(func(actual event.Event) bool {
		expectedTime := time.Unix(expected.Ts, 0)
		actualTime := time.Unix(actual.Ts, 0)
		dt := expectedTime.Sub(actualTime)
		if dt < -allowedDelta || dt > allowedDelta {
			return false
		}
		return eventLike(expected, actual)
	})
}

// Compare an Event on specifics values:
// AggregationKey, Priority, SourceTypeName, EventType, Host, and tag list
func eventLike(expectedEvent, actualEvent event.Event) bool {
	return (assert.ObjectsAreEqualValues(expectedEvent.AggregationKey, actualEvent.AggregationKey) &&
		assert.ObjectsAreEqualValues(expectedEvent.Priority, actualEvent.Priority) &&
		assert.ObjectsAreEqualValues(expectedEvent.SourceTypeName, actualEvent.SourceTypeName) &&
		assert.ObjectsAreEqualValues(expectedEvent.EventType, actualEvent.EventType) &&
		assert.ObjectsAreEqualValues(expectedEvent.Host, actualEvent.Host) &&
		expectedInActual(expectedEvent.Tags, actualEvent.Tags))
}

// Return a bool value if all the elements of expected are inside the actual array
func expectedInActual(expected, actual []string) bool {
	expectedCount := 0
	for _, e := range expected {
		for _, a := range actual {
			if e == a {
				expectedCount++
				break
			}
		}
	}
	return len(expected) == expectedCount
}
