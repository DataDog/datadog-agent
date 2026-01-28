package files

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FindFilesInput defines input parameters
type FindFilesInput struct {
	Path        string `json:"path" jsonschema:"Starting directory path"`
	NamePattern string `json:"name_pattern" jsonschema:"Glob pattern to match filenames (e.g. '*.log', 'test*')"`
	MaxDepth    int    `json:"max_depth" jsonschema:"Maximum directory depth to traverse (default: 10, max: 20)"`
	FileType    string `json:"file_type" jsonschema:"Filter by type: file, dir, symlink (default: all)"`
}

// FoundFile represents a file found by the search
type FoundFile struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size_bytes"`
	Type    string    `json:"type"`
	ModTime time.Time `json:"modified_time"`
}

// FindFilesOutput contains file search results
type FindFilesOutput struct {
	Files     []FoundFile `json:"files,omitempty"`
	Truncated bool        `json:"truncated,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// FindFilesTool provides file search functionality
type FindFilesTool struct{}

// NewFindFilesTool creates a new find files tool
func NewFindFilesTool() *FindFilesTool {
	return &FindFilesTool{}
}

// Handler implements the find files tool
func (t *FindFilesTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input FindFilesInput,
) (*mcp.CallToolResult, FindFilesOutput, error) {
	log.Printf("[find_files] Searching in %s with pattern: %s", input.Path, input.NamePattern)

	// Set defaults
	maxDepth := input.MaxDepth
	if maxDepth == 0 {
		maxDepth = 10
	}
	if maxDepth > 20 {
		maxDepth = 20
	}

	namePattern := input.NamePattern
	if namePattern == "" {
		namePattern = "*"
	}

	// Context with timeout
	searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var files []FoundFile
	truncated := false
	startDepth := countPathDepth(input.Path)

	// Walk directory tree
	err := filepath.WalkDir(input.Path, func(path string, d os.DirEntry, err error) error {
		// Check context cancellation
		select {
		case <-searchCtx.Done():
			return filepath.SkipAll
		default:
		}

		if err != nil {
			// Skip directories we can't access
			return nil
		}

		// Check depth limit
		currentDepth := countPathDepth(path) - startDepth
		if currentDepth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check name pattern
		matched, err := filepath.Match(namePattern, d.Name())
		if err != nil || !matched {
			return nil
		}

		// Get file info
		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Check file type filter
		fileType := getFileType(info)
		if input.FileType != "" && input.FileType != fileType {
			return nil
		}

		files = append(files, FoundFile{
			Path:    path,
			Size:    info.Size(),
			Type:    fileType,
			ModTime: info.ModTime(),
		})

		// Limit to 500 results
		if len(files) >= 500 {
			truncated = true
			return filepath.SkipAll
		}

		return nil
	})

	// Check if timeout occurred
	if searchCtx.Err() == context.DeadlineExceeded {
		truncated = true
		log.Printf("[find_files] Search timed out after 30s")
	}

	if err != nil && err != filepath.SkipAll {
		return &mcp.CallToolResult{}, FindFilesOutput{
			Error: fmt.Sprintf("error searching files: %v", err),
		}, nil
	}

	log.Printf("[find_files] Found %d files (truncated: %v)", len(files), truncated)

	return &mcp.CallToolResult{}, FindFilesOutput{
		Files:     files,
		Truncated: truncated,
	}, nil
}

// countPathDepth counts the number of path separators
func countPathDepth(path string) int {
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == "/" {
		return 0
	}
	count := 0
	for _, r := range cleaned {
		if r == filepath.Separator {
			count++
		}
	}
	return count
}

// getFileType determines the file type
func getFileType(info os.FileInfo) string {
	mode := info.Mode()
	if mode.IsDir() {
		return "dir"
	}
	if mode&os.ModeSymlink != 0 {
		return "symlink"
	}
	if mode&os.ModeDevice != 0 {
		return "device"
	}
	if mode&os.ModeNamedPipe != 0 {
		return "pipe"
	}
	if mode&os.ModeSocket != 0 {
		return "socket"
	}
	return "file"
}

// Register registers the tool with the MCP server
func (t *FindFilesTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "find_files",
		Description: "Recursively search for files matching pattern with depth and type filtering. Uses filepath.WalkDir with 30s timeout. Limited to 500 results and max depth 20.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
