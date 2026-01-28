package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetSystemJournalInput defines input parameters
type GetSystemJournalInput struct {
	Lines    int    `json:"lines" jsonschema:"Number of recent lines to read (default: 500, max: 1000)"`
	Unit     string `json:"unit" jsonschema:"Filter by systemd unit name (e.g. 'sshd.service')"`
	Priority string `json:"priority" jsonschema:"Filter by priority: emerg, alert, crit, err, warning, notice, info, debug"`
	Since    string `json:"since" jsonschema:"Show entries since time (e.g. '1 hour ago', 'yesterday', '2024-01-01')"`
}

// JournalEntry represents a systemd journal entry
type JournalEntry struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Priority  string    `json:"priority"`
	Unit      string    `json:"unit,omitempty"`
	Message   string    `json:"message"`
}

// GetSystemJournalOutput contains journal entries
type GetSystemJournalOutput struct {
	Entries []JournalEntry `json:"entries,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// GetSystemJournalTool provides systemd journal access
type GetSystemJournalTool struct{}

// NewGetSystemJournalTool creates a new system journal tool
func NewGetSystemJournalTool() *GetSystemJournalTool {
	return &GetSystemJournalTool{}
}

// Handler implements the system journal tool
func (t *GetSystemJournalTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetSystemJournalInput,
) (*mcp.CallToolResult, GetSystemJournalOutput, error) {
	log.Printf("[get_system_journal] Reading systemd journal")

	// Check if journalctl is available
	if _, err := exec.LookPath("journalctl"); err != nil {
		return &mcp.CallToolResult{}, GetSystemJournalOutput{
			Error: "journalctl command not available (systemd not present)",
		}, nil
	}

	// Set defaults
	lines := input.Lines
	if lines == 0 {
		lines = 500
	}
	if lines > 1000 {
		lines = 1000
	}

	// Build journalctl command
	args := []string{
		"-n", fmt.Sprintf("%d", lines),
		"--output=json",
		"--no-pager",
	}

	// Add unit filter if specified
	if input.Unit != "" {
		// Validate unit name (basic sanitization)
		if !isValidUnitName(input.Unit) {
			return &mcp.CallToolResult{}, GetSystemJournalOutput{
				Error: "invalid unit name",
			}, nil
		}
		args = append(args, "-u", input.Unit)
	}

	// Add priority filter if specified
	if input.Priority != "" {
		// Validate priority
		validPriorities := map[string]bool{
			"emerg": true, "alert": true, "crit": true, "err": true,
			"warning": true, "notice": true, "info": true, "debug": true,
		}
		if !validPriorities[input.Priority] {
			return &mcp.CallToolResult{}, GetSystemJournalOutput{
				Error: "invalid priority level",
			}, nil
		}
		args = append(args, "-p", input.Priority)
	}

	// Add since filter if specified
	if input.Since != "" {
		// Validate since format (basic check)
		if !isValidSinceFormat(input.Since) {
			return &mcp.CallToolResult{}, GetSystemJournalOutput{
				Error: "invalid since format",
			}, nil
		}
		args = append(args, "--since", input.Since)
	}

	// Execute journalctl with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "journalctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &mcp.CallToolResult{}, GetSystemJournalOutput{
			Error: fmt.Sprintf("journalctl failed: %v", err),
		}, nil
	}

	// Parse JSON output
	entries := parseJournalJSON(output)

	log.Printf("[get_system_journal] Found %d journal entries", len(entries))

	return &mcp.CallToolResult{}, GetSystemJournalOutput{
		Entries: entries,
	}, nil
}

// parseJournalJSON parses journalctl JSON output
func parseJournalJSON(data []byte) []JournalEntry {
	var entries []JournalEntry

	// journalctl --output=json produces one JSON object per line
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		entry := JournalEntry{}

		// Parse timestamp (microseconds since epoch)
		if tsStr, ok := raw["__REALTIME_TIMESTAMP"].(string); ok {
			// Parse microseconds
			var us int64
			fmt.Sscanf(tsStr, "%d", &us)
			entry.Timestamp = time.Unix(0, us*1000)
		}

		// Parse priority
		if priority, ok := raw["PRIORITY"].(string); ok {
			entry.Priority = priorityToString(priority)
		}

		// Parse unit
		if unit, ok := raw["_SYSTEMD_UNIT"].(string); ok {
			entry.Unit = unit
		} else if unit, ok := raw["UNIT"].(string); ok {
			entry.Unit = unit
		}

		// Parse message
		if msg, ok := raw["MESSAGE"].(string); ok {
			entry.Message = msg
		}

		entries = append(entries, entry)
	}

	return entries
}

// priorityToString converts numeric priority to string
func priorityToString(priority string) string {
	priorities := map[string]string{
		"0": "emerg",
		"1": "alert",
		"2": "crit",
		"3": "err",
		"4": "warning",
		"5": "notice",
		"6": "info",
		"7": "debug",
	}

	if name, ok := priorities[priority]; ok {
		return name
	}
	return priority
}

// isValidUnitName checks if unit name is valid
func isValidUnitName(unit string) bool {
	// Basic validation: alphanumeric, dash, underscore, dot
	for _, c := range unit {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '@') {
			return false
		}
	}
	return len(unit) > 0 && len(unit) < 256
}

// isValidSinceFormat checks if since format is reasonable
func isValidSinceFormat(since string) bool {
	// Allow common formats: "1 hour ago", "yesterday", "2024-01-01", etc.
	// Basic validation to prevent injection
	if len(since) > 100 {
		return false
	}

	// Check for dangerous characters
	dangerous := []string{";", "|", "&", "`", "$", "(", ")", "<", ">"}
	for _, d := range dangerous {
		if strings.Contains(since, d) {
			return false
		}
	}

	return true
}

// Register registers the tool with the MCP server
func (t *GetSystemJournalTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_system_journal",
		Description: "Get systemd journal entries with filtering by unit, priority, and time. Executes journalctl with strict input validation. Limited to 1000 entries.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
