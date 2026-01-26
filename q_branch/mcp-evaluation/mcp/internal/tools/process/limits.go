package process

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetProcessLimitsInput defines input parameters
type GetProcessLimitsInput struct {
	PID int `json:"pid" jsonschema:"Process ID to get limits for"`
}

// ProcessLimit represents a single resource limit
type ProcessLimit struct {
	Resource     string `json:"resource"`
	SoftLimit    string `json:"soft_limit"`
	HardLimit    string `json:"hard_limit"`
	Units        string `json:"units"`
}

// GetProcessLimitsOutput contains process resource limits
type GetProcessLimitsOutput struct {
	PID    int            `json:"pid"`
	Limits []ProcessLimit `json:"limits,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// GetProcessLimitsTool provides process resource limits
type GetProcessLimitsTool struct{}

// NewGetProcessLimitsTool creates a new process limits tool
func NewGetProcessLimitsTool() *GetProcessLimitsTool {
	return &GetProcessLimitsTool{}
}

// Handler implements the process limits tool
func (t *GetProcessLimitsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetProcessLimitsInput,
) (*mcp.CallToolResult, GetProcessLimitsOutput, error) {
	log.Printf("[get_process_limits] Getting limits for PID %d", input.PID)

	// Read /proc/[pid]/limits
	limitsPath := fmt.Sprintf("/proc/%d/limits", input.PID)
	file, err := os.Open(limitsPath)
	if err != nil {
		return &mcp.CallToolResult{}, GetProcessLimitsOutput{
			PID:   input.PID,
			Error: fmt.Sprintf("failed to open %s: %v", limitsPath, err),
		}, nil
	}
	defer file.Close()

	var limits []ProcessLimit
	scanner := bufio.NewScanner(file)

	// Skip header line
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()

		// Parse limit lines - format varies but generally:
		// "Resource Name                     Soft Limit           Hard Limit           Units"
		// Split on multiple spaces to handle aligned columns
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Find where the numeric limits start by looking for "unlimited" or numbers
		// The resource name is everything before the limits
		var resourceParts []string
		var limitStart int

		for i, part := range parts {
			if part == "unlimited" || isNumericOrUnlimited(part) {
				limitStart = i
				break
			}
			resourceParts = append(resourceParts, part)
		}

		if limitStart == 0 || limitStart >= len(parts)-1 {
			continue
		}

		resource := strings.Join(resourceParts, " ")
		softLimit := parts[limitStart]
		hardLimit := parts[limitStart+1]

		// Units are optional and appear after hard limit
		units := ""
		if limitStart+2 < len(parts) {
			units = parts[limitStart+2]
		}

		limits = append(limits, ProcessLimit{
			Resource:  resource,
			SoftLimit: softLimit,
			HardLimit: hardLimit,
			Units:     units,
		})
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetProcessLimitsOutput{
			PID:   input.PID,
			Error: fmt.Sprintf("error reading %s: %v", limitsPath, err),
		}, nil
	}

	log.Printf("[get_process_limits] Found %d limits for PID %d", len(limits), input.PID)

	return &mcp.CallToolResult{}, GetProcessLimitsOutput{
		PID:    input.PID,
		Limits: limits,
	}, nil
}

// isNumericOrUnlimited checks if a string is a number or "unlimited"
func isNumericOrUnlimited(s string) bool {
	if s == "unlimited" {
		return true
	}
	// Check if first character is a digit
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		return true
	}
	return false
}

// Register registers the tool with the MCP server
func (t *GetProcessLimitsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_process_limits",
		Description: "Get resource limits for a specific process including max open files, max processes, stack size, etc. Reads from /proc/[pid]/limits.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
