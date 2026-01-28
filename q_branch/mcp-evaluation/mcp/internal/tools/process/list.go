package process

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListProcessesInput defines the input schema
type ListProcessesInput struct {
	Limit *int `json:"limit,omitempty" jsonschema:"Maximum number of processes to return (default: 50, max: 500)"`
}

// ProcessInfo represents information about a single process
type ProcessInfo struct {
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	State   string `json:"state"`
	PPID    int    `json:"ppid"`
	Threads int    `json:"threads"`
	VMRSS   int64  `json:"vmrss_mb"` // Resident memory in MB
	CPU     string `json:"cpu,omitempty"`
}

// ListProcessesOutput defines the output structure
type ListProcessesOutput struct {
	Processes []ProcessInfo `json:"processes,omitempty"`
	Count     int           `json:"count"`
	Error     string        `json:"error,omitempty"`
}

// ListProcessesTool lists running processes
type ListProcessesTool struct{}

func NewListProcessesTool() *ListProcessesTool {
	return &ListProcessesTool{}
}

func (t *ListProcessesTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input ListProcessesInput,
) (
	*mcp.CallToolResult,
	ListProcessesOutput,
	error,
) {
	limit := 50
	if input.Limit != nil {
		limit = *input.Limit
		if limit > 500 {
			limit = 500
		}
		if limit < 1 {
			limit = 1
		}
	}

	log.Printf("[list_processes] Listing processes (limit: %d)", limit)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return &mcp.CallToolResult{}, ListProcessesOutput{
			Error: fmt.Sprintf("failed to read /proc: %v", err),
		}, nil
	}

	var processes []ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // Not a PID directory
		}

		statusPath := filepath.Join("/proc", entry.Name(), "status")
		statusData, err := os.ReadFile(statusPath)
		if err != nil {
			continue // Process may have exited
		}

		proc := ProcessInfo{PID: pid}
		lines := strings.Split(string(statusData), "\n")

		for _, line := range lines {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "Name":
				proc.Name = value
			case "State":
				proc.State = value
			case "PPid":
				proc.PPID, _ = strconv.Atoi(value)
			case "Threads":
				proc.Threads, _ = strconv.Atoi(value)
			case "VmRSS":
				// VmRSS is in kB
				fields := strings.Fields(value)
				if len(fields) > 0 {
					vmrssKB, _ := strconv.ParseInt(fields[0], 10, 64)
					proc.VMRSS = vmrssKB / 1024 // Convert to MB
				}
			}
		}

		processes = append(processes, proc)
	}

	// Sort by memory usage (descending)
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].VMRSS > processes[j].VMRSS
	})

	// Limit results
	if len(processes) > limit {
		processes = processes[:limit]
	}

	log.Printf("[list_processes] Found %d processes (showing top %d by memory)", len(processes), len(processes))

	return &mcp.CallToolResult{}, ListProcessesOutput{
		Processes: processes,
		Count:     len(processes),
	}, nil
}

func (t *ListProcessesTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "list_processes",
		Description: "List running processes with key metrics (PID, name, state, memory). Returns top N processes by memory usage. For specific process details, use get_process_info.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
