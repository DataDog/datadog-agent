//go:build windows

package integration_test

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	evtsubscribe "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

// TestBookmarkCreationProofOfConcept demonstrates how to:
// 1. Query the Windows Event Log to get event handles
// 2. Create bookmarks from those handles
// 3. Inspect the XML format of the bookmarks
//
// This test is designed to run against a real Windows system and requires
// the System event log to have at least one event.
func TestBookmarkCreationProofOfConcept(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := winevtapi.New()

	// Test configuration - using System log which should always have events
	channelPath := "System"
	query := "*" // Get all events

	t.Logf("Starting proof-of-concept for bookmark creation")
	t.Logf("Channel: %s, Query: %s", channelPath, query)

	// Step 1: Create a subscription to get event handles
	t.Log("Step 1: Creating subscription to get event handles...")

	sub := evtsubscribe.NewPullSubscription(
		channelPath,
		query,
		evtsubscribe.WithWindowsEventLogAPI(api),
		evtsubscribe.WithStartAtOldestRecord(), // Start from oldest to ensure we get events
		evtsubscribe.WithEventBatchCount(5),    // Limit to 5 events for testing
	)

	// Start the subscription
	err := sub.Start()
	if err != nil {
		t.Fatalf("Failed to start subscription: %v", err)
	}
	defer sub.Stop()

	// Step 2: Get some events from the subscription
	t.Log("Step 2: Getting events from subscription...")

	var eventRecords []*evtapi.EventRecord

	// Wait for events with timeout
	select {
	case events := <-sub.GetEvents():
		if len(events) == 0 {
			t.Fatal("No events received from subscription")
		}
		eventRecords = events
		t.Logf("Received %d events", len(eventRecords))
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for events")
	}

	// Step 3: Create bookmarks from the event handles and inspect XML
	t.Log("Step 3: Creating bookmarks and inspecting XML format...")

	for i, eventRecord := range eventRecords {
		t.Logf("\n--- Processing Event %d ---", i+1)

		// Create a new bookmark
		bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
		if err != nil {
			t.Errorf("Failed to create bookmark for event %d: %v", i+1, err)
			continue
		}
		defer bookmark.Close()

		// Update the bookmark to point to this event
		err = bookmark.Update(eventRecord.EventRecordHandle)
		if err != nil {
			t.Errorf("Failed to update bookmark for event %d: %v", i+1, err)
			continue
		}

		// Render the bookmark to XML to inspect its format
		bookmarkXML, err := bookmark.Render()
		if err != nil {
			t.Errorf("Failed to render bookmark for event %d: %v", i+1, err)
			continue
		}

		t.Logf("Bookmark XML for event %d:", i+1)
		t.Logf("Length: %d bytes", len(bookmarkXML))
		t.Logf("Content:\n%s", bookmarkXML)

		// Also try to get the event XML for context
		eventXMLData, err := api.EvtRenderEventXml(eventRecord.EventRecordHandle)
		if err != nil {
			t.Logf("Could not render event XML: %v", err)
		} else {
			eventXML := windows.UTF16ToString(eventXMLData)
			t.Logf("Associated Event XML (first 500 chars):")
			if len(eventXML) > 500 {
				t.Logf("%s...", eventXML[:500])
			} else {
				t.Logf("%s", eventXML)
			}
		}

		// Test round-trip: create a new bookmark from the XML
		t.Logf("Testing round-trip bookmark creation...")
		roundTripBookmark, err := evtbookmark.New(
			evtbookmark.WithWindowsEventLogAPI(api),
			evtbookmark.FromXML(bookmarkXML),
		)
		if err != nil {
			t.Errorf("Failed to create bookmark from XML: %v", err)
		} else {
			roundTripXML, err := roundTripBookmark.Render()
			if err != nil {
				t.Errorf("Failed to render round-trip bookmark: %v", err)
			} else {
				if roundTripXML == bookmarkXML {
					t.Logf("✓ Round-trip successful - XML matches")
				} else {
					t.Errorf("✗ Round-trip failed - XML differs")
					t.Logf("Original:  %s", bookmarkXML)
					t.Logf("Round-trip: %s", roundTripXML)
				}
			}
			roundTripBookmark.Close()
		}

		// Clean up event handle
		evtapi.EvtCloseRecord(api, eventRecord.EventRecordHandle)
	}
}

// TestBookmarkXMLStructureAnalysis provides detailed analysis of bookmark XML structure
func TestBookmarkXMLStructureAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := winevtapi.New()

	t.Log("Analyzing bookmark XML structure...")

	// Test with different event logs to see if bookmark format varies
	testChannels := []string{
		"System",
		"Application",
		"Security",
	}

	for _, channel := range testChannels {
		t.Logf("\n=== Testing Channel: %s ===", channel)

		sub := evtsubscribe.NewPullSubscription(
			channel,
			"*",
			evtsubscribe.WithWindowsEventLogAPI(api),
			evtsubscribe.WithStartAtOldestRecord(),
			evtsubscribe.WithEventBatchCount(1), // Just need one event
		)

		err := sub.Start()
		if err != nil {
			t.Logf("Could not access %s channel: %v", channel, err)
			continue
		}

		// Get one event
		select {
		case events := <-sub.GetEvents():
			if len(events) > 0 {
				bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
				if err != nil {
					t.Errorf("Failed to create bookmark: %v", err)
					sub.Stop()
					continue
				}

				err = bookmark.Update(events[0].EventRecordHandle)
				if err != nil {
					t.Errorf("Failed to update bookmark: %v", err)
					bookmark.Close()
					sub.Stop()
					continue
				}

				bookmarkXML, err := bookmark.Render()
				if err != nil {
					t.Errorf("Failed to render bookmark: %v", err)
				} else {
					t.Logf("Channel %s bookmark XML:", channel)
					t.Logf("%s", bookmarkXML)

					// Analyze the XML structure
					analyzeBookmarkXML(t, channel, bookmarkXML)
				}

				bookmark.Close()
				evtapi.EvtCloseRecord(api, events[0].EventRecordHandle)
			} else {
				t.Logf("No events found in %s channel", channel)
			}
		case <-time.After(5 * time.Second):
			t.Logf("Timeout getting events from %s channel", channel)
		}

		sub.Stop()
	}
}

// analyzeBookmarkXML provides detailed analysis of the bookmark XML structure
func analyzeBookmarkXML(t *testing.T, channel, xml string) {
	t.Logf("Analysis for %s:", channel)

	// Basic structure analysis
	if len(xml) == 0 {
		t.Logf("  - Empty XML")
		return
	}

	// Check for common XML elements in bookmarks
	elements := []string{
		"BookmarkList",
		"Bookmark",
		"Channel",
		"RecordId",
		"IsCurrent",
		"CreationTime",
		"LogfileName",
	}

	found := make(map[string]bool)
	for _, element := range elements {
		if contains(xml, element) {
			found[element] = true
			t.Logf("  ✓ Contains: %s", element)
		}
	}

	// Check for attributes vs elements
	for element := range found {
		if contains(xml, fmt.Sprintf(`%s="`, element)) {
			t.Logf("  - %s appears as attribute", element)
		}
		if contains(xml, fmt.Sprintf("<%s>", element)) {
			t.Logf("  - %s appears as element", element)
		}
	}

	// Extract any numeric values that might be record IDs
	t.Logf("  - XML length: %d characters", len(xml))
	t.Logf("  - Channel reference: %s", extractChannelReference(xml))
}

// Helper function to check if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

// extractChannelReference tries to find channel reference in bookmark XML
func extractChannelReference(xml string) string {
	// Look for Channel="..." pattern
	start := findSubstring(xml, `Channel="`)
	if !start {
		return "not found"
	}

	// Find the actual position
	pos := -1
	searchStr := `Channel="`
	for i := 0; i <= len(xml)-len(searchStr); i++ {
		match := true
		for j := 0; j < len(searchStr); j++ {
			if toLower(xml[i+j]) != toLower(searchStr[j]) {
				match = false
				break
			}
		}
		if match {
			pos = i + len(searchStr)
			break
		}
	}

	if pos == -1 {
		return "not found"
	}

	// Extract until closing quote
	end := pos
	for end < len(xml) && xml[end] != '"' {
		end++
	}

	if end >= len(xml) {
		return "malformed"
	}

	return xml[pos:end]
}

// TestEmptyBookmarkXML tests what an empty bookmark looks like
func TestEmptyBookmarkXML(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := winevtapi.New()

	t.Log("Testing empty bookmark XML format...")

	// Create an empty bookmark
	bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
	if err != nil {
		t.Fatalf("Failed to create empty bookmark: %v", err)
	}
	defer bookmark.Close()

	// Try to render empty bookmark
	bookmarkXML, err := bookmark.Render()
	if err != nil {
		t.Logf("Empty bookmark render failed (expected): %v", err)
		t.Log("This tells us that bookmarks must be updated with an event before they can be rendered")
	} else {
		t.Logf("Empty bookmark XML:")
		t.Logf("Length: %d bytes", len(bookmarkXML))
		t.Logf("Content: %s", bookmarkXML)
	}
}
