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

// GetFilesystemInfoInput defines input parameters
type GetFilesystemInfoInput struct{}

// FilesystemInfo represents filesystem mount information
type FilesystemInfo struct {
	MountPoint   string   `json:"mount_point"`
	Device       string   `json:"device"`
	FsType       string   `json:"fs_type"`
	TotalMB      int64    `json:"total_mb"`
	UsedMB       int64    `json:"used_mb"`
	AvailableMB  int64    `json:"available_mb"`
	UsedPercent  float64  `json:"used_percent"`
	MountOptions []string `json:"mount_options,omitempty"`
}

// GetFilesystemInfoOutput contains filesystem information
type GetFilesystemInfoOutput struct {
	Filesystems []FilesystemInfo `json:"filesystems,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// GetFilesystemInfoTool provides filesystem information
type GetFilesystemInfoTool struct{}

// NewGetFilesystemInfoTool creates a new filesystem info tool
func NewGetFilesystemInfoTool() *GetFilesystemInfoTool {
	return &GetFilesystemInfoTool{}
}

// Handler implements the filesystem info tool
func (t *GetFilesystemInfoTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetFilesystemInfoInput,
) (*mcp.CallToolResult, GetFilesystemInfoOutput, error) {
	log.Printf("[get_filesystem_info] Reading filesystem information")

	// Read /proc/mounts to get all mount points
	mountsFile, err := os.Open("/proc/mounts")
	if err != nil {
		return &mcp.CallToolResult{}, GetFilesystemInfoOutput{
			Error: fmt.Sprintf("failed to open /proc/mounts: %v", err),
		}, nil
	}
	defer mountsFile.Close()

	var filesystems []FilesystemInfo
	scanner := bufio.NewScanner(mountsFile)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]
		options := strings.Split(fields[3], ",")

		// Skip pseudo filesystems (keep only real storage)
		if !isRealFilesystem(device, fsType) {
			continue
		}

		// Get filesystem statistics
		var stat syscall.Statfs_t
		err := syscall.Statfs(mountPoint, &stat)
		if err != nil {
			log.Printf("[get_filesystem_info] Failed to statfs %s: %v", mountPoint, err)
			continue
		}

		// Calculate sizes in MB
		blockSize := uint64(stat.Bsize)
		totalBlocks := stat.Blocks
		freeBlocks := stat.Bfree
		availBlocks := stat.Bavail

		totalMB := int64(totalBlocks * blockSize / (1024 * 1024))
		availableMB := int64(availBlocks * blockSize / (1024 * 1024))
		usedMB := totalMB - int64(freeBlocks*blockSize/(1024*1024))

		usedPercent := 0.0
		if totalMB > 0 {
			usedPercent = float64(usedMB) * 100.0 / float64(totalMB)
		}

		filesystems = append(filesystems, FilesystemInfo{
			MountPoint:   mountPoint,
			Device:       device,
			FsType:       fsType,
			TotalMB:      totalMB,
			UsedMB:       usedMB,
			AvailableMB:  availableMB,
			UsedPercent:  usedPercent,
			MountOptions: options,
		})

		// Limit to 100 filesystems
		if len(filesystems) >= 100 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetFilesystemInfoOutput{
			Error: fmt.Sprintf("error reading /proc/mounts: %v", err),
		}, nil
	}

	log.Printf("[get_filesystem_info] Found %d filesystems", len(filesystems))

	return &mcp.CallToolResult{}, GetFilesystemInfoOutput{
		Filesystems: filesystems,
	}, nil
}

// isRealFilesystem checks if a device/fstype represents real storage
func isRealFilesystem(device, fsType string) bool {
	// Include real device filesystems
	if strings.HasPrefix(device, "/dev/") {
		return true
	}

	// Include network filesystems
	if fsType == "nfs" || fsType == "nfs4" || fsType == "cifs" || fsType == "smbfs" {
		return true
	}

	// Exclude pseudo filesystems
	pseudoFS := map[string]bool{
		"proc":         true,
		"sysfs":        true,
		"devpts":       true,
		"tmpfs":        true,
		"devtmpfs":     true,
		"cgroup":       true,
		"cgroup2":      true,
		"pstore":       true,
		"bpf":          true,
		"tracefs":      true,
		"debugfs":      true,
		"securityfs":   true,
		"hugetlbfs":    true,
		"mqueue":       true,
		"configfs":     true,
		"fusectl":      true,
		"fuse.portal":  true,
		"overlay":      true,
		"autofs":       true,
		"binfmt_misc":  true,
	}

	return !pseudoFS[fsType]
}

// Register registers the tool with the MCP server
func (t *GetFilesystemInfoTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_filesystem_info",
		Description: "Get filesystem mount information including device, mount point, type, size, usage, and mount options. Reads from /proc/mounts and uses statfs. Filters out pseudo filesystems.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
