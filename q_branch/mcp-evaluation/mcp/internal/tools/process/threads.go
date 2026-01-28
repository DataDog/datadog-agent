package process

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

// GetProcessThreadsInput defines input parameters
type GetProcessThreadsInput struct {
	PID int `json:"pid" jsonschema:"Process ID to get threads for"`
}

// ProcessThread represents a single thread
type ProcessThread struct {
	TID   int    `json:"tid"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// GetProcessThreadsOutput contains process thread information
type GetProcessThreadsOutput struct {
	PID     int             `json:"pid"`
	Threads []ProcessThread `json:"threads,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// GetProcessThreadsTool provides process thread information
type GetProcessThreadsTool struct{}

// NewGetProcessThreadsTool creates a new process threads tool
func NewGetProcessThreadsTool() *GetProcessThreadsTool {
	return &GetProcessThreadsTool{}
}

// Handler implements the process threads tool
func (t *GetProcessThreadsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetProcessThreadsInput,
) (*mcp.CallToolResult, GetProcessThreadsOutput, error) {
	log.Printf("[get_process_threads] Getting threads for PID %d", input.PID)

	// List /proc/[pid]/task directory
	taskPath := fmt.Sprintf("/proc/%d/task", input.PID)
	entries, err := os.ReadDir(taskPath)
	if err != nil {
		return &mcp.CallToolResult{}, GetProcessThreadsOutput{
			PID:   input.PID,
			Error: fmt.Sprintf("failed to read %s: %v", taskPath, err),
		}, nil
	}

	var threads []ProcessThread

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Parse TID from directory name
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Read thread status file
		statusPath := fmt.Sprintf("%s/%d/status", taskPath, tid)
		name, state := parseThreadStatus(statusPath)

		threads = append(threads, ProcessThread{
			TID:   tid,
			Name:  name,
			State: state,
		})

		// Limit to 1000 threads
		if len(threads) >= 1000 {
			break
		}
	}

	log.Printf("[get_process_threads] Found %d threads for PID %d", len(threads), input.PID)

	return &mcp.CallToolResult{}, GetProcessThreadsOutput{
		PID:     input.PID,
		Threads: threads,
	}, nil
}

// parseThreadStatus reads /proc/[pid]/task/[tid]/status and extracts name and state
func parseThreadStatus(statusPath string) (name string, state string) {
	file, err := os.Open(statusPath)
	if err != nil {
		return "unknown", "?"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Name:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				name = fields[1]
			}
		} else if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				state = fields[1]
			}
		}

		// Exit early if we have both
		if name != "" && state != "" {
			break
		}
	}

	if name == "" {
		name = "unknown"
	}
	if state == "" {
		state = "?"
	}

	return name, state
}

// Register registers the tool with the MCP server
func (t *GetProcessThreadsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_process_threads",
		Description: "Get list of threads for a specific process including TID, name, and state. Lists /proc/[pid]/task and reads each thread's status.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
