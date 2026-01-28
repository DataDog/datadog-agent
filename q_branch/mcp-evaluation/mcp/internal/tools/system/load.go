package system

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetLoadHistoryInput defines input parameters
type GetLoadHistoryInput struct{}

// GetLoadHistoryOutput contains load average information
type GetLoadHistoryOutput struct {
	Load1Min         float64 `json:"load_1min"`
	Load5Min         float64 `json:"load_5min"`
	Load15Min        float64 `json:"load_15min"`
	RunningProcesses int     `json:"running_processes"`
	TotalProcesses   int     `json:"total_processes"`
	LastPID          int     `json:"last_pid"`
	Error            string  `json:"error,omitempty"`
}

// GetLoadHistoryTool provides system load average information
type GetLoadHistoryTool struct{}

// NewGetLoadHistoryTool creates a new load history tool
func NewGetLoadHistoryTool() *GetLoadHistoryTool {
	return &GetLoadHistoryTool{}
}

// Handler implements the load history tool
func (t *GetLoadHistoryTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetLoadHistoryInput,
) (*mcp.CallToolResult, GetLoadHistoryOutput, error) {
	log.Printf("[get_load_history] Reading system load averages")

	// Read /proc/loadavg
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return &mcp.CallToolResult{}, GetLoadHistoryOutput{
			Error: fmt.Sprintf("failed to open /proc/loadavg: %v", err),
		}, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return &mcp.CallToolResult{}, GetLoadHistoryOutput{
			Error: "failed to read /proc/loadavg",
		}, nil
	}

	// Format: 0.52 0.58 0.59 3/602 12345
	// load1 load5 load15 running/total lastPID
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 {
		return &mcp.CallToolResult{}, GetLoadHistoryOutput{
			Error: "unexpected format in /proc/loadavg",
		}, nil
	}

	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return &mcp.CallToolResult{}, GetLoadHistoryOutput{
			Error: fmt.Sprintf("failed to parse load1: %v", err),
		}, nil
	}

	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return &mcp.CallToolResult{}, GetLoadHistoryOutput{
			Error: fmt.Sprintf("failed to parse load5: %v", err),
		}, nil
	}

	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return &mcp.CallToolResult{}, GetLoadHistoryOutput{
			Error: fmt.Sprintf("failed to parse load15: %v", err),
		}, nil
	}

	// Parse running/total processes
	processParts := strings.Split(fields[3], "/")
	running, _ := strconv.Atoi(processParts[0])
	total, _ := strconv.Atoi(processParts[1])

	lastPID, _ := strconv.Atoi(fields[4])

	log.Printf("[get_load_history] Load averages: 1min=%.2f, 5min=%.2f, 15min=%.2f, running=%d/%d",
		load1, load5, load15, running, total)

	return &mcp.CallToolResult{}, GetLoadHistoryOutput{
		Load1Min:         load1,
		Load5Min:         load5,
		Load15Min:        load15,
		RunningProcesses: running,
		TotalProcesses:   total,
		LastPID:          lastPID,
	}, nil
}

// Register registers the tool with the MCP server
func (t *GetLoadHistoryTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_load_history",
		Description: "Get system load averages (1, 5, and 15 minutes) and process counts. Reads from /proc/loadavg.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
