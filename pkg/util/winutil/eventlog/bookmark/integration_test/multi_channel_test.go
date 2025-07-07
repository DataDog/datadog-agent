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

// Windows Event Log APIs support querying multiple channels with XML QueryList,
// and bookmark format is different - it contains entries for multiple event logs
func TestMultiChannelXMLQueryList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := NewEvtQueryAPI()

	t.Log("=== Multi-Channel XML QueryList Bookmark Testing ===")

	// Test both single-channel and multi-channel approaches
	testCases := []MultiChannelTest{
		{
			Name:        "Single Channel - System Only",
			Description: "Traditional single channel query using channel_path",
			QueryType:   "single",
			ChannelPath: "System",
			Query:       "*[System[(Level=1 or Level=2 or Level=3)]]",
		},
		{
			Name:        "Multi-Channel - Application & System",
			Description: "XML QueryList spanning Application and System logs",
			QueryType:   "multi",
			ChannelPath: "", // Ignored in multi-channel mode as Branden noted
			Query: `<QueryList>
  <Query Id="0" Path="Application">
    <Select Path="Application">*[System[(Level=1 or Level=2 or Level=3)]]</Select>
  </Query>
  <Query Id="1" Path="System">
    <Select Path="System">*[System[(Level=1 or Level=2 or Level=3)]]</Select>
  </Query>
</QueryList>`,
		},
		{
			Name:        "Multi-Channel - System, Application & Security",
			Description: "XML QueryList spanning three major event logs",
			QueryType:   "multi",
			ChannelPath: "",
			Query: `<QueryList>
  <Query Id="0" Path="System">
    <Select Path="System">*[System[(Level=1 or Level=2 or Level=3)]]</Select>
  </Query>
  <Query Id="1" Path="Application">
    <Select Path="Application">*[System[(Level=1 or Level=2 or Level=3)]]</Select>
  </Query>
  <Query Id="2" Path="Security">
    <Select Path="Security">*</Select>
  </Query>
</QueryList>`,
		},
	}

	var results []MultiChannelResult

	for i, testCase := range testCases {
		t.Logf("\n--- Test Case %d/%d: %s ---", i+1, len(testCases), testCase.Name)
		t.Logf("Description: %s", testCase.Description)
		t.Logf("Query Type: %s", testCase.QueryType)

		if testCase.QueryType == "single" {
			t.Logf("Channel Path: %s", testCase.ChannelPath)
		}

		t.Logf("Query: %s", strings.ReplaceAll(testCase.Query, "\n", "\\n"))

		result := executeMultiChannelTest(t, api, testCase)
		results = append(results, result)

		if result.Success {
			t.Logf("Success: %d events, bookmark created", result.EventCount)
			t.Logf("Bookmark XML length: %d characters", len(result.BookmarkXML))

			// Analyze bookmark structure for this test case
			analyzeMultiChannelBookmark(t, testCase, result.BookmarkXML)
		} else {
			t.Logf("Failed: %v", result.Error)
		}
	}

	// Compare bookmark formats between single and multi-channel
	t.Logf("\n=== COMPARATIVE ANALYSIS ===")
	compareBookmarkFormats(t, results)
}

// MultiChannelTest defines a test case for single vs multi-channel queries
type MultiChannelTest struct {
	Name        string
	Description string
	QueryType   string // "single" or "multi"
	ChannelPath string // Used only for single-channel queries
	Query       string // XPath for single, XML QueryList for multi
}

// MultiChannelResult holds results from a multi-channel test
type MultiChannelResult struct {
	TestCase    MultiChannelTest
	Success     bool
	EventCount  int
	BookmarkXML string
	Error       error
}

// executeMultiChannelTest runs a single test case
func executeMultiChannelTest(t *testing.T, api *EvtQueryAPI, testCase MultiChannelTest) MultiChannelResult {
	result := MultiChannelResult{TestCase: testCase}

	// Determine the correct flags and path based on query type
	var flags uint
	var path string

	if testCase.QueryType == "single" {
		// Single channel mode - use channel path
		flags = 0x1 | 0x200 // EvtQueryChannelPath | EvtQueryReverseDirection
		path = testCase.ChannelPath
	} else {
		// Multi-channel mode - path is ignored, query contains the XML QueryList
		flags = 0x2 | 0x200 // EvtQueryFilePath | EvtQueryReverseDirection (FilePath flag works for QueryList)
		path = ""           // Path parameter is ignored when using XML QueryList
	}

	t.Logf("  Using flags: %#x, path: '%s'", flags, path)

	// Execute the query
	resultSetHandle, err := api.EvtQuery(0, path, testCase.Query, flags)
	if err != nil {
		result.Error = fmt.Errorf("EvtQuery failed: %v", err)
		return result
	}
	defer evtapi.EvtCloseResultSet(api, resultSetHandle)

	// Get events from the query result
	const maxEvents = 1
	eventHandles := make([]evtapi.EventRecordHandle, maxEvents)

	returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, maxEvents, 3000)
	if err != nil {
		if err == windows.ERROR_NO_MORE_ITEMS {
			result.Success = true
			result.EventCount = 0
			t.Logf("  No events found matching criteria")
			return result
		}
		result.Error = fmt.Errorf("EvtNext failed: %v", err)
		return result
	}

	result.Success = true
	result.EventCount = len(returnedHandles)

	// Create bookmark from the first event
	if len(returnedHandles) > 0 {
		bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
		if err != nil {
			result.Error = fmt.Errorf("failed to create bookmark: %v", err)
		} else {
			err = bookmark.Update(returnedHandles[0])
			if err != nil {
				result.Error = fmt.Errorf("failed to update bookmark: %v", err)
			} else {
				bookmarkXML, err := bookmark.Render()
				if err != nil {
					result.Error = fmt.Errorf("failed to render bookmark: %v", err)
				} else {
					result.BookmarkXML = bookmarkXML
				}
			}
			bookmark.Close()
		}
	}

	// Clean up event handles
	for _, handle := range returnedHandles {
		evtapi.EvtCloseRecord(api, handle)
	}

	return result
}

// analyzeMultiChannelBookmark analyzes bookmark structure for single vs multi-channel
func analyzeMultiChannelBookmark(t *testing.T, testCase MultiChannelTest, bookmarkXML string) {
	t.Logf("  Bookmark Analysis for %s:", testCase.QueryType)

	if len(bookmarkXML) == 0 {
		t.Logf("    Empty bookmark XML")
		return
	}

	// Count number of <Bookmark> elements
	bookmarkCount := countSubstring(bookmarkXML, "<Bookmark")
	t.Logf("    Number of <Bookmark> elements: %d", bookmarkCount)

	// Extract all Channel attributes
	channels := extractAllChannelAttributes(bookmarkXML)
	t.Logf("    Channels found: %v", channels)

	// Extract all RecordId attributes
	recordIds := extractAllRecordIdAttributes(bookmarkXML)
	t.Logf("    RecordIds found: %v", recordIds)

	// Show the complete XML for comparison (cleaned for display)
	cleanedXML := cleanBookmarkForDisplay(bookmarkXML)
	t.Logf("    Complete Bookmark XML:")
	for i, line := range strings.Split(cleanedXML, "\n") {
		t.Logf("      %d: %s", i+1, line)
	}

	// Validate expectations based on query type
	if testCase.QueryType == "single" {
		if bookmarkCount == 1 && len(channels) == 1 {
			t.Logf("    Single-channel bookmark format as expected")
		} else {
			t.Logf("    Unexpected format for single-channel query")
		}
	} else {
		if bookmarkCount > 1 || len(channels) > 1 {
			t.Logf("    Multi-channel bookmark format detected!")
		} else if bookmarkCount == 1 && len(channels) == 1 {
			t.Logf("    Single-channel format for multi-channel query - may indicate only one channel had events")
		} else {
			t.Logf("    Unexpected bookmark format")
		}
	}
}

// compareBookmarkFormats compares results between single and multi-channel tests
func compareBookmarkFormats(t *testing.T, results []MultiChannelResult) {
	var singleChannelResults []MultiChannelResult
	var multiChannelResults []MultiChannelResult

	// Separate results by type
	for _, result := range results {
		if !result.Success {
			continue
		}
		if result.TestCase.QueryType == "single" {
			singleChannelResults = append(singleChannelResults, result)
		} else {
			multiChannelResults = append(multiChannelResults, result)
		}
	}

	t.Logf("Single-channel results: %d, Multi-channel results: %d",
		len(singleChannelResults), len(multiChannelResults))

	// Compare bookmark element counts
	if len(singleChannelResults) > 0 && len(multiChannelResults) > 0 {
		t.Log("\nBookmark Format Comparison:")

		for _, single := range singleChannelResults {
			singleBookmarks := countSubstring(single.BookmarkXML, "<Bookmark")
			singleChannels := len(extractAllChannelAttributes(single.BookmarkXML))
			t.Logf("  Single-channel (%s): %d bookmark elements, %d channels",
				single.TestCase.Name, singleBookmarks, singleChannels)
		}

		for _, multi := range multiChannelResults {
			multiBookmarks := countSubstring(multi.BookmarkXML, "<Bookmark")
			multiChannels := len(extractAllChannelAttributes(multi.BookmarkXML))
			t.Logf("  Multi-channel (%s): %d bookmark elements, %d channels",
				multi.TestCase.Name, multiBookmarks, multiChannels)
		}

		// Key finding: does multi-channel produce different bookmark format?
		hasMultiChannelBookmarks := false
		for _, multi := range multiChannelResults {
			if countSubstring(multi.BookmarkXML, "<Bookmark") > 1 {
				hasMultiChannelBookmarks = true
				break
			}
		}

		if hasMultiChannelBookmarks {
			t.Log("\nCONFIRMED: Multi-channel queries produce different bookmark format")
			t.Log("   Bookmark XML contains multiple <Bookmark> elements for multiple channels")
		} else {
			t.Log("\nMulti-channel queries produced single-channel bookmark format")
			t.Log("   This could mean: only one channel had matching events, or different behavior than expected")
		}
	}
}

// Helper functions for XML analysis

// countSubstring counts occurrences of substring in string
func countSubstring(s, substr string) int {
	count := 0
	start := 0
	for {
		pos := strings.Index(s[start:], substr)
		if pos == -1 {
			break
		}
		count++
		start += pos + len(substr)
	}
	return count
}

// extractAllChannelAttributes finds all Channel="..." attributes in bookmark XML
func extractAllChannelAttributes(xml string) []string {
	var channels []string

	// Handle both single and double quotes
	searchPatterns := []string{`Channel="`, `Channel='`}

	for _, searchStr := range searchPatterns {
		start := 0
		quote := searchStr[len(searchStr)-1:] // Get the quote character

		for {
			pos := strings.Index(xml[start:], searchStr)
			if pos == -1 {
				break
			}

			// Found Channel=" or Channel=', now find the closing quote
			valueStart := start + pos + len(searchStr)
			valueEnd := strings.Index(xml[valueStart:], quote)
			if valueEnd == -1 {
				break
			}

			channel := xml[valueStart : valueStart+valueEnd]
			channels = append(channels, channel)

			start = valueStart + valueEnd + 1
		}
	}

	return channels
}

// extractAllRecordIdAttributes finds all RecordId="..." attributes in bookmark XML
func extractAllRecordIdAttributes(xml string) []string {
	var recordIds []string

	// Handle both single and double quotes
	searchPatterns := []string{`RecordId="`, `RecordId='`}

	for _, searchStr := range searchPatterns {
		start := 0
		quote := searchStr[len(searchStr)-1:] // Get the quote character

		for {
			pos := strings.Index(xml[start:], searchStr)
			if pos == -1 {
				break
			}

			valueStart := start + pos + len(searchStr)
			valueEnd := strings.Index(xml[valueStart:], quote)
			if valueEnd == -1 {
				break
			}

			recordId := xml[valueStart : valueStart+valueEnd]
			recordIds = append(recordIds, recordId)

			start = valueStart + valueEnd + 1
		}
	}

	return recordIds
}

// TestQueryListFlags tests different flag combinations for XML QueryList
func TestQueryListFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	api := NewEvtQueryAPI()

	t.Log("=== Testing Different Flags for XML QueryList ===")

	xmlQuery := `<QueryList>
  <Query Id="0" Path="System">
    <Select Path="System">*[System[(Level=1 or Level=2 or Level=3)]]</Select>
  </Query>
  <Query Id="1" Path="Application">
    <Select Path="Application">*[System[(Level=1 or Level=2 or Level=3)]]</Select>
  </Query>
</QueryList>`

	// Test different flag combinations for XML QueryList
	flagTests := []struct {
		Name        string
		Flags       uint
		Description string
	}{
		{
			Name:        "EvtQueryFilePath",
			Flags:       0x2,
			Description: "FilePath flag (often used for XML QueryList)",
		},
		{
			Name:        "EvtQueryFilePath + Reverse",
			Flags:       0x2 | 0x200,
			Description: "FilePath + ReverseDirection",
		},
		{
			Name:        "EvtQueryChannelPath + Reverse",
			Flags:       0x1 | 0x200,
			Description: "ChannelPath + ReverseDirection (may not work with QueryList)",
		},
		{
			Name:        "EvtQueryFilePath + Tolerant",
			Flags:       0x2 | 0x1000,
			Description: "FilePath + TolerateQueryErrors",
		},
	}

	for _, flagTest := range flagTests {
		t.Logf("\n--- Testing: %s ---", flagTest.Name)
		t.Logf("Flags: %#x (%s)", flagTest.Flags, flagTest.Description)

		resultSetHandle, err := api.EvtQuery(0, "", xmlQuery, flagTest.Flags)
		if err != nil {
			t.Logf("EvtQuery failed: %v", err)
			continue
		}

		// Try to get events
		eventHandles := make([]evtapi.EventRecordHandle, 3)
		returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, 3, 2000)

		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				t.Logf("Query succeeded, no events available")
			} else {
				t.Logf("EvtNext failed: %v", err)
			}
		} else {
			t.Logf("Successfully got %d events", len(returnedHandles))

			// Create bookmark to see format
			if len(returnedHandles) > 0 {
				bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
				if err == nil {
					err = bookmark.Update(returnedHandles[0])
					if err == nil {
						bookmarkXML, err := bookmark.Render()
						if err == nil {
							bookmarkCount := countSubstring(bookmarkXML, "<Bookmark")
							channels := extractAllChannelAttributes(bookmarkXML)
							t.Logf("  Bookmark: %d elements, channels: %v", bookmarkCount, channels)
						}
					}
					bookmark.Close()
				}
			}

			// Clean up
			for _, handle := range returnedHandles {
				evtapi.EvtCloseRecord(api, handle)
			}
		}

		evtapi.EvtCloseResultSet(api, resultSetHandle)
	}
}
