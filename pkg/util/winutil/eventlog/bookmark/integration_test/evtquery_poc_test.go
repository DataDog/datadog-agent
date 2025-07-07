//go:build windows

package integration_test

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
)

// EvtQueryAPI extends the base API with EvtQuery functionality
type EvtQueryAPI struct {
	evtapi.API
	evtQuery *windows.LazyProc
}

// NewEvtQueryAPI creates an API wrapper that includes EvtQuery
func NewEvtQueryAPI() *EvtQueryAPI {
	baseAPI := winevtapi.New()
	wevtapi := windows.NewLazySystemDLL("wevtapi.dll")

	return &EvtQueryAPI{
		API:      baseAPI,
		evtQuery: wevtapi.NewProc("EvtQuery"),
	}
}

// EvtQuery implements the Windows EvtQuery API call
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtquery
func (api *EvtQueryAPI) EvtQuery(
	session evtapi.EventSessionHandle,
	path string,
	query string,
	flags uint) (evtapi.EventResultSetHandle, error) {

	// Convert Go strings to Windows API strings
	pathPtr, err := winutil.UTF16PtrOrNilFromString(path)
	if err != nil {
		return evtapi.EventResultSetHandle(0), err
	}

	queryPtr, err := winutil.UTF16PtrOrNilFromString(query)
	if err != nil {
		return evtapi.EventResultSetHandle(0), err
	}

	// Call EvtQuery
	r1, _, lastErr := api.evtQuery.Call(
		uintptr(session),                  // Session (0 for local)
		uintptr(unsafe.Pointer(pathPtr)),  // Path
		uintptr(unsafe.Pointer(queryPtr)), // Query
		uintptr(flags))                    // Flags

	// EvtQuery returns NULL on error
	if r1 == 0 {
		return evtapi.EventResultSetHandle(0), lastErr
	}

	return evtapi.EventResultSetHandle(r1), nil
}

// TestEvtQueryBookmarkCreation demonstrates using EvtQuery directly to get events and create bookmarks
func TestEvtQueryBookmarkCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := NewEvtQueryAPI()

	t.Log("=== Proof-of-Concept: EvtQuery + Bookmark Creation ===")

	// Test configuration
	channelPath := "System"
	query := "*[System[(Level=1 or Level=2 or Level=3)]]" // Errors, warnings, info

	t.Logf("Channel Path: %s", channelPath)
	t.Logf("Query: %s", query)

	// Step 1: Use EvtQuery to get a result set handle
	t.Log("\nStep 1: Calling EvtQuery to get result set...")

	resultSetHandle, err := api.EvtQuery(
		0, // Session (0 = local computer)
		channelPath,
		query,
		evtapi.EvtQueryChannelPath|evtapi.EvtQueryReverseDirection)

	if err != nil {
		t.Fatalf("EvtQuery failed: %v", err)
	}
	defer evtapi.EvtCloseResultSet(api, resultSetHandle)

	t.Logf("✓ EvtQuery successful, handle: %#x", resultSetHandle)

	// Step 2: Use EvtNext to get event handles from the query result
	t.Log("\nStep 2: Using EvtNext to get event handles...")

	const maxEvents = 3
	eventHandles := make([]evtapi.EventRecordHandle, maxEvents)

	returnedHandles, err := api.EvtNext(
		resultSetHandle,
		eventHandles,
		maxEvents,
		1000) // 1 second timeout

	if err != nil {
		// Check if it's just no events available
		if err == windows.ERROR_NO_MORE_ITEMS {
			t.Log("No events available in query result")
			return
		}
		t.Fatalf("EvtNext failed: %v", err)
	}

	t.Logf("✓ EvtNext returned %d event handles", len(returnedHandles))

	// Step 3: Create bookmarks from the event handles and inspect XML
	t.Log("\nStep 3: Creating bookmarks from event handles...")

	for i, eventHandle := range returnedHandles {
		t.Logf("\n--- Processing Event Handle %d: %#x ---", i+1, eventHandle)

		// Create bookmark
		bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
		if err != nil {
			t.Errorf("Failed to create bookmark: %v", err)
			evtapi.EvtCloseRecord(api, eventHandle)
			continue
		}

		// Update bookmark with the event handle
		err = bookmark.Update(eventHandle)
		if err != nil {
			t.Errorf("Failed to update bookmark: %v", err)
			bookmark.Close()
			evtapi.EvtCloseRecord(api, eventHandle)
			continue
		}

		// Render bookmark to XML
		bookmarkXML, err := bookmark.Render()
		if err != nil {
			t.Errorf("Failed to render bookmark: %v", err)
			bookmark.Close()
			evtapi.EvtCloseRecord(api, eventHandle)
			continue
		}

		// Clean up bookmark XML for display (remove direction info)
		cleanBookmarkXML := cleanBookmarkForDisplay(bookmarkXML)
		t.Logf("Bookmark XML (Length: %d):", len(bookmarkXML))
		t.Logf("%s", cleanBookmarkXML)

		// Render the event XML for comparison
		eventXMLData, err := api.EvtRenderEventXml(eventHandle)
		if err != nil {
			t.Logf("Could not render event XML: %v", err)
		} else {
			eventXML := windows.UTF16ToString(eventXMLData)
			t.Logf("\nCorresponding Event XML (first 1000 chars):")
			if len(eventXML) > 1000 {
				t.Logf("%s...", eventXML[:1000])
			} else {
				t.Logf("%s", eventXML)
			}
		}

		// Extract key information from the bookmark XML
		extractBookmarkInfo(t, bookmarkXML)

		// Clean up
		bookmark.Close()
		evtapi.EvtCloseRecord(api, eventHandle)
	}
}

// extractBookmarkInfo analyzes bookmark XML and extracts key information
func extractBookmarkInfo(t *testing.T, bookmarkXML string) {
	t.Log("\nBookmark Analysis:")

	if len(bookmarkXML) == 0 {
		t.Log("  - Empty bookmark XML")
		return
	}

	// Simple XML parsing to extract key attributes
	info := make(map[string]string)

	// Look for common bookmark attributes
	attributes := []string{
		"Channel",
		"RecordId",
		"IsCurrent",
		"LogfileName",
	}

	for _, attr := range attributes {
		value := extractXMLAttribute(bookmarkXML, attr)
		if value != "" {
			info[attr] = value
			t.Logf("  - %s: %s", attr, value)
		}
	}

	// Check if this looks like a valid bookmark
	if info["Channel"] != "" && info["RecordId"] != "" {
		t.Log("  ✓ Bookmark appears to contain valid position information")
	} else {
		t.Log("  ? Bookmark may not contain expected position information")
	}
}

// extractXMLAttribute extracts an attribute value from XML
// This is a simple implementation for demo purposes
func extractXMLAttribute(xml, attrName string) string {
	// Look for pattern: AttrName="value"
	pattern := attrName + `="`
	start := -1

	// Find the pattern (case insensitive)
	for i := 0; i <= len(xml)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if toLower(xml[i+j]) != toLower(pattern[j]) {
				match = false
				break
			}
		}
		if match {
			start = i + len(pattern)
			break
		}
	}

	if start == -1 {
		return ""
	}

	// Find the closing quote
	end := start
	for end < len(xml) && xml[end] != '"' {
		end++
	}

	if end >= len(xml) {
		return ""
	}

	return xml[start:end]
}

// TestEvtQueryWithDifferentFlags tests EvtQuery with various flag combinations
func TestEvtQueryWithDifferentFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := NewEvtQueryAPI()

	t.Log("=== Testing EvtQuery with Different Flags ===")

	channelPath := "System"
	query := "*"

	// Test different flag combinations
	testCases := []struct {
		name  string
		flags uint
	}{
		{"Forward Direction", evtapi.EvtQueryChannelPath},
		{"Reverse Direction", evtapi.EvtQueryChannelPath | evtapi.EvtQueryReverseDirection},
		{"Forward with Tolerant", evtapi.EvtQueryChannelPath | evtapi.EvtQueryTolerateQueryErrors},
	}

	for _, tc := range testCases {
		t.Logf("\n--- Testing: %s (flags: %#x) ---", tc.name, tc.flags)

		resultSetHandle, err := api.EvtQuery(0, channelPath, query, tc.flags)
		if err != nil {
			t.Logf("❌ EvtQuery failed: %v", err)
			continue
		}

		// Try to get one event
		eventHandles := make([]evtapi.EventRecordHandle, 1)
		returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, 1, 1000)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				t.Logf("⚠️  No events available")
			} else {
				t.Logf("❌ EvtNext failed: %v", err)
			}
		} else {
			t.Logf("✓ Successfully got %d event(s)", len(returnedHandles))

			// Create a bookmark from the first event
			if len(returnedHandles) > 0 {
				bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
				if err == nil {
					err = bookmark.Update(returnedHandles[0])
					if err == nil {
						bookmarkXML, err := bookmark.Render()
						if err == nil {
							t.Logf("  Bookmark XML length: %d characters", len(bookmarkXML))
							channel := extractXMLAttribute(bookmarkXML, "Channel")
							recordId := extractXMLAttribute(bookmarkXML, "RecordId")
							t.Logf("  Channel: %s, RecordId: %s", channel, recordId)
						}
					}
					bookmark.Close()
				}
				evtapi.EvtCloseRecord(api, returnedHandles[0])
			}
		}

		evtapi.EvtCloseResultSet(api, resultSetHandle)
	}
}

// TestBookmarkXMLFormat demonstrates the exact format of bookmark XML
func TestBookmarkXMLFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := NewEvtQueryAPI()

	t.Log("=== Demonstrating Bookmark XML Format ===")

	// Get one event from the system log
	resultSetHandle, err := api.EvtQuery(0, "System", "*", evtapi.EvtQueryChannelPath|evtapi.EvtQueryReverseDirection) // Newest first
	if err != nil {
		t.Fatalf("EvtQuery failed: %v", err)
	}
	defer evtapi.EvtCloseResultSet(api, resultSetHandle)

	eventHandles := make([]evtapi.EventRecordHandle, 1)
	returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, 1, 5000)
	if err != nil {
		t.Fatalf("EvtNext failed: %v", err)
	}

	if len(returnedHandles) == 0 {
		t.Fatal("No events returned")
	}

	eventHandle := returnedHandles[0]
	defer evtapi.EvtCloseRecord(api, eventHandle)

	// Create bookmark
	bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
	if err != nil {
		t.Fatalf("Failed to create bookmark: %v", err)
	}
	defer bookmark.Close()

	err = bookmark.Update(eventHandle)
	if err != nil {
		t.Fatalf("Failed to update bookmark: %v", err)
	}

	bookmarkXML, err := bookmark.Render()
	if err != nil {
		t.Fatalf("Failed to render bookmark: %v", err)
	}

	// Clean up bookmark XML for display
	cleanBookmarkXML := cleanBookmarkForDisplay(bookmarkXML)
	t.Log("\n📋 COMPLETE BOOKMARK XML:")
	t.Log("=" + fmt.Sprintf("%s", repeatString("=", 60)))
	t.Log(cleanBookmarkXML)
	t.Log("=" + fmt.Sprintf("%s", repeatString("=", 60)))

	t.Logf("\n📊 BOOKMARK STATISTICS:")
	t.Logf("  - Length: %d characters", len(bookmarkXML))
	t.Logf("  - Lines: %d", countLines(bookmarkXML))

	t.Log("\n🔍 EXTRACTED INFORMATION:")
	attributes := []string{"Channel", "RecordId", "IsCurrent", "LogfileName"}
	for _, attr := range attributes {
		value := extractXMLAttribute(bookmarkXML, attr)
		if value != "" {
			t.Logf("  - %s: %s", attr, value)
		} else {
			t.Logf("  - %s: [not found]", attr)
		}
	}
}

// Helper functions
func repeatString(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

func countLines(s string) int {
	count := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			count++
		}
	}
	return count
}

// cleanBookmarkForDisplay removes direction information from bookmark XML for cleaner test output
func cleanBookmarkForDisplay(bookmarkXML string) string {
	// Remove Direction='backward' or Direction='forward' attributes
	cleaned := bookmarkXML

	// Replace Direction='backward' with nothing
	cleaned = strings.ReplaceAll(cleaned, " Direction='backward'", "")
	cleaned = strings.ReplaceAll(cleaned, " Direction='forward'", "")

	// Also handle double quotes
	cleaned = strings.ReplaceAll(cleaned, " Direction=\"backward\"", "")
	cleaned = strings.ReplaceAll(cleaned, " Direction=\"forward\"", "")

	// Clean up any extra whitespace that might be left
	cleaned = strings.ReplaceAll(cleaned, "  ", " ")

	return cleaned
}
