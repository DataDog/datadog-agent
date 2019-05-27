package writer

import (
	"math/rand"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
)

// payloadConstructedHandlerArgs encodes the arguments passed to a PayloadConstructedHandler call.
type payloadConstructedHandlerArgs struct {
	payload *payload
	stats   interface{}
}

// testEndpoint represents a mocked endpoint that replies with a configurable error and records successful and failed
// payloads.
type testEndpoint struct {
	sync.RWMutex
	err             error
	successPayloads []*payload
	errorPayloads   []*payload
}

func (e *testEndpoint) baseURL() string { return "<testEndpoint>" }

// Write mocks the writing of a payload to a remote endpoint, recording it and replying with the configured error (or
// success in its absence).
func (e *testEndpoint) write(payload *payload) error {
	e.Lock()
	defer e.Unlock()
	if e.err != nil {
		e.errorPayloads = append(e.errorPayloads, payload)
	} else {
		e.successPayloads = append(e.successPayloads, payload)
	}
	return e.err
}

func (e *testEndpoint) Error() error {
	e.RLock()
	defer e.RUnlock()
	return e.err
}

// ErrorPayloads returns all the error payloads registered with the test endpoint.
func (e *testEndpoint) ErrorPayloads() []*payload {
	e.RLock()
	defer e.RUnlock()
	return e.errorPayloads
}

// SuccessPayloads returns all the success payloads registered with the test endpoint.
func (e *testEndpoint) SuccessPayloads() []*payload {
	e.RLock()
	defer e.RUnlock()
	return e.successPayloads
}

// SetError sets the passed error on the endpoint.
func (e *testEndpoint) SetError(err error) {
	e.Lock()
	defer e.Unlock()
	e.err = err
}

func (e *testEndpoint) String() string {
	return "testEndpoint"
}

// RandomPayload creates a new payload instance using random data and up to 32 bytes.
func randomPayload() *payload {
	return randomSizedPayload(rand.Intn(32))
}

// randomSizedPayload creates a new payload instance using random data with the specified size.
func randomSizedPayload(size int) *payload {
	return newPayload(testutil.RandomSizedBytes(size), testutil.RandomStringMap())
}

// testPayloadSender is a PayloadSender that is connected to a testEndpoint, used for testing.
type testPayloadSender struct {
	*queuableSender
	testEndpoint *testEndpoint
}

// newTestPayloadSender creates a new instance of a testPayloadSender.
func newTestPayloadSender() *testPayloadSender {
	testEndpoint := &testEndpoint{}
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	queuableSender := newSender(testEndpoint, conf)
	return &testPayloadSender{
		testEndpoint:   testEndpoint,
		queuableSender: queuableSender,
	}
}

// Start asynchronously starts this payload sender.
func (c *testPayloadSender) Start() {
	go c.Run()
}

// Run executes the core loop of this sender.
func (c *testPayloadSender) Run() {
	defer close(c.exit)

	for {
		select {
		case payload := <-c.in:
			stats, err := c.doSend(payload)

			if err != nil {
				c.notifyError(payload, err)
			} else {
				c.notifySuccess(payload, stats)
			}
		case <-c.exit:
			return
		}
	}
}

// Payloads allows access to all payloads recorded as being successfully sent by this sender.
func (c *testPayloadSender) Payloads() []*payload {
	return c.testEndpoint.SuccessPayloads()
}

// Endpoint allows access to the underlying testEndpoint.
func (c *testPayloadSender) Endpoint() *testEndpoint {
	return c.testEndpoint
}

func (c *testPayloadSender) setEndpoint(e endpoint) {
	c.testEndpoint = e.(*testEndpoint)
}

// testPayloadSenderMonitor monitors a PayloadSender and stores all events
type testPayloadSenderMonitor struct {
	events []monitorEvent
	sender payloadSender
	exit   chan struct{}
}

// newTestPayloadSenderMonitor creates a new testPayloadSenderMonitor monitoring the specified sender.
func newTestPayloadSenderMonitor(sender payloadSender) *testPayloadSenderMonitor {
	return &testPayloadSenderMonitor{
		sender: sender,
		exit:   make(chan struct{}),
	}
}

// Start asynchronously starts this payload monitor.
func (m *testPayloadSenderMonitor) Start() {
	go m.Run()
}

// Run executes the core loop of this monitor.
func (m *testPayloadSenderMonitor) Run() {
	defer close(m.exit)

	for {
		select {
		case event, ok := <-m.sender.Monitor():
			if !ok {
				continue // wait for exit
			}
			m.events = append(m.events, event)
		case <-m.exit:
			return
		}
	}
}

// Stop stops this payload monitor and waits for it to stop.
func (m *testPayloadSenderMonitor) Stop() {
	m.exit <- struct{}{}
	<-m.exit
}

// SuccessPayloads returns a slice containing all successful payloads.
func (m *testPayloadSenderMonitor) SuccessPayloads() []*payload {
	return m.eventPayloads(eventTypeSuccess)
}

// FailurePayloads returns a slice containing all failed payloads.
func (m *testPayloadSenderMonitor) FailurePayloads() []*payload {
	return m.eventPayloads(eventTypeFailure)
}

// FailureEvents returns all failure events.
func (m *testPayloadSenderMonitor) FailureEvents() []monitorEvent {
	return m.eventsByType(eventTypeFailure)
}

// RetryPayloads returns a slice containing all failed payloads.
func (m *testPayloadSenderMonitor) RetryPayloads() []*payload {
	return m.eventPayloads(eventTypeRetry)
}

func (m *testPayloadSenderMonitor) eventPayloads(t eventType) []*payload {
	res := make([]*payload, 0)
	for _, e := range m.events {
		if e.typ != t {
			continue
		}
		res = append(res, e.payload)
	}
	return res
}

func (m *testPayloadSenderMonitor) eventsByType(t eventType) []monitorEvent {
	res := make([]monitorEvent, 0)
	for _, e := range m.events {
		if e.typ != t {
			continue
		}
		res = append(res, e)
	}
	return res
}
