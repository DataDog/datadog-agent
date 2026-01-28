package kernel

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetServiceStatusInput defines input parameters
type GetServiceStatusInput struct {
	ServiceName string `json:"service_name" jsonschema:"Name of the systemd service (e.g. 'sshd', 'nginx.service')"`
}

// ServiceStatus represents systemd service status
type ServiceStatus struct {
	Name        string  `json:"name"`
	Active      string  `json:"active"`
	Enabled     string  `json:"enabled"`
	PID         int     `json:"pid,omitempty"`
	MemoryMB    float64 `json:"memory_mb,omitempty"`
	CPUPercent  float64 `json:"cpu_percent,omitempty"`
	Restarts    int     `json:"restarts,omitempty"`
	Description string  `json:"description,omitempty"`
}

// GetServiceStatusOutput contains service status
type GetServiceStatusOutput struct {
	Status ServiceStatus `json:"status,omitempty"`
	Error  string        `json:"error,omitempty"`
}

// GetServiceStatusTool provides systemd service status
type GetServiceStatusTool struct{}

// NewGetServiceStatusTool creates a new service status tool
func NewGetServiceStatusTool() *GetServiceStatusTool {
	return &GetServiceStatusTool{}
}

// Handler implements the service status tool
func (t *GetServiceStatusTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetServiceStatusInput,
) (*mcp.CallToolResult, GetServiceStatusOutput, error) {
	log.Printf("[get_service_status] Getting status for service: %s", input.ServiceName)

	// Check if systemctl is available
	if _, err := exec.LookPath("systemctl"); err != nil {
		return &mcp.CallToolResult{}, GetServiceStatusOutput{
			Error: "systemctl command not available (systemd not present)",
		}, nil
	}

	// Validate service name
	if !isValidServiceName(input.ServiceName) {
		return &mcp.CallToolResult{}, GetServiceStatusOutput{
			Error: "invalid service name",
		}, nil
	}

	// Execute systemctl status with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "systemctl", "status", input.ServiceName)
	output, err := cmd.CombinedOutput()

	// Note: systemctl status returns non-zero exit code for inactive services,
	// so we don't treat non-zero exit as an error

	// Parse status output
	status := parseSystemctlStatus(input.ServiceName, string(output))

	// Get enabled status separately
	enabledCmd := exec.CommandContext(cmdCtx, "systemctl", "is-enabled", input.ServiceName)
	enabledOutput, _ := enabledCmd.Output()
	status.Enabled = strings.TrimSpace(string(enabledOutput))

	log.Printf("[get_service_status] Service %s: active=%s, enabled=%s, pid=%d",
		input.ServiceName, status.Active, status.Enabled, status.PID)

	if err != nil && status.Active == "" {
		return &mcp.CallToolResult{}, GetServiceStatusOutput{
			Error: fmt.Sprintf("failed to get service status: %v", err),
		}, nil
	}

	return &mcp.CallToolResult{}, GetServiceStatusOutput{
		Status: status,
	}, nil
}

// parseSystemctlStatus parses systemctl status output
func parseSystemctlStatus(serviceName, output string) ServiceStatus {
	status := ServiceStatus{
		Name: serviceName,
	}

	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse Active status
		if strings.HasPrefix(line, "Active:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				status.Active = parts[1]
			}
		}

		// Parse Main PID
		if strings.Contains(line, "Main PID:") {
			re := regexp.MustCompile(`Main PID:\s+(\d+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) >= 2 {
				status.PID, _ = strconv.Atoi(matches[1])
			}
		}

		// Parse Memory
		if strings.Contains(line, "Memory:") {
			re := regexp.MustCompile(`Memory:\s+([\d.]+)([KMG])`)
			matches := re.FindStringSubmatch(line)
			if len(matches) >= 3 {
				value, _ := strconv.ParseFloat(matches[1], 64)
				unit := matches[2]

				switch unit {
				case "K":
					status.MemoryMB = value / 1024
				case "M":
					status.MemoryMB = value
				case "G":
					status.MemoryMB = value * 1024
				}
			}
		}

		// Parse CPU
		if strings.Contains(line, "CPU:") {
			re := regexp.MustCompile(`CPU:\s+([\d.]+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) >= 2 {
				status.CPUPercent, _ = strconv.ParseFloat(matches[1], 64)
			}
		}

		// Parse Description
		if strings.HasPrefix(line, "Loaded:") {
			// Description is often on the next line after Loaded
			// Try to extract from this line if it contains it
			if idx := strings.Index(line, ";"); idx != -1 {
				descPart := line[idx+1:]
				descPart = strings.TrimSpace(descPart)
				if descPart != "" {
					status.Description = descPart
				}
			}
		}
	}

	return status
}

// isValidServiceName checks if service name is valid
func isValidServiceName(name string) bool {
	// Basic validation: alphanumeric, dash, underscore, dot, @
	// Should not contain shell metacharacters
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '@') {
			return false
		}
	}

	// Length check
	return len(name) > 0 && len(name) < 256
}

// Register registers the tool with the MCP server
func (t *GetServiceStatusTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_service_status",
		Description: "Get systemd service status including active state, enabled state, PID, memory, and CPU usage. Executes systemctl with strict input validation.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
