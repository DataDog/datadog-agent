package logs

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetSystemEventsInput defines input parameters
type GetSystemEventsInput struct {
	Lines     int    `json:"lines" jsonschema:"Number of recent lines to read (default: 500, max: 1000)"`
	EventType string `json:"event_type" jsonschema:"Filter by type: auth, security, system (default: all)"`
}

// SystemEvent represents a system log event
type SystemEvent struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
}

// GetSystemEventsOutput contains system events
type GetSystemEventsOutput struct {
	Events []SystemEvent `json:"events,omitempty"`
	Error  string        `json:"error,omitempty"`
}

// GetSystemEventsTool provides system log events
type GetSystemEventsTool struct{}

// NewGetSystemEventsTool creates a new system events tool
func NewGetSystemEventsTool() *GetSystemEventsTool {
	return &GetSystemEventsTool{}
}

// Handler implements the system events tool
func (t *GetSystemEventsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetSystemEventsInput,
) (*mcp.CallToolResult, GetSystemEventsOutput, error) {
	log.Printf("[get_system_events] Reading system events (type: %s)", input.EventType)

	// Set defaults
	lines := input.Lines
	if lines == 0 {
		lines = 500
	}
	if lines > 1000 {
		lines = 1000
	}

	var events []SystemEvent

	// Read different log files based on event type
	switch input.EventType {
	case "auth":
		authEvents, err := readLogFile("/var/log/auth.log", lines, "auth")
		if err != nil {
			log.Printf("[get_system_events] Failed to read auth.log: %v", err)
		} else {
			events = append(events, authEvents...)
		}

	case "security":
		secEvents, err := readLogFile("/var/log/secure", lines, "security")
		if err != nil {
			log.Printf("[get_system_events] Failed to read secure: %v", err)
		} else {
			events = append(events, secEvents...)
		}

	case "system":
		sysEvents, err := readLogFile("/var/log/syslog", lines, "system")
		if err != nil {
			// Try /var/log/messages as fallback
			sysEvents, err = readLogFile("/var/log/messages", lines, "system")
			if err != nil {
				log.Printf("[get_system_events] Failed to read system logs: %v", err)
			} else {
				events = append(events, sysEvents...)
			}
		} else {
			events = append(events, sysEvents...)
		}

	default:
		// Read all available logs
		authEvents, _ := readLogFile("/var/log/auth.log", lines/3, "auth")
		events = append(events, authEvents...)

		secEvents, _ := readLogFile("/var/log/secure", lines/3, "security")
		events = append(events, secEvents...)

		sysEvents, err := readLogFile("/var/log/syslog", lines/3, "system")
		if err != nil {
			sysEvents, _ = readLogFile("/var/log/messages", lines/3, "system")
		}
		events = append(events, sysEvents...)
	}

	// Limit to requested number of lines
	if len(events) > lines {
		events = events[len(events)-lines:]
	}

	log.Printf("[get_system_events] Found %d system events", len(events))

	return &mcp.CallToolResult{}, GetSystemEventsOutput{
		Events: events,
	}, nil
}

// readLogFile reads recent lines from a log file
func readLogFile(path string, maxLines int, eventType string) ([]SystemEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Take last N lines
	startIdx := 0
	if len(allLines) > maxLines {
		startIdx = len(allLines) - maxLines
	}

	var events []SystemEvent
	for _, line := range allLines[startIdx:] {
		if line == "" {
			continue
		}

		// Try to parse timestamp (basic parsing for common syslog format)
		timestamp := parseLogTimestamp(line)

		events = append(events, SystemEvent{
			Timestamp: timestamp,
			Type:      eventType,
			Message:   line,
		})
	}

	return events, nil
}

// parseLogTimestamp attempts to parse timestamp from syslog-style log line
func parseLogTimestamp(line string) time.Time {
	// Common syslog format: "Jan 23 12:34:56 hostname message"
	// We'll do basic parsing
	fields := strings.Fields(line)
	if len(fields) >= 3 {
		// Try to parse "Jan 23 12:34:56"
		timeStr := strings.Join(fields[:3], " ")

		// Add current year since syslog doesn't include it
		year := time.Now().Year()
		fullTimeStr := fmt.Sprintf("%s %d", timeStr, year)

		layouts := []string{
			"Jan 2 15:04:05 2006",
			"Jan  2 15:04:05 2006",
			"2006-01-02 15:04:05",
		}

		for _, layout := range layouts {
			if t, err := time.Parse(layout, fullTimeStr); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

// Register registers the tool with the MCP server
func (t *GetSystemEventsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_system_events",
		Description: "Get recent system log events from auth.log, secure, syslog, or messages. Filter by event type: auth, security, system. Limited to 1000 lines.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
