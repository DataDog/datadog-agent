//go:build windows

package integration_test

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
)

// TestRecentEventsAndBookmarks shows the last 5 warning/error events from major Windows Event Logs
// This gives you a quick view of what's happening on your system and tests bookmark creation
func TestRecentEventsAndBookmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := NewEvtQueryAPI()

	t.Log("=== Recent Windows Events (Warnings & Errors) ===")
	t.Log("Showing last 5 warning/error events from major event logs")

	// Define channels to check (excluding Security to avoid access issues)
	channels := []EventLogChannel{
		{
			Name:        "System",
			Description: "Windows system events",
			Query:       "*[System[(Level=1 or Level=2 or Level=3)]]", // Errors and warnings
		},
		{
			Name:        "Application", 
			Description: "Application events",
			Query:       "*[System[(Level=1 or Level=2 or Level=3)]]",
		},
		{
			Name:        "Setup",
			Description: "Windows setup and installation events", 
			Query:       "*[System[(Level=1 or Level=2 or Level=3)]]",
		},
	}

	totalEvents := 0
	
	for _, channel := range channels {
		t.Logf("\n--- %s (%s) ---", channel.Name, channel.Description)
		
		events := getRecentEvents(t, api, channel, 5)
		if len(events) == 0 {
			t.Logf("No recent warning/error events found")
			continue
		}
		
		totalEvents += len(events)
		t.Logf("Found %d recent events:", len(events))
		
		for i, event := range events {
			// Format timestamp for display
			timeDisplay := event.TimeCreated
			if timeDisplay != "" {
				// Remove the 'T' and 'Z' and truncate to just date and time
				timeDisplay = strings.ReplaceAll(timeDisplay, "T", " ")
				timeDisplay = strings.ReplaceAll(timeDisplay, "Z", "")
				if len(timeDisplay) > 19 {
					timeDisplay = timeDisplay[:19] // Keep YYYY-MM-DD HH:MM:SS
				}
			}
			
			t.Logf("  %d. EventID:%s RecordID:%s Time:%s", 
				i+1, event.EventID, event.RecordID, timeDisplay)
			t.Logf("     Level:%s Provider:%s", event.Level, event.Provider)
			if event.Message != "" {
				// Truncate long messages
				msg := event.Message
				if len(msg) > 100 {
					msg = msg[:100] + "..."
				}
				t.Logf("     Message: %s", strings.ReplaceAll(msg, "\n", " "))
			}
		}
		
		// Test bookmark creation with the first event
		if len(events) > 0 {
			testBookmarkCreation(t, api, channel.Name, events[0])
		}
	}
	
	t.Logf("\nTotal warning/error events found: %d", totalEvents)
}

// EventLogChannel represents a Windows Event Log channel to query
type EventLogChannel struct {
	Name        string
	Description string
	Query       string
}

// EventInfo holds basic information about a Windows event
type EventInfo struct {
	EventID     string
	RecordID    string
	TimeCreated string
	Level       string
	Provider    string
	Message     string
	Handle      evtapi.EventRecordHandle
}

// getRecentEvents retrieves recent events from the specified channel
func getRecentEvents(t *testing.T, api *EvtQueryAPI, channel EventLogChannel, maxEvents int) []EventInfo {
	// Query for recent events (newest first)
	resultSetHandle, err := api.EvtQuery(
		0,                  // Local session
		channel.Name,       // Channel name
		channel.Query,      // XPath query
		0x1|0x200,         // EvtQueryChannelPath | EvtQueryReverseDirection
	)
	
	if err != nil {
		t.Logf("Could not query %s: %v", channel.Name, err)
		return nil
	}
	defer evtapi.EvtCloseResultSet(api, resultSetHandle)
	
	// Get event handles
	eventHandles := make([]evtapi.EventRecordHandle, maxEvents)
	returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, uint(maxEvents), 2000)
	if err != nil {
		if err == windows.ERROR_NO_MORE_ITEMS {
			return nil
		}
		t.Logf("EvtNext failed for %s: %v", channel.Name, err)
		return nil
	}
	
	var events []EventInfo
	
	// Extract information from each event
	for _, handle := range returnedHandles {
		event := extractEventInfo(t, api, handle)
		if event != nil {
			events = append(events, *event)
		}
		// Don't close the handle yet - we might use it for bookmarks
	}
	
	return events
}

// extractEventInfo extracts key information from a Windows event
func extractEventInfo(t *testing.T, api *EvtQueryAPI, handle evtapi.EventRecordHandle) *EventInfo {
	// Get the event XML
	eventXMLData, err := api.EvtRenderEventXml(handle)
	if err != nil {
		return nil
	}
	
	eventXML := windows.UTF16ToString(eventXMLData)
	
	// Parse key fields from XML (simple string parsing)
	event := &EventInfo{
		Handle:      handle,
		EventID:     extractXMLValue(eventXML, "<EventID>"),
		RecordID:    extractXMLValue(eventXML, "<EventRecordID>"),
		TimeCreated: extractXMLAttribute(eventXML, "SystemTime"),
		Level:       extractXMLValue(eventXML, "<Level>"),
		Provider:    extractProviderName(eventXML),
	}
	
	// Try to get a simple message (first Data element)
	if strings.Contains(eventXML, "<Data") {
		start := strings.Index(eventXML, "<Data")
		if start != -1 {
			end := strings.Index(eventXML[start:], "</Data>")
			if end != -1 {
				dataSection := eventXML[start : start+end]
				dataStart := strings.Index(dataSection, ">")
				if dataStart != -1 {
					event.Message = dataSection[dataStart+1:]
				}
			}
		}
	}
	
	return event
}

// extractXMLValue extracts content between XML tags
func extractXMLValue(xml, tag string) string {
	start := strings.Index(xml, tag)
	if start == -1 {
		return ""
	}
	
	// Find the end of the opening tag
	tagEnd := strings.Index(xml[start:], ">")
	if tagEnd == -1 {
		return ""
	}
	
	contentStart := start + tagEnd + 1
	
	// Find the closing tag
	closingTag := "</" + tag[1:] // Remove < from tag
	contentEnd := strings.Index(xml[contentStart:], closingTag)
	if contentEnd == -1 {
		return ""
	}
	
	return xml[contentStart : contentStart+contentEnd]
}

// extractProviderName extracts the Provider Name attribute from XML
func extractProviderName(xml string) string {
	// Look for <Provider Name="..." pattern
	start := strings.Index(xml, "<Provider")
	if start == -1 {
		return ""
	}
	
	// Find Name attribute
	nameStart := strings.Index(xml[start:], `Name="`)
	if nameStart == -1 {
		// Try single quotes
		nameStart = strings.Index(xml[start:], `Name='`)
		if nameStart == -1 {
			return ""
		}
		// Extract with single quotes
		valueStart := start + nameStart + 6 // len(`Name='`)
		valueEnd := strings.Index(xml[valueStart:], "'")
		if valueEnd == -1 {
			return ""
		}
		return xml[valueStart : valueStart+valueEnd]
	}
	
	// Extract with double quotes
	valueStart := start + nameStart + 6 // len(`Name="`)
	valueEnd := strings.Index(xml[valueStart:], `"`)
	if valueEnd == -1 {
		return ""
	}
	
	return xml[valueStart : valueStart+valueEnd]
}

// testBookmarkCreation tests creating a bookmark from an event
func testBookmarkCreation(t *testing.T, api *EvtQueryAPI, channelName string, event EventInfo) {
	t.Logf("  Testing bookmark creation for %s event %s...", channelName, event.RecordID)
	
	// Create bookmark
	bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
	if err != nil {
		t.Logf("  Failed to create bookmark: %v", err)
		evtapi.EvtCloseRecord(api, event.Handle)
		return
	}
	defer bookmark.Close()
	
	// Update bookmark with this event
	err = bookmark.Update(event.Handle)
	if err != nil {
		t.Logf("  Failed to update bookmark: %v", err)
		evtapi.EvtCloseRecord(api, event.Handle)
		return
	}
	
	// Render bookmark to XML
	bookmarkXML, err := bookmark.Render()
	if err != nil {
		t.Logf("  Failed to render bookmark: %v", err)
		evtapi.EvtCloseRecord(api, event.Handle)
		return
	}
	
	// Clean and display bookmark
	cleanedXML := cleanBookmarkForDisplay(bookmarkXML)
	cleanedXML = strings.ReplaceAll(cleanedXML, "\n", " ")
	cleanedXML = strings.ReplaceAll(cleanedXML, "\t", " ")
	cleanedXML = strings.ReplaceAll(cleanedXML, "  ", " ")
	t.Logf("  Bookmark created: %s", cleanedXML)
	
	// Verify bookmark contains expected information
	if strings.Contains(bookmarkXML, channelName) && strings.Contains(bookmarkXML, event.RecordID) {
		t.Logf("  Bookmark validation: PASS (contains channel and record ID)")
	} else {
		t.Logf("  Bookmark validation: WARN (missing expected information)")
	}
	
	// Clean up
	evtapi.EvtCloseRecord(api, event.Handle)
}

// TestMultiChannelBookmarkComparison tests the difference between single and multi-channel bookmarks
func TestMultiChannelBookmarkComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := NewEvtQueryAPI()

	t.Log("=== Multi-Channel Bookmark Format Comparison ===")

	// Test 1: Single-channel bookmark
	t.Log("\n--- Single-Channel Query ---")
	singleBookmark := createSingleChannelBookmark(t, api, "System")
	
	// Test 2: Multi-channel bookmark  
	t.Log("\n--- Multi-Channel Query ---")
	multiBookmark := createMultiChannelBookmark(t, api, []string{"System", "Application"})
	
	// Compare the formats
	t.Log("\n--- Format Comparison ---")
	if singleBookmark != "" && multiBookmark != "" {
		singleCount := countSubstring(singleBookmark, "<Bookmark")
		multiCount := countSubstring(multiBookmark, "<Bookmark")
		
		t.Logf("Single-channel bookmark elements: %d", singleCount)
		t.Logf("Multi-channel bookmark elements: %d", multiCount)
		
		if multiCount > singleCount {
			t.Log("CONFIRMED: Multi-channel bookmarks contain more bookmark elements")
		} else {
			t.Log("NOTE: Both formats have similar bookmark element counts")
		}
	}
}

// createSingleChannelBookmark creates a bookmark from a single channel
func createSingleChannelBookmark(t *testing.T, api *EvtQueryAPI, channel string) string {
	resultSetHandle, err := api.EvtQuery(0, channel, "*[System[(Level=1 or Level=2 or Level=3)]]", 0x1|0x200)
	if err != nil {
		t.Logf("Single-channel query failed: %v", err)
		return ""
	}
	defer evtapi.EvtCloseResultSet(api, resultSetHandle)
	
	eventHandles := make([]evtapi.EventRecordHandle, 1)
	returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, 1, 1000)
	if err != nil || len(returnedHandles) == 0 {
		t.Log("No events found for single-channel test")
		return ""
	}
	
	bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
	if err != nil {
		evtapi.EvtCloseRecord(api, returnedHandles[0])
		return ""
	}
	defer bookmark.Close()
	
	err = bookmark.Update(returnedHandles[0])
	if err != nil {
		evtapi.EvtCloseRecord(api, returnedHandles[0])
		return ""
	}
	
	bookmarkXML, err := bookmark.Render()
	evtapi.EvtCloseRecord(api, returnedHandles[0])
	
	if err != nil {
		return ""
	}
	
	cleanedXML := cleanBookmarkForDisplay(bookmarkXML)
	t.Logf("Single-channel bookmark: %s", strings.ReplaceAll(cleanedXML, "\n", " "))
	return bookmarkXML
}

// createMultiChannelBookmark creates a bookmark from multiple channels
func createMultiChannelBookmark(t *testing.T, api *EvtQueryAPI, channels []string) string {
	// Build XML QueryList
	xmlQuery := "<QueryList>\n"
	for i, channel := range channels {
		xmlQuery += fmt.Sprintf(`  <Query Id="%d" Path="%s">
    <Select Path="%s">*[System[(Level=1 or Level=2 or Level=3)]]</Select>
  </Query>
`, i, channel, channel)
	}
	xmlQuery += "</QueryList>"
	
	t.Logf("Multi-channel query: %s", strings.ReplaceAll(xmlQuery, "\n", "\\n"))
	
	resultSetHandle, err := api.EvtQuery(0, "", xmlQuery, 0x2|0x200) // FilePath flag for QueryList
	if err != nil {
		t.Logf("Multi-channel query failed: %v", err)
		return ""
	}
	defer evtapi.EvtCloseResultSet(api, resultSetHandle)
	
	eventHandles := make([]evtapi.EventRecordHandle, 1)
	returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, 1, 1000)
	if err != nil || len(returnedHandles) == 0 {
		t.Log("No events found for multi-channel test")
		return ""
	}
	
	bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
	if err != nil {
		evtapi.EvtCloseRecord(api, returnedHandles[0])
		return ""
	}
	defer bookmark.Close()
	
	err = bookmark.Update(returnedHandles[0])
	if err != nil {
		evtapi.EvtCloseRecord(api, returnedHandles[0])
		return ""
	}
	
	bookmarkXML, err := bookmark.Render()
	evtapi.EvtCloseRecord(api, returnedHandles[0])
	
	if err != nil {
		return ""
	}
	
	cleanedXML := cleanBookmarkForDisplay(bookmarkXML)
	t.Logf("Multi-channel bookmark: %s", strings.ReplaceAll(cleanedXML, "\n", " "))
	return bookmarkXML
}