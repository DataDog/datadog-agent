// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows || darwin

package notableeventsimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// newTestEventForwarder creates test forwarder and hostname components with isolated dependencies.
func newTestEventForwarder(t *testing.T) (eventplatform.Forwarder, hostnameinterface.Component) {
	t.Helper()
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	return eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression), hostname
}

type recordingEventForwarder struct {
	nonblockingErr   error
	blockingErr      error
	nonblockingCalls int
	blockingCalls    int
}

func (f *recordingEventForwarder) SendEventPlatformEvent(*message.Message, string) error {
	f.nonblockingCalls++
	return f.nonblockingErr
}

func (f *recordingEventForwarder) SendEventPlatformEventBlocking(*message.Message, string) error {
	f.blockingCalls++
	return f.blockingErr
}

func (f *recordingEventForwarder) Purge() map[string][]*message.Message {
	return nil
}

// TestSubmitter_DrainChannelAndPayloadFormat verifies shutdown drains events into the expected envelope.
func TestSubmitter_DrainChannelAndPayloadFormat(t *testing.T) {
	// Create noop forwarder
	forwarder, hostname := newTestEventForwarder(t)

	// Create event channel
	eventChan := make(chan eventPayload)

	// Create submitter with forwarder
	sub := newSubmitter(forwarder, eventChan, hostname)

	// Start submitter
	sub.start()

	// Send multiple test events
	numEvents := 5
	for i := 0; i < numEvents; i++ {
		eventChan <- eventPayload{
			Timestamp: time.Now(),
			EventType: "Test event type",
			Title:     fmt.Sprintf("Test event %d", i),
			Message:   "Test message",
			Custom: map[string]interface{}{
				"windows_event_log": map[string]interface{}{
					"test_key": "test_value",
				},
			},
		}
	}

	// Stop submitter (close channel and wait for drain)
	close(eventChan)
	sub.stop()

	// Verify sent messages
	sentMessages := forwarder.Purge()
	eventsV2Messages := sentMessages[eventplatform.EventTypeEventManagement]
	require.Len(t, eventsV2Messages, numEvents, "Expected %d events to be sent", numEvents)

	// Verify structure for each event
	for i := 0; i < numEvents; i++ {
		var payload map[string]interface{}
		err := json.Unmarshal(eventsV2Messages[i].GetContent(), &payload)
		require.NoError(t, err, "Payload should be valid JSON")

		data, ok := payload["data"].(map[string]interface{})
		require.True(t, ok, "Payload should have 'data' field")
		attributes, ok := data["attributes"].(map[string]interface{})
		require.True(t, ok, "Data should have 'attributes' field")
		title, ok := attributes["title"].(string)
		require.True(t, ok, "Attributes should have 'title' field")
		assert.Equal(t, fmt.Sprintf("Test event %d", i), title)
		if i != 0 {
			continue
		}

		// Static envelope fields need checking only once; titles above verify all events drain in order.
		assert.Equal(t, "event", data["type"])
		assert.Equal(t, "Test message", attributes["message"])
		assert.Equal(t, "alert", attributes["category"])
		nestedAttrs, ok := attributes["attributes"].(map[string]interface{})
		require.True(t, ok, "Attributes should have nested 'attributes' field")
		assert.Equal(t, "error", nestedAttrs["status"])
		assert.Equal(t, "5", nestedAttrs["priority"])
		custom, ok := nestedAttrs["custom"].(map[string]interface{})
		require.True(t, ok, "Nested attributes should have 'custom' field")
		windowsEventLog, ok := custom["windows_event_log"].(map[string]interface{})
		require.True(t, ok, "Custom should have 'windows_event_log' field")
		assert.Equal(t, "test_value", windowsEventLog["test_key"])
	}
}

// TestSubmitterReportsForwarderFailure verifies the optional completion channel receives submission errors.
func TestSubmitterReportsForwarderFailure(t *testing.T) {
	_, hostname := newTestEventForwarder(t)
	expectedErr := errors.New("forwarder failed")
	eventChan := make(chan eventPayload, 1)
	completion := make(chan error, 1)
	forwarder := &recordingEventForwarder{nonblockingErr: expectedErr}
	submitter := newSubmitter(forwarder, eventChan, hostname)
	submitter.start()

	eventChan <- eventPayload{Title: "failed event", completion: completion}
	close(eventChan)
	// Read completion only after stop to prove an error result cannot block shutdown.
	submitter.stop()

	assert.ErrorIs(t, <-completion, expectedErr)
	assert.Equal(t, 1, forwarder.nonblockingCalls)
	assert.Zero(t, forwarder.blockingCalls)
}

func TestSubmitterReportsNonblockingSuccess(t *testing.T) {
	_, hostname := newTestEventForwarder(t)
	eventChan := make(chan eventPayload, 1)
	completion := make(chan error, 1)
	forwarder := &recordingEventForwarder{}
	submitter := newSubmitter(forwarder, eventChan, hostname)
	submitter.start()

	eventChan <- eventPayload{Title: "successful event", completion: completion}
	close(eventChan)
	submitter.stop()

	assert.NoError(t, <-completion)
	assert.Equal(t, 1, forwarder.nonblockingCalls)
	assert.Zero(t, forwarder.blockingCalls)
}

func TestSubmitterWithoutCompletionUsesBlockingForwarder(t *testing.T) {
	_, hostname := newTestEventForwarder(t)
	eventChan := make(chan eventPayload, 1)
	forwarder := &recordingEventForwarder{}
	submitter := newSubmitter(forwarder, eventChan, hostname)
	submitter.start()

	eventChan <- eventPayload{Title: "Windows event"}
	close(eventChan)
	submitter.stop()

	assert.Zero(t, forwarder.nonblockingCalls)
	assert.Equal(t, 1, forwarder.blockingCalls)
}
