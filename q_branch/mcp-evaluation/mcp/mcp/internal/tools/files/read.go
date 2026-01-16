package files

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxFileSize = 10 * 1024 * 1024 // 10MB

// ReadFileInput defines the input schema
type ReadFileInput struct {
	Path string `json:"path" jsonschema:"Absolute path to the file to read"`
}

// ReadFileOutput defines the output structure
type ReadFileOutput struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	Truncated bool   `json:"truncated,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ReadFileTool reads file contents with safety limits
type ReadFileTool struct{}

func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{}
}

func (t *ReadFileTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input ReadFileInput,
) (
	*mcp.CallToolResult,
	ReadFileOutput,
	error,
) {
	log.Printf("[read_file] Reading file: %s", input.Path)

	// Get file info first
	info, err := os.Stat(input.Path)
	if err != nil {
		return &mcp.CallToolResult{}, ReadFileOutput{
			Path:  input.Path,
			Error: fmt.Sprintf("failed to stat file: %v", err),
		}, nil
	}

	if info.IsDir() {
		return &mcp.CallToolResult{}, ReadFileOutput{
			Path:  input.Path,
			Error: "path is a directory, not a file",
		}, nil
	}

	size := info.Size()
	truncated := false

	// Read file with size limit
	data, err := os.ReadFile(input.Path)
	if err != nil {
		return &mcp.CallToolResult{}, ReadFileOutput{
			Path:      input.Path,
			SizeBytes: size,
			Error:     fmt.Sprintf("failed to read file: %v", err),
		}, nil
	}

	// Truncate if too large
	if len(data) > maxFileSize {
		data = data[:maxFileSize]
		truncated = true
	}

	output := ReadFileOutput{
		Path:      input.Path,
		Content:   string(data),
		SizeBytes: size,
		Truncated: truncated,
	}

	log.Printf("[read_file] Read %d bytes from %s (truncated: %v)", len(data), input.Path, truncated)

	return &mcp.CallToolResult{}, output, nil
}

func (t *ReadFileTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "read_file",
		Description: "Read file contents with a 10MB safety limit. Returns content, size, and truncation status. Use tail_file for large log files.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
