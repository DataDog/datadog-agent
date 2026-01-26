package files

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetInodeUsageInput defines input parameters
type GetInodeUsageInput struct {
	Path string `json:"path" jsonschema:"Optional path to check specific filesystem (default: all filesystems)"`
}

// InodeInfo represents inode usage for a filesystem
type InodeInfo struct {
	MountPoint   string  `json:"mount_point"`
	Device       string  `json:"device"`
	FsType       string  `json:"fs_type"`
	TotalInodes  uint64  `json:"total_inodes"`
	FreeInodes   uint64  `json:"free_inodes"`
	UsedInodes   uint64  `json:"used_inodes"`
	PercentUsed  float64 `json:"percent_used"`
}

// GetInodeUsageOutput contains inode usage information
type GetInodeUsageOutput struct {
	Filesystems []InodeInfo `json:"filesystems,omitempty"`
	Error       string      `json:"error,omitempty"`
}

// GetInodeUsageTool provides filesystem inode usage information
type GetInodeUsageTool struct{}

// NewGetInodeUsageTool creates a new inode usage tool
func NewGetInodeUsageTool() *GetInodeUsageTool {
	return &GetInodeUsageTool{}
}

// Handler implements the inode usage tool
func (t *GetInodeUsageTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetInodeUsageInput,
) (*mcp.CallToolResult, GetInodeUsageOutput, error) {
	log.Printf("[get_inode_usage] Getting inode usage information")

	// Read /proc/mounts to get all mount points
	mountsFile, err := os.Open("/proc/mounts")
	if err != nil {
		return &mcp.CallToolResult{}, GetInodeUsageOutput{
			Error: fmt.Sprintf("failed to open /proc/mounts: %v", err),
		}, nil
	}
	defer mountsFile.Close()

	var filesystems []InodeInfo
	scanner := bufio.NewScanner(mountsFile)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		// Skip pseudo filesystems
		if strings.HasPrefix(device, "/dev") || fsType == "nfs" || fsType == "cifs" {
			// Include real filesystems
		} else {
			continue
		}

		// If a specific path was requested, only check that filesystem
		if input.Path != "" && mountPoint != input.Path {
			continue
		}

		// Get filesystem statistics
		var stat syscall.Statfs_t
		err := syscall.Statfs(mountPoint, &stat)
		if err != nil {
			log.Printf("[get_inode_usage] Failed to statfs %s: %v", mountPoint, err)
			continue
		}

		totalInodes := stat.Files
		freeInodes := stat.Ffree
		usedInodes := totalInodes - freeInodes

		percentUsed := 0.0
		if totalInodes > 0 {
			percentUsed = float64(usedInodes) * 100.0 / float64(totalInodes)
		}

		filesystems = append(filesystems, InodeInfo{
			MountPoint:  mountPoint,
			Device:      device,
			FsType:      fsType,
			TotalInodes: totalInodes,
			FreeInodes:  freeInodes,
			UsedInodes:  usedInodes,
			PercentUsed: percentUsed,
		})

		// Limit to 100 filesystems
		if len(filesystems) >= 100 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetInodeUsageOutput{
			Error: fmt.Sprintf("error reading /proc/mounts: %v", err),
		}, nil
	}

	log.Printf("[get_inode_usage] Found %d filesystems with inode information", len(filesystems))

	return &mcp.CallToolResult{}, GetInodeUsageOutput{
		Filesystems: filesystems,
	}, nil
}

// Register registers the tool with the MCP server
func (t *GetInodeUsageTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_inode_usage",
		Description: "Get inode usage statistics for filesystems. Shows total, free, and used inodes with percentage. Useful for diagnosing 'No space left on device' errors caused by inode exhaustion.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
