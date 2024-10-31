// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mocksender

import (
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logimpl "github.com/DataDog/datadog-agent/comp/core/log/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// NewMockSender initiates the aggregator and returns a
// functional mocked Sender for testing
func NewMockSender(id checkid.ID) *MockSender {
	return NewMockSenderWithSenderManager(id, CreateDefaultDemultiplexer())
}

// CreateDefaultDemultiplexer creates a default demultiplexer for testing
func CreateDefaultDemultiplexer() *aggregator.AgentDemultiplexer {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 1 * time.Hour
	opts.DontStartForwarders = true
	log := logimpl.NewTemporaryLoggerWithoutInit()
	sharedForwarder := defaultforwarder.NewDefaultForwarder(pkgconfigsetup.Datadog(), log, defaultforwarder.NewOptions(pkgconfigsetup.Datadog(), log, nil))
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	eventPlatformForwarder := optional.NewOptionPtr[eventplatform.Forwarder](eventplatformimpl.NewNoopEventPlatformForwarder(hostnameimpl.NewHostnameService()))
	taggerComponent := nooptagger.NewTaggerClient()
	return aggregator.InitAndStartAgentDemultiplexer(log, sharedForwarder, &orchestratorForwarder, opts, eventPlatformForwarder, compressionimpl.NewMockCompressor(), taggerComponent, "")

}

// NewMockSenderWithSenderManager returns a functional mocked Sender for testing
func NewMockSenderWithSenderManager(id checkid.ID, senderManager sender.SenderManager) *MockSender {
	mockSender := new(MockSender)

	mockSender.senderManager = senderManager
	SetSender(mockSender, id)

	return mockSender
}

// SetSender sets passed sender with the passed ID.
func SetSender(sender *MockSender, id checkid.ID) {
	sender.senderManager.SetSender(sender, id) //nolint:errcheck
}

// MockSender allows mocking of the checks sender for unit testing
type MockSender struct {
	mock.Mock
	senderManager sender.SenderManager
}

// GetSenderManager returns the instance of sender.SenderManager
func (m *MockSender) GetSenderManager() sender.SenderManager {
	return m.senderManager
}

// SetupAcceptAll sets mock expectations to accept any call in the Sender interface
func (m *MockSender) SetupAcceptAll() {
	metricCalls := []string{"Rate", "Count", "MonotonicCount", "Counter", "Histogram", "Historate", "Gauge", "Distribution"}
	for _, call := range metricCalls {
		m.On(call,
			mock.AnythingOfType("string"),   // Metric
			mock.AnythingOfType("float64"),  // Value
			mock.AnythingOfType("string"),   // Hostname
			mock.AnythingOfType("[]string"), // Tags
		).Return()
	}
	m.On("MonotonicCountWithFlushFirstValue",
		mock.AnythingOfType("string"),   // Metric
		mock.AnythingOfType("float64"),  // Value
		mock.AnythingOfType("string"),   // Hostname
		mock.AnythingOfType("[]string"), // Tags
		mock.AnythingOfType("bool"),     // FlushFirstValue
	).Return()
	metricWithTimestampCalls := []string{"GaugeWithTimestamp", "CountWithTimestamp"}
	for _, call := range metricWithTimestampCalls {
		m.On(call,
			mock.AnythingOfType("string"),   // Metric
			mock.AnythingOfType("float64"),  // Value
			mock.AnythingOfType("string"),   // Hostname
			mock.AnythingOfType("[]string"), // Tags
			mock.AnythingOfType("float64"),  // Timestamp
		).Return(nil)
	}
	m.On("ServiceCheck",
		mock.AnythingOfType("string"),                          // checkName (e.g: docker.exit)
		mock.AnythingOfType("servicecheck.ServiceCheckStatus"), // (e.g: servicecheck.ServiceCheckOK)
		mock.AnythingOfType("string"),                          // Hostname
		mock.AnythingOfType("[]string"),                        // Tags
		mock.AnythingOfType("string"),                          // message
	).Return()
	m.On("Event", mock.AnythingOfType("event.Event")).Return()
	// The second argument should have been `mock.AnythingOfType("[]byte")` instead of `mock.AnythingOfType("[]uint8")`
	// See https://github.com/stretchr/testify/issues/387
	m.On("EventPlatformEvent", mock.AnythingOfType("[]uint8"), mock.AnythingOfType("string")).Return()
	m.On("HistogramBucket",
		mock.AnythingOfType("string"),   // metric name
		mock.AnythingOfType("int64"),    // value
		mock.AnythingOfType("float64"),  // lower bound
		mock.AnythingOfType("float64"),  // upper bound
		mock.AnythingOfType("bool"),     // monotonic
		mock.AnythingOfType("string"),   // hostname
		mock.AnythingOfType("[]string"), // tags
		mock.AnythingOfType("bool"),     // FlushFirstValue
	).Return()
	m.On("GetSenderStats", mock.AnythingOfType("stats.SenderStats")).Return()
	m.On("DisableDefaultHostname", mock.AnythingOfType("bool")).Return()
	m.On("SetCheckCustomTags", mock.AnythingOfType("[]string")).Return()
	m.On("SetCheckService", mock.AnythingOfType("string")).Return()
	m.On("FinalizeCheckServiceTag").Return()
	m.On("SetNoIndex", mock.AnythingOfType("bool")).Return()
	m.On("Commit").Return()
	m.On("OrchestratorMetadata",
		mock.AnythingOfType("[]process.MessageBody"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("int"),
	).Return()
}

// ResetCalls makes the mock forget previous calls
func (m *MockSender) ResetCalls() {
	m.Mock.Calls = m.Mock.Calls[0:0]
}
