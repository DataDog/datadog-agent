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
	return m.Mock.AssertCalled(t, "ServiceCheck", checkName, status, hostname, AssertTagsContain(t, tags), message)
}

// AssertMetric allows to assert a metric was emitted with given parameters.
// Additional tags over the ones specified don't make it fail
func (m *MockSender) AssertMetric(t *testing.T, method string, metric string, value float64, hostname string, tags []string) bool {
	return m.Mock.AssertCalled(t, method, metric, value, hostname, AssertTagsContain(t, tags))
}

// AssertTagsContain is a mock.argumentMatcher builder to be used in asserts.
// It allows to check if tags are emitted, ignoring unexpected ones and order.
func AssertTagsContain(t *testing.T, expected []string) interface{} {
	return mock.MatchedBy(func(actual []string) bool {
		for _, tag := range expected {
			if !assert.Contains(t, actual, tag) {
				return false
			}
		}
		return true
	})
}
