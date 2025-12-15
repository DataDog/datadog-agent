// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventlog_test "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"
)

// TestBuildEventLookup asserts that events are correctly indexed by provider and event ID.
func TestBuildEventLookup(t *testing.T) {
	events := []eventDefinition{
		{Provider: "Provider-A", EventID: 1, EventType: "Type1", Title: "Title1", Message: "Msg1"},
		{Provider: "Provider-A", EventID: 2, EventType: "Type2", Title: "Title2", Message: "Msg2"},
		{Provider: "Provider-B", EventID: 1, EventType: "Type3", Title: "Title3", Message: "Msg3"},
	}

	lookup, err := buildEventLookup(events)
	require.NoError(t, err)
	require.Len(t, lookup, 3)

	// Verify lookups
	def, found := lookup[eventKey{Provider: "Provider-A", EventID: 1}]
	require.True(t, found)
	assert.Equal(t, "Type1", def.EventType)

	def, found = lookup[eventKey{Provider: "Provider-A", EventID: 2}]
	require.True(t, found)
	assert.Equal(t, "Type2", def.EventType)

	def, found = lookup[eventKey{Provider: "Provider-B", EventID: 1}]
	require.True(t, found)
	assert.Equal(t, "Type3", def.EventType)

	// Non-existent key
	_, found = lookup[eventKey{Provider: "Provider-B", EventID: 999}]
	assert.False(t, found)
}

// TestBuildEventLookup_DuplicateKey asserts that duplicate event definitions are not allowed.
func TestBuildEventLookup_DuplicateKey(t *testing.T) {
	events := []eventDefinition{
		{Provider: "Provider-A", EventID: 1, EventType: "Type1"},
		{Provider: "Provider-A", EventID: 1, EventType: "Type2"}, // Duplicate
	}

	lookup, err := buildEventLookup(events)
	require.Error(t, err)
	assert.Nil(t, lookup)
	assert.Contains(t, err.Error(), "duplicate event definition")
	assert.Contains(t, err.Error(), "Provider-A/1")
}

func TestBuildQuery(t *testing.T) {
	events := []eventDefinition{
		{
			Channel:   "System",
			QueryBody: `    <Select Path="System">*[System[Provider[@Name='Test-Provider'] and EventID=123]]</Select>`,
		},
	}

	query := buildQuery(events)

	expected := `<QueryList>
  <Query Id="0" Path="System">
    <Select Path="System">*[System[Provider[@Name='Test-Provider'] and EventID=123]]</Select>
  </Query>
</QueryList>`
	assert.Equal(t, expected, query)
}

func TestBuildQuery_MultipleEvents(t *testing.T) {
	events := []eventDefinition{
		{
			Channel:   "System",
			QueryBody: `    <Select Path="System">*[System[EventID=1]]</Select>`,
		},
		{
			Channel:   "Application",
			QueryBody: `    <Select Path="Application">*[System[EventID=2]]</Select>`,
		},
		{
			Channel: "Security",
			QueryBody: `    <Select Path="Security">*[System[EventID=3]]</Select>
    <Suppress Path="Security">*[EventData[Data='exclude']]</Suppress>`,
		},
	}

	query := buildQuery(events)

	expected := `<QueryList>
  <Query Id="0" Path="System">
    <Select Path="System">*[System[EventID=1]]</Select>
  </Query>
  <Query Id="1" Path="Application">
    <Select Path="Application">*[System[EventID=2]]</Select>
  </Query>
  <Query Id="2" Path="Security">
    <Select Path="Security">*[System[EventID=3]]</Select>
    <Suppress Path="Security">*[EventData[Data='exclude']]</Suppress>
  </Query>
</QueryList>`
	assert.Equal(t, expected, query)
}

func TestCollector_CollectEvents(t *testing.T) {
	ctx := t.Context()

	// only run test with Windows API tester, needs event rendering
	enabledAPIs := eventlog_test.GetEnabledAPITesters()
	if !slices.Contains(enabledAPIs, "Windows") {
		t.Skip("Windows API tester not enabled")
	}
	ti := eventlog_test.GetAPITesterByName("Windows", t)

	// Create test channel
	channelPath := "dd-test-notable-events"
	eventSource := "dd-test-source"

	// Set up test log
	err := ti.InstallChannel(channelPath)
	require.NoError(t, err)
	defer ti.RemoveChannel(channelPath)

	err = ti.API().EvtClearLog(channelPath)
	require.NoError(t, err)

	err = ti.InstallSource(channelPath, eventSource)
	require.NoError(t, err)
	defer ti.RemoveSource(channelPath, eventSource)

	// Create output channel
	outChan := make(chan eventPayload)

	// Create collector using constructor
	collector, err := newCollector(outChan)
	require.NoError(t, err)

	// Customize for testing with test API and test channel
	collector.api = ti.API()
	collector.query = fmt.Sprintf(`<QueryList><Query Id="0"><Select Path="%s">*</Select></Query></QueryList>`, channelPath) // Collect all events from the test channel

	// Add test event source to the lookup so test events can be processed
	testEventDef := &eventDefinition{
		Provider:  eventSource,
		EventID:   1000,
		EventType: "Test event",
		Title:     "Test event title",
		Message:   "Test event message",
		Channel:   channelPath,
	}
	collector.eventLookup[eventKey{Provider: testEventDef.Provider, EventID: testEventDef.EventID}] = testEventDef

	// Start collector
	err = collector.start()
	require.NoError(t, err)
	defer collector.stop()

	// Wait for subscription to be running
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if collector.sub.Running() {
			break
		}
		select {
		case <-ticker.C:
			// Continue waiting
		case <-ctx.Done():
			t.Fatal("Context cancelled while waiting for subscription to start")
		}
	}

	// Generate test events using helper
	err = ti.GenerateEvents(eventSource, 3)
	require.NoError(t, err)

	// Wait for events to be collected and sent to output channel
	var receivedEvents []eventPayload

	for i := 0; i < 3; i++ {
		select {
		case payload := <-outChan:
			receivedEvents = append(receivedEvents, payload)
		case <-ctx.Done():
			t.Fatalf("Context cancelled while waiting for event %d. Received %d events so far", i+1, len(receivedEvents))
		}
	}

	// Verify we received all expected events
	require.Len(t, receivedEvents, 3, "Should have received 3 events")

	// Verify event payloads have correct metadata from test event definition
	for i, event := range receivedEvents {
		assert.Equal(t, "Test event title", event.Title, "Event %d should have correct title", i)
		assert.Equal(t, "Test event", event.EventType, "Event %d should have correct event type", i)
		assert.Equal(t, "Test event message", event.Message, "Event %d should have correct message", i)
		assert.NotNil(t, event.Custom, "Event %d should have custom data", i)
		assert.Contains(t, event.Custom, "windows_event_log", "Event %d custom data should contain windows_event_log", i)
	}

	// Verify no more events are in the channel
	select {
	case unexpected := <-outChan:
		t.Fatalf("Received unexpected event: %+v", unexpected)
	case <-time.After(100 * time.Millisecond):
		// Expected - no more events
	}
}
