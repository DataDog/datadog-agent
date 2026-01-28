package files

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListDirectoryInput defines input parameters
type ListDirectoryInput struct {
	Path       string `json:"path" jsonschema:"Absolute path to directory"`
	ShowHidden bool   `json:"show_hidden" jsonschema:"Include hidden files (starting with .)"`
	SortBy     string `json:"sort_by" jsonschema:"Sort by: name, size, mtime (default: name)"`
}

// DirectoryEntry represents a file or directory entry
type DirectoryEntry struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Size        int64     `json:"size_bytes"`
	Permissions string    `json:"permissions"`
	ModTime     time.Time `json:"modified_time"`
}

// ListDirectoryOutput contains directory listing
type ListDirectoryOutput struct {
	Path      string           `json:"path"`
	Entries   []DirectoryEntry `json:"entries,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// ListDirectoryTool provides directory listing functionality
type ListDirectoryTool struct{}

// NewListDirectoryTool creates a new list directory tool
func NewListDirectoryTool() *ListDirectoryTool {
	return &ListDirectoryTool{}
}

// Handler implements the list directory tool
func (t *ListDirectoryTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input ListDirectoryInput,
) (*mcp.CallToolResult, ListDirectoryOutput, error) {
	log.Printf("[list_directory] Listing directory: %s", input.Path)

	// Read directory
	entries, err := os.ReadDir(input.Path)
	if err != nil {
		return &mcp.CallToolResult{}, ListDirectoryOutput{
			Path:  input.Path,
			Error: fmt.Sprintf("failed to read directory: %v", err),
		}, nil
	}

	var result []DirectoryEntry

	for _, entry := range entries {
		// Skip hidden files if requested
		if !input.ShowHidden && len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			continue
		}

		// Get file info
		info, err := entry.Info()
		if err != nil {
			// Skip entries we can't stat
			continue
		}

		fileType := "file"
		if info.IsDir() {
			fileType = "dir"
		} else if info.Mode()&os.ModeSymlink != 0 {
			fileType = "symlink"
		} else if info.Mode()&os.ModeDevice != 0 {
			fileType = "device"
		} else if info.Mode()&os.ModeNamedPipe != 0 {
			fileType = "pipe"
		} else if info.Mode()&os.ModeSocket != 0 {
			fileType = "socket"
		}

		result = append(result, DirectoryEntry{
			Name:        entry.Name(),
			Type:        fileType,
			Size:        info.Size(),
			Permissions: info.Mode().String(),
			ModTime:     info.ModTime(),
		})

		// Limit to 1000 entries
		if len(result) >= 1000 {
			break
		}
	}

	truncated := len(result) >= 1000

	// Sort results
	sortBy := input.SortBy
	if sortBy == "" {
		sortBy = "name"
	}

	switch sortBy {
	case "name":
		sort.Slice(result, func(i, j int) bool {
			return result[i].Name < result[j].Name
		})
	case "size":
		sort.Slice(result, func(i, j int) bool {
			return result[i].Size > result[j].Size
		})
	case "mtime":
		sort.Slice(result, func(i, j int) bool {
			return result[i].ModTime.After(result[j].ModTime)
		})
	}

	log.Printf("[list_directory] Found %d entries in %s (truncated: %v)",
		len(result), input.Path, truncated)

	return &mcp.CallToolResult{}, ListDirectoryOutput{
		Path:      input.Path,
		Entries:   result,
		Truncated: truncated,
	}, nil
}

// Register registers the tool with the MCP server
func (t *ListDirectoryTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "list_directory",
		Description: "List directory contents with file metadata including name, type, size, permissions, and modification time. Supports sorting and filtering hidden files. Limited to 1000 entries.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
