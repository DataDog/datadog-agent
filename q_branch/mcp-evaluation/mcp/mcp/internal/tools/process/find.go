package process

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FindProcessInput defines the input schema
type FindProcessInput struct {
	Name *string `json:"name,omitempty" jsonschema:"Process name to search for (partial match)"`
	User *string `json:"user,omitempty" jsonschema:"Username/UID to filter by"`
}

// FindProcessOutput defines the output structure
type FindProcessOutput struct {
	Processes []ProcessInfo `json:"processes,omitempty"`
	Count     int           `json:"count"`
	Error     string        `json:"error,omitempty"`
}

// FindProcessTool finds processes by name or user
type FindProcessTool struct{}

func NewFindProcessTool() *FindProcessTool {
	return &FindProcessTool{}
}

func (t *FindProcessTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input FindProcessInput,
) (
	*mcp.CallToolResult,
	FindProcessOutput,
	error,
) {
	if input.Name == nil && input.User == nil {
		return &mcp.CallToolResult{}, FindProcessOutput{
			Error: "at least one of 'name' or 'user' must be specified",
		}, nil
	}

	searchName := ""
	if input.Name != nil {
		searchName = strings.ToLower(*input.Name)
	}

	searchUID := -1
	if input.User != nil {
		searchUID, _ = strconv.Atoi(*input.User)
	}

	log.Printf("[find_process] Searching for processes (name: %q, user: %v)", searchName, input.User)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return &mcp.CallToolResult{}, FindProcessOutput{
			Error: fmt.Sprintf("failed to read /proc: %v", err),
		}, nil
	}

	var matches []ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		statusPath := filepath.Join("/proc", entry.Name(), "status")
		statusData, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}

		proc := ProcessInfo{PID: pid}
		uid := -1
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
			case "Uid":
				fields := strings.Fields(value)
				if len(fields) > 0 {
					uid, _ = strconv.Atoi(fields[0])
				}
			case "VmRSS":
				fields := strings.Fields(value)
				if len(fields) > 0 {
					vmrssKB, _ := strconv.ParseInt(fields[0], 10, 64)
					proc.VMRSS = vmrssKB / 1024
				}
			}
		}

		// Apply filters
		nameMatch := searchName == "" || strings.Contains(strings.ToLower(proc.Name), searchName)
		userMatch := searchUID == -1 || uid == searchUID

		if nameMatch && userMatch {
			// Get cmdline for better matching
			cmdlineData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
			if err == nil {
				cmdline := strings.ReplaceAll(string(cmdlineData), "\x00", " ")
				if searchName != "" && !strings.Contains(strings.ToLower(cmdline), searchName) {
					nameMatch = strings.Contains(strings.ToLower(proc.Name), searchName)
				}
			}

			if nameMatch && userMatch {
				matches = append(matches, proc)
			}
		}
	}

	log.Printf("[find_process] Found %d matching processes", len(matches))

	return &mcp.CallToolResult{}, FindProcessOutput{
		Processes: matches,
		Count:     len(matches),
	}, nil
}

func (t *FindProcessTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "find_process",
		Description: "Find processes by name (partial match) or user/UID. Returns matching processes with basic info.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
