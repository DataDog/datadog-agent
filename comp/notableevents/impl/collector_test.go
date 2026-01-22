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

	agentConfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
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

	// Create tmpdir to store bookmark. Necessary to isolate test runs from each other.
	testDir := t.TempDir()
	mockConfig := agentConfigmock.New(t)
	mockConfig.SetWithoutSource("run_path", testDir)

	// === First collector instance ===
	outChan := make(chan eventPayload)
	collector := createTestCollector(t, ti, channelPath, eventSource, outChan)

	err = collector.start()
	require.NoError(t, err)

	waitForSubscription(t, collector)

	// Generate and collect 3 test events
	err = ti.GenerateEvents(eventSource, 3)
	require.NoError(t, err)

	receivedEvents := collectEvents(t, outChan, 3)
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

	// Stop first collector - this should save the bookmark
	collector.stop()

	// Verify bookmark was persisted to disk
	bookmarkXML, err := persistentcache.Read(bookmarkPersistentCacheKey)
	require.NoError(t, err)
	assert.NotEmpty(t, bookmarkXML, "Bookmark should be persisted after processing events")

	// === Test bookmark persistence across restart ===
	// Generate events while collector is stopped (simulating downtime)
	err = ti.GenerateEvents(eventSource, 2)
	require.NoError(t, err)

	// Create second collector instance (simulating restart)
	outChan2 := make(chan eventPayload)
	collector2 := createTestCollector(t, ti, channelPath, eventSource, outChan2)

	err = collector2.start()
	require.NoError(t, err)
	defer collector2.stop()

	waitForSubscription(t, collector2)

	// Collect events - should only receive the 2 events generated during downtime
	receivedEvents2 := collectEvents(t, outChan2, 2)
	require.Len(t, receivedEvents2, 2, "Second collector should receive 2 events generated during downtime")

	// Verify no duplicate events from the original 3
	select {
	case unexpected := <-outChan2:
		t.Fatalf("Second collector received unexpected event (possible duplicate): %+v", unexpected)
	case <-time.After(500 * time.Millisecond):
		// Expected - no more events
	}

	// Verify bookmark was updated
	bookmarkXML2, err := persistentcache.Read(bookmarkPersistentCacheKey)
	require.NoError(t, err)
	assert.NotEqual(t, bookmarkXML, bookmarkXML2, "Bookmark should be updated after processing new events")
}

// createTestCollector creates a collector configured for testing with the given parameters
func createTestCollector(t *testing.T, ti eventlog_test.APITester, channelPath, eventSource string, outChan chan eventPayload) *collector {
	c, err := newCollector(outChan)
	require.NoError(t, err)

	// Customize for testing with test API and test channel
	c.api = ti.API()
	c.query = fmt.Sprintf(`<QueryList><Query Id="0"><Select Path="%s">*</Select></Query></QueryList>`, channelPath)

	// Add test event source to the lookup so test events can be processed
	testEventDef := &eventDefinition{
		Provider:  eventSource,
		EventID:   1000,
		EventType: "Test event",
		Title:     "Test event title",
		Message:   "Test event message",
		Channel:   channelPath,
	}
	c.eventLookup[eventKey{Provider: testEventDef.Provider, EventID: testEventDef.EventID}] = testEventDef

	return c
}

// waitForSubscription waits for the collector's subscription to be running
func waitForSubscription(t *testing.T, c *collector) {
	t.Helper()
	ctx := t.Context()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if c.sub != nil && c.sub.Running() {
			return
		}
		select {
		case <-ticker.C:
			// Continue waiting
		case <-ctx.Done():
			t.Fatal("Context cancelled while waiting for subscription to start")
		}
	}
}

// collectEvents collects n events from the channel
func collectEvents(t *testing.T, outChan <-chan eventPayload, n int) []eventPayload {
	t.Helper()
	ctx := t.Context()
	var events []eventPayload
	for len(events) < n {
		select {
		case e := <-outChan:
			events = append(events, e)
		case <-ctx.Done():
			t.Fatalf("Context cancelled while collecting events: expected %d, got %d", n, len(events))
		}
	}
	return events
}
