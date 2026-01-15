package system

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetDiskUsageInput defines the input schema
type GetDiskUsageInput struct {
	Path string `json:"path,omitempty" jsonschema:"Path to check disk usage for (default: /)"`
}

// DiskInfo represents disk usage for a single mount point
type DiskInfo struct {
	Path        string  `json:"path"`
	TotalGB     float64 `json:"total_gb"`
	UsedGB      float64 `json:"used_gb"`
	AvailableGB float64 `json:"available_gb"`
	UsedPercent float64 `json:"used_percent"`
}

// GetDiskUsageOutput defines the output structure
type GetDiskUsageOutput struct {
	Disk  *DiskInfo `json:"disk,omitempty"`
	Error string    `json:"error,omitempty"`
}

// GetDiskUsageTool reads disk usage information
type GetDiskUsageTool struct{}

func NewGetDiskUsageTool() *GetDiskUsageTool {
	return &GetDiskUsageTool{}
}

func (t *GetDiskUsageTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetDiskUsageInput,
) (
	*mcp.CallToolResult,
	GetDiskUsageOutput,
	error,
) {
	path := input.Path
	if path == "" {
		path = "/"
	}

	log.Printf("[get_disk_usage] Checking disk usage for: %s", path)

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return &mcp.CallToolResult{}, GetDiskUsageOutput{
			Error: fmt.Sprintf("failed to stat %s: %v", path, err),
		}, nil
	}

	// Calculate sizes in GB
	blockSize := float64(stat.Bsize)
	totalGB := float64(stat.Blocks) * blockSize / (1024 * 1024 * 1024)
	availableGB := float64(stat.Bavail) * blockSize / (1024 * 1024 * 1024)
	usedGB := totalGB - availableGB
	usedPercent := 0.0
	if totalGB > 0 {
		usedPercent = (usedGB / totalGB) * 100
	}

	disk := DiskInfo{
		Path:        path,
		TotalGB:     totalGB,
		UsedGB:      usedGB,
		AvailableGB: availableGB,
		UsedPercent: usedPercent,
	}

	log.Printf("[get_disk_usage] %s: %.1f/%.1f GB used (%.1f%%)", path, usedGB, totalGB, usedPercent)

	return &mcp.CallToolResult{}, GetDiskUsageOutput{Disk: &disk}, nil
}

func (t *GetDiskUsageTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_disk_usage",
		Description: "Get disk space usage for a path (default: root filesystem). Returns total, used, available space in GB and usage percentage.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
