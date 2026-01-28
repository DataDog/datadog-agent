package files

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxTailLines = 1000

// TailFileInput defines the input schema
type TailFileInput struct {
	Path  string `json:"path" jsonschema:"Absolute path to the file to tail"`
	Lines *int   `json:"lines,omitempty" jsonschema:"Number of lines to return (default: 50, max: 1000)"`
}

// TailFileOutput defines the output structure
type TailFileOutput struct {
	Path      string   `json:"path"`
	Lines     []string `json:"lines,omitempty"`
	LineCount int      `json:"line_count"`
	Truncated bool     `json:"truncated,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// TailFileTool gets the last N lines of a file
type TailFileTool struct{}

func NewTailFileTool() *TailFileTool {
	return &TailFileTool{}
}

func (t *TailFileTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input TailFileInput,
) (
	*mcp.CallToolResult,
	TailFileOutput,
	error,
) {
	numLines := 50
	if input.Lines != nil {
		numLines = *input.Lines
		if numLines > maxTailLines {
			numLines = maxTailLines
		}
		if numLines < 1 {
			numLines = 1
		}
	}

	log.Printf("[tail_file] Tailing %s (last %d lines)", input.Path, numLines)

	file, err := os.Open(input.Path)
	if err != nil {
		return &mcp.CallToolResult{}, TailFileOutput{
			Path:  input.Path,
			Error: fmt.Sprintf("failed to open file: %v", err),
		}, nil
	}
	defer file.Close()

	// Read all lines into a circular buffer
	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		// Keep only the last numLines in memory
		if len(lines) > numLines {
			lines = lines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, TailFileOutput{
			Path:  input.Path,
			Error: fmt.Sprintf("error reading file: %v", err),
		}, nil
	}

	output := TailFileOutput{
		Path:      input.Path,
		Lines:     lines,
		LineCount: len(lines),
		Truncated: false, // We always return the last N lines
	}

	log.Printf("[tail_file] Returned %d lines from %s", len(lines), input.Path)

	return &mcp.CallToolResult{}, output, nil
}

func (t *TailFileTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "tail_file",
		Description: "Get the last N lines of a file (default: 50, max: 1000). Useful for reading log files. Returns array of lines.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
