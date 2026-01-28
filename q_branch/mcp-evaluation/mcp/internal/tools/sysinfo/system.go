package sysinfo

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetSystemInfoInput defines the input schema
type GetSystemInfoInput struct{}

// GetSystemInfoOutput defines the output structure
type GetSystemInfoOutput struct {
	Hostname       string  `json:"hostname"`
	OS             string  `json:"os"`
	Architecture   string  `json:"architecture"`
	KernelVersion  string  `json:"kernel_version,omitempty"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	UptimeReadable string  `json:"uptime_readable"`
	Error          string  `json:"error,omitempty"`
}

// GetSystemInfoTool gets system information
type GetSystemInfoTool struct{}

func NewGetSystemInfoTool() *GetSystemInfoTool {
	return &GetSystemInfoTool{}
}

func (t *GetSystemInfoTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetSystemInfoInput,
) (
	*mcp.CallToolResult,
	GetSystemInfoOutput,
	error,
) {
	log.Printf("[get_system_info] Getting system information")

	output := GetSystemInfoOutput{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err == nil {
		output.Hostname = hostname
	}

	// Get kernel version from /proc/version
	versionData, err := os.ReadFile("/proc/version")
	if err == nil {
		output.KernelVersion = strings.TrimSpace(string(versionData))
	}

	// Get uptime from /proc/uptime
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return &mcp.CallToolResult{}, GetSystemInfoOutput{
			Error: fmt.Sprintf("failed to read /proc/uptime: %v", err),
		}, nil
	}

	fields := strings.Fields(string(uptimeData))
	if len(fields) > 0 {
		uptimeFloat, _ := strconv.ParseFloat(fields[0], 64)
		uptimeSeconds := int64(uptimeFloat)
		output.UptimeSeconds = uptimeSeconds

		// Format as readable string
		days := uptimeSeconds / 86400
		hours := (uptimeSeconds % 86400) / 3600
		minutes := (uptimeSeconds % 3600) / 60
		seconds := uptimeSeconds % 60

		if days > 0 {
			output.UptimeReadable = fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
		} else if hours > 0 {
			output.UptimeReadable = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
		} else if minutes > 0 {
			output.UptimeReadable = fmt.Sprintf("%dm %ds", minutes, seconds)
		} else {
			output.UptimeReadable = fmt.Sprintf("%ds", seconds)
		}
	}

	log.Printf("[get_system_info] Hostname: %s, OS: %s, Uptime: %s",
		output.Hostname, output.OS, output.UptimeReadable)

	return &mcp.CallToolResult{}, output, nil
}

func (t *GetSystemInfoTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_system_info",
		Description: "Get system overview: hostname, OS, architecture, kernel version, and uptime. Returns key system identification info.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
