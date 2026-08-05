// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableeventsimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notableevents "github.com/DataDog/datadog-agent/pkg/notableevents/types"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

type mockNotableEventsClient struct {
	mu sync.Mutex

	events   []notableevents.Event
	getErrs  []error
	ackErrs  []error
	getCalls int
	ackCalls [][]string
	getFunc  func(context.Context) ([]notableevents.Event, error)
	ackFunc  func(context.Context, []string) error
}

// GetEvents returns configured polling results while recording call count.
func (m *mockNotableEventsClient) GetEvents(ctx context.Context) ([]notableevents.Event, error) {
	m.mu.Lock()
	m.getCalls++
	getFunc := m.getFunc
	if getFunc != nil {
		m.mu.Unlock()
		return getFunc(ctx)
	}
	if len(m.getErrs) > 0 {
		err := m.getErrs[0]
		m.getErrs = m.getErrs[1:]
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
	}
	events := append([]notableevents.Event(nil), m.events...)
	m.mu.Unlock()
	return events, nil
}

// Ack records each acknowledgement attempt and returns its configured result.
func (m *mockNotableEventsClient) Ack(ctx context.Context, ids []string) error {
	m.mu.Lock()
	ids = append([]string(nil), ids...)
	m.ackCalls = append(m.ackCalls, ids)
	ackFunc := m.ackFunc
	if ackFunc != nil {
		m.mu.Unlock()
		return ackFunc(ctx, ids)
	}
	if len(m.ackErrs) == 0 {
		m.mu.Unlock()
		return nil
	}
	err := m.ackErrs[0]
	m.ackErrs = m.ackErrs[1:]
	m.mu.Unlock()
	return err
}

// acknowledgements returns a concurrency-safe deep copy of recorded acknowledgement calls.
func (m *mockNotableEventsClient) acknowledgements() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	acks := make([][]string, len(m.ackCalls))
	for i, ids := range m.ackCalls {
		acks[i] = append([]string(nil), ids...)
	}
	return acks
}

// getCallCount returns the concurrency-safe number of polling attempts.
func (m *mockNotableEventsClient) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getCalls
}

// testDarwinEvent returns a representative sanitized event from system-probe.
func testDarwinEvent() notableevents.Event {
	return notableevents.Event{
		ID:        "event-1",
		Timestamp: time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC),
		EventType: "Application crash",
		Title:     "Application crash: ExampleApp",
		Message:   "An application crashed unexpectedly",
		Custom: map[string]interface{}{
			"macos_diagnostic_report": map[string]interface{}{
				"incident_id": "incident-1",
				"scope":       "user",
				"report": map[string]interface{}{
					"procName": "ExampleApp",
				},
			},
		},
	}
}

// testDarwinEvents returns representative events with deterministic IDs and titles.
func testDarwinEvents(ids ...string) []notableevents.Event {
	events := make([]notableevents.Event, 0, len(ids))
	for _, id := range ids {
		event := testDarwinEvent()
		event.ID = id
		event.Title = "Application crash: " + id
		events = append(events, event)
	}
	return events
}

// runPoll drives one poll through sequential submission completions.
func runPoll(t *testing.T, c *collector, eventChan <-chan eventPayload, submitErrs ...error) []eventPayload {
	t.Helper()
	done := make(chan struct{})
	go func() {
		c.poll(context.Background())
		close(done)
	}()

	payloads := make([]eventPayload, 0, len(submitErrs))
	for _, submitErr := range submitErrs {
		select {
		case payload := <-eventChan:
			payloads = append(payloads, payload)
			payload.completion <- submitErr
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event payload")
		}
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for poll to finish")
	}
	return payloads
}

func TestWaitForDarwinSubmission(t *testing.T) {
	submitFailure := errors.New("submit failed")
	tests := []struct {
		name          string
		cancel        bool
		buffer        bool
		submitErr     error
		wantCompleted bool
		wantCanceled  bool
	}{
		{name: "live success", buffer: true, wantCompleted: true},
		{name: "live failure", buffer: true, submitErr: submitFailure, wantCompleted: true},
		{name: "canceled buffered success", cancel: true, buffer: true, wantCompleted: true, wantCanceled: true},
		{name: "canceled buffered failure", cancel: true, buffer: true, submitErr: submitFailure, wantCompleted: true, wantCanceled: true},
		{name: "canceled without completion", cancel: true, wantCanceled: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			completion := make(chan error, 1)
			if test.buffer {
				completion <- test.submitErr
			}
			if test.cancel {
				cancel()
			}

			completed, canceled, submitErr := waitForSubmission(ctx, completion)
			assert.ErrorIs(t, submitErr, test.submitErr)
			assert.Equal(t, test.wantCompleted, completed)
			assert.Equal(t, test.wantCanceled, canceled)
		})
	}
}

// TestDarwinPollerAcknowledgesSuccessfulSubmission verifies delivered events are acknowledged.
func TestDarwinPollerAcknowledgesSuccessfulSubmission(t *testing.T) {
	client := &mockNotableEventsClient{events: []notableevents.Event{testDarwinEvent()}}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)

	payloads := runPoll(t, c, eventChan, nil)
	require.Len(t, payloads, 1)
	payload := payloads[0]

	assert.Equal(t, testDarwinEvent().Timestamp, payload.Timestamp)
	assert.Equal(t, testDarwinEvent().Title, payload.Title)
	assert.Equal(t, testDarwinEvent().Custom, payload.Custom)
	assert.Equal(t, [][]string{{"event-1"}}, client.acknowledgements())
}

// TestDarwinPollerDoesNotAcknowledgeSubmitFailure verifies failed delivery leaves events pending.
func TestDarwinPollerDoesNotAcknowledgeSubmitFailure(t *testing.T) {
	client := &mockNotableEventsClient{events: []notableevents.Event{testDarwinEvent()}}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)

	runPoll(t, c, eventChan, errors.New("forwarder failed"))

	assert.Empty(t, client.acknowledgements())
}

// TestDarwinPollerBatchesOnlySuccessfulSubmissions verifies mixed outcomes produce one ordered ACK.
func TestDarwinPollerBatchesOnlySuccessfulSubmissions(t *testing.T) {
	client := &mockNotableEventsClient{events: testDarwinEvents("event-1", "event-2", "event-3")}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)

	payloads := runPoll(t, c, eventChan, nil, errors.New("forwarder failed"), nil)

	require.Len(t, payloads, 3)
	assert.Equal(t, []string{
		"Application crash: event-1",
		"Application crash: event-2",
		"Application crash: event-3",
	}, []string{payloads[0].Title, payloads[1].Title, payloads[2].Title})
	assert.Equal(t, [][]string{{"event-1", "event-3"}}, client.acknowledgements())
}

// TestDarwinPollerSkipsAckWithoutSuccessfulSubmissions verifies an empty success set sends no request.
func TestDarwinPollerSkipsAckWithoutSuccessfulSubmissions(t *testing.T) {
	client := &mockNotableEventsClient{events: testDarwinEvents("event-1", "event-2")}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)

	runPoll(t, c, eventChan, errors.New("first failed"), errors.New("second failed"))

	assert.Empty(t, client.acknowledgements())
}

// TestDarwinPollerAcknowledgesPriorSuccessesOnCancellation verifies shutdown preserves unambiguous deliveries.
func TestDarwinPollerAcknowledgesPriorSuccessesOnCancellation(t *testing.T) {
	client := &mockNotableEventsClient{events: testDarwinEvents("event-1", "event-2")}
	finalAckHadDeadline := false
	client.ackFunc = func(ctx context.Context, _ []string) error {
		_, finalAckHadDeadline = ctx.Deadline()
		return nil
	}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.poll(ctx)
		close(done)
	}()

	first := <-eventChan
	first.completion <- nil
	<-eventChan
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancelled poll")
	}
	assert.Equal(t, [][]string{{"event-1"}}, client.acknowledgements())
	assert.True(t, finalAckHadDeadline)
}

func TestDarwinPollerFinalAckIncludesCompletionObservableAtCancellation(t *testing.T) {
	client := &mockNotableEventsClient{events: testDarwinEvents("event-1", "event-2")}
	finalAckHadDeadline := false
	client.ackFunc = func(ctx context.Context, _ []string) error {
		_, finalAckHadDeadline = ctx.Deadline()
		return nil
	}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.poll(ctx)
		close(done)
	}()

	first := <-eventChan
	first.completion <- nil
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancelled poll")
	}
	assert.Equal(t, [][]string{{"event-1"}}, client.acknowledgements())
	assert.True(t, finalAckHadDeadline)
}

// TestDarwinPollerRetriesAfterAcknowledgementFailure verifies failed acknowledgements cause redelivery.
func TestDarwinPollerRetriesAfterAcknowledgementFailure(t *testing.T) {
	client := &mockNotableEventsClient{
		events:  []notableevents.Event{testDarwinEvent()},
		ackErrs: []error{errors.New("ack failed"), nil},
	}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)

	runPoll(t, c, eventChan, nil)
	runPoll(t, c, eventChan, nil)

	assert.Equal(t, [][]string{{"event-1"}, {"event-1"}}, client.acknowledgements())
}

// TestDarwinPollerRetriesStartupAndSocketFailures verifies transient system-probe failures recover.
func TestDarwinPollerRetriesStartupAndSocketFailures(t *testing.T) {
	client := &mockNotableEventsClient{
		events:  []notableevents.Event{testDarwinEvent()},
		getErrs: []error{sysprobeclient.ErrNotStartedYet, errors.New("socket unavailable")},
	}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, 5*time.Millisecond)
	require.NoError(t, c.start())

	payload := <-eventChan
	payload.completion <- nil
	require.Eventually(t, func() bool {
		return len(client.acknowledgements()) == 1
	}, time.Second, 5*time.Millisecond)
	c.stop()

	assert.GreaterOrEqual(t, client.getCallCount(), 3)
	assert.Equal(t, [][]string{{"event-1"}}, client.acknowledgements())
}

// TestDarwinPollerCancellationUnblocksPendingSubmission verifies shutdown does not await delivery forever.
func TestDarwinPollerCancellationUnblocksPendingSubmission(t *testing.T) {
	client := &mockNotableEventsClient{
		events: []notableevents.Event{testDarwinEvent()},
	}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)
	require.NoError(t, c.start())

	var payload eventPayload
	select {
	case payload = <-eventChan:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pending submission")
	}

	stopped := make(chan struct{})
	go func() {
		c.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("collector stop blocked")
	}
	payload.completion <- nil
	assert.Empty(t, client.acknowledgements())
}

func TestDarwinPollerStopCancelsBlockedGetEvents(t *testing.T) {
	getStarted := make(chan struct{})
	client := &mockNotableEventsClient{}
	client.getFunc = func(ctx context.Context) ([]notableevents.Event, error) {
		close(getStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c := newCollectorWithClient(make(chan eventPayload), client, time.Hour)
	require.NoError(t, c.start())
	<-getStarted

	stopped := make(chan struct{})
	go func() {
		c.stop()
		close(stopped)
	}()
	<-stopped
	assert.Empty(t, client.acknowledgements())
}

func TestDarwinPollerRetriesCanceledInFlightAckOnceDuringShutdown(t *testing.T) {
	firstAckStarted := make(chan struct{})
	finalAckStarted := make(chan struct{})
	ackAttempt := 0
	client := &mockNotableEventsClient{events: []notableevents.Event{testDarwinEvent()}}
	client.ackFunc = func(ctx context.Context, _ []string) error {
		ackAttempt++
		if ackAttempt == 1 {
			close(firstAckStarted)
			<-ctx.Done()
			return ctx.Err()
		}
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
		close(finalAckStarted)
		return nil
	}
	eventChan := make(chan eventPayload)
	c := newCollectorWithClient(eventChan, client, time.Hour)
	require.NoError(t, c.start())

	payload := <-eventChan
	payload.completion <- nil
	<-firstAckStarted

	stopped := make(chan struct{})
	go func() {
		c.stop()
		close(stopped)
	}()
	<-finalAckStarted
	<-stopped

	assert.Equal(t, [][]string{{"event-1"}, {"event-1"}}, client.acknowledgements())
	assert.Equal(t, 2, ackAttempt)
}
