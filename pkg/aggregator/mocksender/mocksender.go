// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package mocksender

import (
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// NewMockSender initiates the aggregator and returns a
// functional mocked Sender for testing
func NewMockSender(id check.ID) *MockSender {
	mockSender := new(MockSender)
	// The MockSender will be injected in the corecheck via the aggregator
	aggregator.InitAggregatorWithFlushInterval(nil, "", "", 1*time.Hour)
	SetSender(mockSender, id)

	return mockSender
}

// SetSender sets passed sender with the passed ID.
func SetSender(sender *MockSender, id check.ID) {
	aggregator.SetSender(sender, id) //nolint:errcheck
}

//MockSender allows mocking of the checks sender for unit testing
type MockSender struct {
	mock.Mock
}

// SetupAcceptAll sets mock expectations to accept any call in the Sender interface
func (m *MockSender) SetupAcceptAll() {
	metricCalls := []string{"Rate", "Count", "MonotonicCount", "Counter", "Histogram", "Historate", "Gauge"}
	for _, call := range metricCalls {
		m.On(call,
			mock.AnythingOfType("string"),   // Metric
			mock.AnythingOfType("float64"),  // Value
			mock.AnythingOfType("string"),   // Hostname
			mock.AnythingOfType("[]string"), // Tags
		).Return()
	}
	m.On("ServiceCheck",
		mock.AnythingOfType("string"),                     // checkName (e.g: docker.exit)
		mock.AnythingOfType("metrics.ServiceCheckStatus"), // (e.g: metrics.ServiceCheckOK)
		mock.AnythingOfType("string"),                     // Hostname
		mock.AnythingOfType("[]string"),                   // Tags
		mock.AnythingOfType("string"),                     // message
	).Return()
	m.On("Event", mock.AnythingOfType("metrics.Event")).Return()
	m.On("HistogramBucket",
		mock.AnythingOfType("string"),   // metric name
		mock.AnythingOfType("int64"),    // value
		mock.AnythingOfType("float64"),  // lower bound
		mock.AnythingOfType("float64"),  // upper bound
		mock.AnythingOfType("bool"),     // monotonic
		mock.AnythingOfType("string"),   // hostname
		mock.AnythingOfType("[]string"), // tags
	).Return()
	m.On("GetMetricStats", mock.AnythingOfType("map[string]int64")).Return()
	m.On("DisableDefaultHostname", mock.AnythingOfType("bool")).Return()
	m.On("SetCheckCustomTags", mock.AnythingOfType("[]string")).Return()
	m.On("SetCheckService", mock.AnythingOfType("string")).Return()
	m.On("FinalizeCheckServiceTag").Return()
	m.On("Commit").Return()
}

// ResetCalls makes the mock forget previous calls
func (m *MockSender) ResetCalls() {
	m.Mock.Calls = m.Mock.Calls[0:0]
}
