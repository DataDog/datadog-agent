//go:build linux

package files

import (
	"context"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetFileMetadataInput defines input parameters
type GetFileMetadataInput struct {
	Path string `json:"path" jsonschema:"Absolute path to the file or directory"`
}

// GetFileMetadataOutput contains detailed file metadata
type GetFileMetadataOutput struct {
	Path         string    `json:"path"`
	Size         int64     `json:"size_bytes"`
	Permissions  string    `json:"permissions"`
	PermOctal    string    `json:"permissions_octal"`
	Owner        uint32    `json:"owner_uid"`
	Group        uint32    `json:"group_gid"`
	ModifiedTime time.Time `json:"modified_time"`
	AccessTime   time.Time `json:"access_time"`
	ChangeTime   time.Time `json:"change_time"`
	IsDir        bool      `json:"is_dir"`
	IsSymlink    bool      `json:"is_symlink"`
	LinkTarget   string    `json:"link_target,omitempty"`
	Inode        uint64    `json:"inode"`
	Links        uint64    `json:"hard_links"`
	Error        string    `json:"error,omitempty"`
}

// GetFileMetadataTool provides detailed file/directory metadata
type GetFileMetadataTool struct{}

// NewGetFileMetadataTool creates a new file metadata tool
func NewGetFileMetadataTool() *GetFileMetadataTool {
	return &GetFileMetadataTool{}
}

// Handler implements the file metadata tool
func (t *GetFileMetadataTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetFileMetadataInput,
) (*mcp.CallToolResult, GetFileMetadataOutput, error) {
	log.Printf("[get_file_metadata] Getting metadata for: %s", input.Path)

	// Use Lstat to not follow symlinks
	fileInfo, err := os.Lstat(input.Path)
	if err != nil {
		return &mcp.CallToolResult{}, GetFileMetadataOutput{
			Path:  input.Path,
			Error: fmt.Sprintf("failed to stat file: %v", err),
		}, nil
	}

	// Get system-specific stat info
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return &mcp.CallToolResult{}, GetFileMetadataOutput{
			Path:  input.Path,
			Error: "failed to get system stat information",
		}, nil
	}

	isSymlink := fileInfo.Mode()&os.ModeSymlink != 0
	var linkTarget string
	if isSymlink {
		linkTarget, err = os.Readlink(input.Path)
		if err != nil {
			linkTarget = fmt.Sprintf("(error reading link: %v)", err)
		}
	}

	// Convert permissions to octal string
	permOctal := fmt.Sprintf("%04o", fileInfo.Mode().Perm())

	// Access and change times (Linux)
	atime := time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
	ctime := time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)

	output := GetFileMetadataOutput{
		Path:         input.Path,
		Size:         fileInfo.Size(),
		Permissions:  fileInfo.Mode().String(),
		PermOctal:    permOctal,
		Owner:        stat.Uid,
		Group:        stat.Gid,
		ModifiedTime: fileInfo.ModTime(),
		AccessTime:   atime,
		ChangeTime:   ctime,
		IsDir:        fileInfo.IsDir(),
		IsSymlink:    isSymlink,
		LinkTarget:   linkTarget,
		Inode:        stat.Ino,
		Links:        uint64(stat.Nlink),
	}

	log.Printf("[get_file_metadata] %s: size=%d, perms=%s, owner=%d:%d",
		input.Path, output.Size, output.PermOctal, output.Owner, output.Group)

	return &mcp.CallToolResult{}, output, nil
}

// Register registers the tool with the MCP server
func (t *GetFileMetadataTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_file_metadata",
		Description: "Get detailed metadata for a file or directory including size, permissions, ownership, timestamps, and inode information.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
