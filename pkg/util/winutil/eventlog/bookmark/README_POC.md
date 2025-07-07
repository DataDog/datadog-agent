# Windows Event Log Bookmark Creation Proof-of-Concept

This directory contains proof-of-concept tests that demonstrate how to create bookmarks from Windows Event Log entries and inspect their XML format. These tests were created to address WINA-596 (Seed/Initial Event Log bookmark issue).

## Test Files

The tests are located in the `integration_test/` directory to avoid import cycle issues.

### 1. `integration_test/bookmark_poc_test.go`
Demonstrates bookmark creation using the existing subscription mechanism:
- Uses `EvtSubscribe` to get event handles  
- Creates bookmarks from those handles
- Analyzes bookmark XML structure
- Tests round-trip bookmark creation (XML → Bookmark → XML)

### 2. `integration_test/evtquery_poc_test.go` 
Demonstrates bookmark creation using the Windows `EvtQuery` API directly:
- Implements `EvtQuery` wrapper functionality
- Uses `EvtQuery` to get event handles without subscriptions
- Creates bookmarks and inspects XML format
- Tests various query flags and configurations

### 3. `integration_test/multi_channel_test.go` ⭐ **NEW**
Implements Branden's requirement for multi-channel XML QueryList support:
- Tests single-channel vs multi-channel bookmark formats
- Uses XML QueryList to query multiple event logs simultaneously
- Compares bookmark XML structure differences between modes
- Validates that multi-channel bookmarks contain entries for multiple event logs

## Running the Tests

### Prerequisites
- Windows system (tests use `//go:build windows`)
- Administrative privileges may be required for some event logs
- Go development environment set up

### Running Individual Tests

```bash
# Navigate to the integration test directory
cd pkg/util/winutil/eventlog/bookmark/integration_test

# Run basic proof-of-concept (uses subscription)
go test -v -run TestBookmarkCreationProofOfConcept

# Run EvtQuery-based proof-of-concept  
go test -v -run TestEvtQueryBookmarkCreation

# Analyze bookmark XML structure across different channels
go test -v -run TestBookmarkXMLStructureAnalysis

# Test different EvtQuery flag combinations
go test -v -run TestEvtQueryWithDifferentFlags

# See detailed bookmark XML format
go test -v -run TestBookmarkXMLFormat

# Test empty bookmark behavior
go test -v -run TestEmptyBookmarkXML

# NEW: Test multi-channel XML QueryList (Branden's requirement)
go test -v -run TestMultiChannelXMLQueryList

# Test different flags for XML QueryList
go test -v -run TestQueryListFlags

# Run all proof-of-concept tests
go test -v -run "POC|EvtQuery|XMLFormat|Empty"

# Run all tests including multi-channel
go test -v -run "POC|EvtQuery|XMLFormat|Empty|MultiChannel"

# NEW: Show recent Windows events and test bookmarks
go test -v -run TestRecentEventsAndBookmarks

# Test multi-channel bookmark comparison
go test -v -run TestMultiChannelBookmarkComparison
```

### Running From Root Directory

```bash
# Run integration tests from project root
go test -v ./pkg/util/winutil/eventlog/bookmark/integration_test/...

# Skip slow tests
go test -v -short ./pkg/util/winutil/eventlog/bookmark/integration_test/...
```

## What These Tests Demonstrate

### Key Findings

1. **Event Handle Requirement**: Bookmarks can only be created from valid event record handles
2. **XML Structure**: Bookmark XML contains channel name, record ID, and position information
3. **Round-trip Compatibility**: Bookmarks can be serialized to XML and recreated from XML
4. **Query vs Subscribe**: Both `EvtQuery` and `EvtSubscribe` can provide event handles for bookmark creation

### Expected XML Format

The bookmark XML typically looks like:
```xml
<BookmarkList>
  <Bookmark Channel="System" RecordId="12345" IsCurrent="true"/>
</BookmarkList>
```

### WINA-596 Implications

These tests help understand the bookmark creation process for solving the initial bookmark issue:

1. **Current Limitation**: Cannot create bookmarks without first reading events
2. **Potential Solutions**:
   - Use `EvtQuery` to get the latest event, create bookmark from it
   - Parse existing bookmark XML format to understand structure
   - Query event log metadata to get latest record ID

## Test Output Examples

### Successful Bookmark Creation
```
=== Proof-of-Concept: EvtQuery + Bookmark Creation ===
Channel Path: System
Query: *[System[(Level=1 or Level=2 or Level=3)]]

Step 1: Calling EvtQuery to get result set...
✓ EvtQuery successful, handle: 0x1a2b3c4d

Step 2: Using EvtNext to get event handles...
✓ EvtNext returned 3 event handles

Step 3: Creating bookmarks from event handles...

--- Processing Event Handle 1: 0x5e6f7a8b ---
Bookmark XML (Length: 156):
<BookmarkList>
  <Bookmark Channel="System" RecordId="98765" IsCurrent="true"/>
</BookmarkList>

Bookmark Analysis:
  - Channel: System
  - RecordId: 98765
  - IsCurrent: true
  ✓ Bookmark appears to contain valid position information
```

## Key Code Patterns

### Creating Bookmark from Event Handle
```go
// Get event handle (via EvtQuery or EvtSubscribe)
eventHandle := getEventHandle()

// Create bookmark
bookmark, err := New(WithWindowsEventLogAPI(api))
if err != nil {
    return err
}
defer bookmark.Close()

// Update bookmark to point to the event
err = bookmark.Update(eventHandle)
if err != nil {
    return err
}

// Get XML representation
bookmarkXML, err := bookmark.Render()
if err != nil {
    return err
}
```

### Using EvtQuery Directly
```go
// Create extended API that includes EvtQuery
api := NewEvtQueryAPI()

// Query for events
resultSetHandle, err := api.EvtQuery(0, "System", "*", EvtQueryChannelPath)
if err != nil {
    return err
}
defer evtapi.EvtCloseResultSet(api, resultSetHandle)

// Get event handles
eventHandles := make([]evtapi.EventRecordHandle, 10)
returnedHandles, err := api.EvtNext(resultSetHandle, eventHandles, 10, 1000)
if err != nil {
    return err
}

// Create bookmarks from the handles...
```

## Next Steps for WINA-596

Based on these proof-of-concept tests, potential approaches for solving the initial bookmark issue:

1. **Query Latest Event**: Use `EvtQuery` with reverse direction to get the most recent event, create initial bookmark from it
2. **Bookmark Format Analysis**: Understand the exact XML structure to potentially create synthetic bookmarks
3. **Event Log Metadata**: Query event log properties to get latest record ID and construct bookmark XML

## Troubleshooting

### Common Issues

- **Access Denied**: Some event logs require administrative privileges
- **No Events Available**: Some channels may be empty or have restrictive queries
- **Handle Cleanup**: Always close event record handles to prevent resource leaks

### Debug Output

The tests provide verbose output showing:
- Event handle values
- Complete bookmark XML
- Extracted attributes
- Error conditions and handling 