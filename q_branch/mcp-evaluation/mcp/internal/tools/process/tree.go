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

// GetProcessTreeInput defines input parameters
type GetProcessTreeInput struct {
	RootPID int `json:"root_pid" jsonschema:"Optional root PID to start tree from (default: show all processes)"`
}

// ProcessNode represents a node in the process tree
type ProcessNode struct {
	PID   int    `json:"pid"`
	PPID  int    `json:"ppid"`
	Name  string `json:"name"`
	State string `json:"state"`
	Level int    `json:"level"` // Tree depth level
}

// GetProcessTreeOutput contains process tree information (flattened)
type GetProcessTreeOutput struct {
	Processes []ProcessNode `json:"processes,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// GetProcessTreeTool provides process tree information
type GetProcessTreeTool struct{}

// NewGetProcessTreeTool creates a new process tree tool
func NewGetProcessTreeTool() *GetProcessTreeTool {
	return &GetProcessTreeTool{}
}

// Handler implements the process tree tool
func (t *GetProcessTreeTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetProcessTreeInput,
) (*mcp.CallToolResult, GetProcessTreeOutput, error) {
	log.Printf("[get_process_tree] Building process tree (root_pid: %d)", input.RootPID)

	// Read all processes from /proc
	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		return &mcp.CallToolResult{}, GetProcessTreeOutput{
			Error: fmt.Sprintf("failed to read /proc: %v", err),
		}, nil
	}

	// Map of PID -> ProcessNode
	processMap := make(map[int]ProcessNode)

	// Collect all process information
	for _, entry := range procEntries {
		if !entry.IsDir() {
			continue
		}

		// Try to parse directory name as PID
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Read process status
		statusPath := fmt.Sprintf("/proc/%d/status", pid)
		name, state, ppid := parseProcessStatus(statusPath)

		if name == "" {
			continue
		}

		node := ProcessNode{
			PID:   pid,
			PPID:  ppid,
			Name:  name,
			State: state,
			Level: 0, // Will be computed later
		}

		processMap[pid] = node
	}

	// Compute tree levels
	for pid := range processMap {
		computeLevel(pid, processMap)
	}

	// Build result: either from specific root or all processes
	var result []ProcessNode

	if input.RootPID != 0 {
		// Return subtree starting from specific PID
		if _, exists := processMap[input.RootPID]; !exists {
			return &mcp.CallToolResult{}, GetProcessTreeOutput{
				Error: fmt.Sprintf("PID %d not found", input.RootPID),
			}, nil
		}

		// Collect all descendants
		for _, node := range processMap {
			if node.PID == input.RootPID || isDescendantOf(node.PID, input.RootPID, processMap) {
				result = append(result, node)
			}
		}
	} else {
		// Return all processes
		for _, node := range processMap {
			result = append(result, node)
		}
	}

	log.Printf("[get_process_tree] Built tree with %d processes", len(result))

	return &mcp.CallToolResult{}, GetProcessTreeOutput{
		Processes: result,
	}, nil
}

// parseProcessStatus reads /proc/[pid]/status and extracts name, state, and ppid
func parseProcessStatus(statusPath string) (name string, state string, ppid int) {
	file, err := os.Open(statusPath)
	if err != nil {
		return "", "", 0
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
		} else if strings.HasPrefix(line, "PPid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ppid, _ = strconv.Atoi(fields[1])
			}
		}

		// Exit early if we have everything
		if name != "" && state != "" && ppid != 0 {
			break
		}
	}

	return name, state, ppid
}

// computeLevel calculates the depth level in the process tree
func computeLevel(pid int, processMap map[int]ProcessNode) int {
	node, exists := processMap[pid]
	if !exists {
		return 0
	}

	if node.Level != 0 {
		return node.Level
	}

	if node.PPID == 0 || node.PPID == pid {
		node.Level = 0
		processMap[pid] = node
		return 0
	}

	level := computeLevel(node.PPID, processMap) + 1
	node.Level = level
	processMap[pid] = node
	return level
}

// isDescendantOf checks if pid is a descendant of ancestorPID
func isDescendantOf(pid int, ancestorPID int, processMap map[int]ProcessNode) bool {
	if pid == ancestorPID {
		return false
	}

	current := pid
	visited := make(map[int]bool)

	for {
		if visited[current] {
			return false // Cycle detected
		}
		visited[current] = true

		node, exists := processMap[current]
		if !exists {
			return false
		}

		if node.PPID == ancestorPID {
			return true
		}

		if node.PPID == 0 || node.PPID == current {
			return false
		}

		current = node.PPID
	}
}

// Register registers the tool with the MCP server
func (t *GetProcessTreeTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_process_tree",
		Description: "Get hierarchical process tree showing parent-child relationships. Optionally filter to subtree starting from specific PID. Builds tree from /proc/[pid]/status files.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
