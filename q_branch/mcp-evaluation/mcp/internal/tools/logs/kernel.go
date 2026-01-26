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

// GetKernelMessagesInput defines input parameters
type GetKernelMessagesInput struct {
	Lines int    `json:"lines" jsonschema:"Number of recent lines to read (default: 500, max: 1000)"`
	Level string `json:"level" jsonschema:"Filter by level: emerg, alert, crit, err, warn, notice, info, debug"`
}

// KernelMessage represents a kernel log message
type KernelMessage struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// GetKernelMessagesOutput contains kernel messages
type GetKernelMessagesOutput struct {
	Messages []KernelMessage `json:"messages,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// GetKernelMessagesTool provides kernel log messages
type GetKernelMessagesTool struct{}

// NewGetKernelMessagesTool creates a new kernel messages tool
func NewGetKernelMessagesTool() *GetKernelMessagesTool {
	return &GetKernelMessagesTool{}
}

// Handler implements the kernel messages tool
func (t *GetKernelMessagesTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetKernelMessagesInput,
) (*mcp.CallToolResult, GetKernelMessagesOutput, error) {
	log.Printf("[get_kernel_messages] Reading kernel messages")

	// Set defaults
	lines := input.Lines
	if lines == 0 {
		lines = 500
	}
	if lines > 1000 {
		lines = 1000
	}

	// Try reading from dmesg file first, fallback to /var/log/dmesg or /var/log/kern.log
	var messages []KernelMessage
	var err error

	// Try dmesg command output or /var/log/dmesg
	messages, err = readDmesgLog(lines, input.Level)
	if err != nil {
		log.Printf("[get_kernel_messages] Failed to read dmesg: %v, trying /var/log/kern.log", err)
		messages, err = readKernLog(lines, input.Level)
		if err != nil {
			return &mcp.CallToolResult{}, GetKernelMessagesOutput{
				Error: fmt.Sprintf("failed to read kernel messages: %v", err),
			}, nil
		}
	}

	log.Printf("[get_kernel_messages] Found %d kernel messages", len(messages))

	return &mcp.CallToolResult{}, GetKernelMessagesOutput{
		Messages: messages,
	}, nil
}

// readDmesgLog reads from /var/log/dmesg
func readDmesgLog(maxLines int, levelFilter string) ([]KernelMessage, error) {
	file, err := os.Open("/var/log/dmesg")
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

	var messages []KernelMessage
	for _, line := range allLines[startIdx:] {
		msg := parseKernelMessage(line, levelFilter)
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	return messages, nil
}

// readKernLog reads from /var/log/kern.log
func readKernLog(maxLines int, levelFilter string) ([]KernelMessage, error) {
	file, err := os.Open("/var/log/kern.log")
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

	var messages []KernelMessage
	for _, line := range allLines[startIdx:] {
		msg := parseKernelMessage(line, levelFilter)
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	return messages, nil
}

// parseKernelMessage parses a kernel log line
func parseKernelMessage(line string, levelFilter string) *KernelMessage {
	if line == "" {
		return nil
	}

	// Parse level from message
	level := "info" // default
	for _, lvl := range []string{"emerg", "alert", "crit", "err", "warn", "notice", "info", "debug"} {
		if strings.Contains(strings.ToLower(line), lvl) {
			level = lvl
			break
		}
	}

	// Filter by level if specified
	if levelFilter != "" && level != levelFilter {
		return nil
	}

	return &KernelMessage{
		Level:   level,
		Message: line,
	}
}

// Register registers the tool with the MCP server
func (t *GetKernelMessagesTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_kernel_messages",
		Description: "Get recent kernel log messages (dmesg) with optional level filtering. Reads from /var/log/dmesg or /var/log/kern.log. Limited to 1000 lines.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
