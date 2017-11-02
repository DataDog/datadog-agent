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

// AssertServiceCheck allows to assert a ServiceCheck was emitted with given parameters.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertServiceCheck(t *testing.T, checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string) bool {
	return m.Mock.AssertCalled(t, "ServiceCheck", checkName, status, hostname, AssertTagsContains(t, tags), message)
}

// AssertMetric allows to assert a metric was emitted with given parameters.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetric(t *testing.T, method string, metric string, value float64, hostname string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, value, hostname, AssertTagsContains(t, tags))
}

// AssertMetricInRange allows to assert a metric was emitted with given parameters, with a value in a given range.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetricInRange(t *testing.T, method string, metric string, min float64, max float64, hostname string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, AssertFloatInRange(t, min, max), hostname, AssertTagsContains(t, tags))
}

// AssertMetricTaggedWith allows to assert a metric was emitted with given tags, value and hostname not tested.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetricTaggedWith(t *testing.T, method string, metric string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, mock.AnythingOfType("float64"), mock.AnythingOfType("string"), AssertTagsContains(t, tags))
}

// AssertMetricNotTaggedWith allows to assert tags were never emitted for a metric.
func (m *MockSender) AssertMetricNotTaggedWith(t *testing.T, method string, metric string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, mock.AnythingOfType("float64"), mock.AnythingOfType("string"), AssertTagsNotContains(t, tags))
}

// AssertTagsContains is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if tags are emitted, ignoring unexpected ones and order.
func AssertTagsContains(t *testing.T, expected []string) interface{} {
	return mock.MatchedBy(func(actual []string) bool {
		for _, tag := range expected {
			if !assert.Contains(t, actual, tag) {
				return false
			}
		}
		return true
	})
}

// AssertTagsNotContains is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if tags are NOT emitted.
func AssertTagsNotContains(t *testing.T, expected []string) interface{} {
	return mock.MatchedBy(func(actual []string) bool {
		for _, tag := range expected {
			if !assert.NotContains(t, actual, tag) {
				return false
			}
		}
		return true
	})
}

// AssertFloatInRange is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if a metric value is in a given range instead of matching exactly.
func AssertFloatInRange(t *testing.T, min float64, max float64) interface{} {
	return mock.MatchedBy(func(actual float64) bool {
		return actual >= min && actual <= max
	})
}
