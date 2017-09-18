// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package py

import (
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func init() {
	// The MockSender requires an aggregator
	aggregator.InitAggregator(nil, "")
}

//MockSender allows mocking of the checks sender
type MockSender struct {
	mock.Mock
}

//Rate adds a rate type to the mock calls.
func (m *MockSender) Rate(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//Count adds a count type to the mock calls.
func (m *MockSender) Count(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//MonotonicCount adds a monotonic count type to the mock calls.
func (m *MockSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//Histogram adds a histogram type to the mock calls.
func (m *MockSender) Histogram(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//Historate adds a historate type to the mock calls.
func (m *MockSender) Historate(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//Gauge adds a gauge type to the mock calls.
func (m *MockSender) Gauge(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//ServiceCheck enables the service check mock call.
func (m *MockSender) ServiceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string) {
	m.Called(checkName, status, hostname, tags, message)
}

//Event enables the event mock call.
func (m *MockSender) Event(e metrics.Event) {
	m.Called(e)
}

//Commit enables the commit mock call.
func (m *MockSender) Commit() {
	m.Called()
}

func (m *MockSender) GetMetricStats() map[string]int64 {
	m.Called()
	return map[string]int64{}
}
