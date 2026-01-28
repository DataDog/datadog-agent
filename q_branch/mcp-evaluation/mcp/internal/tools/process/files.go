package process

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetOpenFilesInput defines input parameters
type GetOpenFilesInput struct {
	PID int `json:"pid" jsonschema:"Process ID to get open files for"`
}

// OpenFile represents an open file descriptor
type OpenFile struct {
	FD   int    `json:"fd"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// GetOpenFilesOutput contains open file information
type GetOpenFilesOutput struct {
	PID   int        `json:"pid"`
	Files []OpenFile `json:"files,omitempty"`
	Error string     `json:"error,omitempty"`
}

// GetOpenFilesTool provides open file information
type GetOpenFilesTool struct{}

// NewGetOpenFilesTool creates a new open files tool
func NewGetOpenFilesTool() *GetOpenFilesTool {
	return &GetOpenFilesTool{}
}

// Handler implements the open files tool
func (t *GetOpenFilesTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetOpenFilesInput,
) (*mcp.CallToolResult, GetOpenFilesOutput, error) {
	log.Printf("[get_open_files] Getting open files for PID %d", input.PID)

	// List /proc/[pid]/fd directory
	fdPath := fmt.Sprintf("/proc/%d/fd", input.PID)
	entries, err := os.ReadDir(fdPath)
	if err != nil {
		return &mcp.CallToolResult{}, GetOpenFilesOutput{
			PID:   input.PID,
			Error: fmt.Sprintf("failed to read %s: %v", fdPath, err),
		}, nil
	}

	var files []OpenFile

	for _, entry := range entries {
		// Parse FD number from entry name
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Resolve symlink to get actual file path
		linkPath := fmt.Sprintf("%s/%s", fdPath, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			// If we can't read the link, record the error
			target = fmt.Sprintf("(error: %v)", err)
		}

		// Determine file type based on path
		fileType := determineFileType(target)

		files = append(files, OpenFile{
			FD:   fd,
			Path: target,
			Type: fileType,
		})

		// Limit to 1000 files
		if len(files) >= 1000 {
			break
		}
	}

	log.Printf("[get_open_files] Found %d open files for PID %d", len(files), input.PID)

	return &mcp.CallToolResult{}, GetOpenFilesOutput{
		PID:   input.PID,
		Files: files,
	}, nil
}

// determineFileType classifies file type based on path
func determineFileType(path string) string {
	if len(path) == 0 {
		return "unknown"
	}

	switch path[0] {
	case '/':
		// Regular file path
		return "file"
	case 's':
		// socket:[inode] or similar
		if len(path) > 6 && path[:7] == "socket:" {
			return "socket"
		}
	case 'p':
		// pipe:[inode]
		if len(path) > 5 && path[:5] == "pipe:" {
			return "pipe"
		}
	case 'a':
		// anon_inode:[eventfd], anon_inode:[eventpoll], etc.
		if len(path) > 10 && path[:11] == "anon_inode:" {
			return "anon_inode"
		}
	}

	// Check for other special files
	if path == "(error" || path[0] == '(' {
		return "error"
	}

	return "other"
}

// Register registers the tool with the MCP server
func (t *GetOpenFilesTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_open_files",
		Description: "Get list of open files for a specific process including file descriptor number, path, and type. Lists /proc/[pid]/fd and resolves symlinks.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
