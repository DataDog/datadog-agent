// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mocksender

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logimpl "github.com/DataDog/datadog-agent/comp/core/log/impl"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwardernoop "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/noop-impl"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"

	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// NewMockSender initiates the aggregator and returns a
// functional mocked Sender for testing
func NewMockSender(tb testing.TB, id checkid.ID) *MockSender {
	return NewMockSenderWithSenderManager(id, CreateDefaultDemultiplexer(tb))
}

// CreateDefaultDemultiplexer creates a default demultiplexer for testing and
// registers t.Cleanup to stop it when the test ends.
func CreateDefaultDemultiplexer(tb testing.TB) *aggregator.AgentDemultiplexer {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 1 * time.Hour
	opts.DontStartForwarders = true
	log := logimpl.NewTemporaryLoggerWithoutInit()
	sharedForwarder := defaultforwardernoop.NewComponent()
	orchestratorForwarder := option.New[defaultforwarder.Forwarder](defaultforwardernoop.NewComponent())
	// Use hostnamemock as hostnameimpl.NewHostnameService() would start a goroutine blocking on
	// ctx.Done() forever, while the hostname is never observed in NewNoopEventPlatformForwarder.
	hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname("hostname"))
	eventPlatformForwarder := option.NewPtr[eventplatform.Forwarder](eventplatformimpl.NewNoopEventPlatformForwarder(hostname, logscompressionmock.NewMockCompressor()))
	taggerComponent := nooptagger.NewComponent()
	filterList := filterlist.NewNoopFilterList()
	demux := aggregator.InitAndStartAgentDemultiplexer(log, sharedForwarder, &orchestratorForwarder, opts, eventPlatformForwarder, haagentmock.NewMockHaAgent(), metricscompressionmock.NewMockCompressor(), taggerComponent, filterList, "")
	tb.Cleanup(demux.Stop)
	return demux
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
	checkTags     []string
	infraTagger   *infratags.Tagger
}

// GetSenderManager returns the instance of sender.SenderManager
func (m *MockSender) GetSenderManager() sender.SenderManager {
	return m.senderManager
}

// SetupAcceptAll sets mock expectations to accept any call in the Sender interface
func (m *MockSender) SetupAcceptAll() {
	metricCalls := []string{"Rate", "Count", "MonotonicCount", "Counter", "Histogram", "Historate", "Gauge", "GaugeNoIndex", "Distribution"}
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
	bucketCalls := []string{"OpenmetricsBucket", "HistogramBucket"}
	for _, call := range bucketCalls {
		m.On(call,
			mock.AnythingOfType("string"),   // metric name
			mock.AnythingOfType("int64"),    // value
			mock.AnythingOfType("float64"),  // lower bound
			mock.AnythingOfType("float64"),  // upper bound
			mock.AnythingOfType("bool"),     // monotonic
			mock.AnythingOfType("string"),   // hostname
			mock.AnythingOfType("[]string"), // tags
			mock.AnythingOfType("bool"),     // FlushFirstValue
		).Return()
	}
	m.On("GetSenderStats", mock.AnythingOfType("stats.SenderStats")).Return()
	m.On("DisableDefaultHostname", mock.AnythingOfType("bool")).Return()
	m.On("SetCheckCustomTags", mock.AnythingOfType("[]string")).Return()
	m.On("SetInfraTagger", mock.Anything).Return()
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
