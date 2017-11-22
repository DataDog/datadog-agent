// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package mocksender

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// AssertServiceCheck allows to assert a ServiceCheck was exclusively emitted with given parameters.
// Additional tags over the ones specified don't make it fail
// Assert the ServiceCheck wasn't called with any other possible status
func (m *MockSender) AssertServiceCheck(t *testing.T, checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string) bool {
	okCall := m.Mock.AssertCalled(t, "ServiceCheck", checkName, status, hostname, MatchTagsContains(tags), message)
	notOkCalls := m.Mock.AssertNotCalled(t, "ServiceCheck", checkName, AnythingBut(status), hostname, MatchTagsContains(tags), mock.AnythingOfType("string"))
	return okCall && notOkCalls
}

// AssertMetric allows to assert a metric was emitted with given parameters.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetric(t *testing.T, method string, metric string, value float64, hostname string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, value, hostname, MatchTagsContains(tags))
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
	return m.Mock.AssertCalled(t, method, metric, mock.AnythingOfType("float64"), mock.AnythingOfType("string"), AssertTagsNotContains(tags))
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

// AssertTagsNotContains is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if tags are NOT emitted.
func AssertTagsNotContains(expected []string) interface{} {
	return mock.MatchedBy(func(actual []string) bool {
		return !expectedInActual(expected, actual)
	})
}

// AssertFloatInRange is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if a metric value is in a given range instead of matching exactly.
func AssertFloatInRange(min float64, max float64) interface{} {
	return mock.MatchedBy(func(actual float64) bool {
		return actual >= min && actual <= max
	})
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
