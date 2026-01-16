package files

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxSearchResults = 500

// SearchFileInput defines the input schema
type SearchFileInput struct {
	Path    string `json:"path" jsonschema:"Absolute path to the file to search"`
	Pattern string `json:"pattern" jsonschema:"Pattern to search for (supports regex)"`
	Limit   *int   `json:"limit,omitempty" jsonschema:"Maximum number of matches to return (default: 100, max: 500)"`
}

// MatchInfo represents a single match
type MatchInfo struct {
	LineNumber int    `json:"line_number"`
	Line       string `json:"line"`
}

// SearchFileOutput defines the output structure
type SearchFileOutput struct {
	Path      string      `json:"path"`
	Pattern   string      `json:"pattern"`
	Matches   []MatchInfo `json:"matches,omitempty"`
	Count     int         `json:"count"`
	Truncated bool        `json:"truncated,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// SearchFileTool searches for a pattern in a file
type SearchFileTool struct{}

func NewSearchFileTool() *SearchFileTool {
	return &SearchFileTool{}
}

func (t *SearchFileTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input SearchFileInput,
) (
	*mcp.CallToolResult,
	SearchFileOutput,
	error,
) {
	limit := 100
	if input.Limit != nil {
		limit = *input.Limit
		if limit > maxSearchResults {
			limit = maxSearchResults
		}
		if limit < 1 {
			limit = 1
		}
	}

	log.Printf("[search_file] Searching %s for pattern: %q (limit: %d)", input.Path, input.Pattern, limit)

	// Compile regex pattern
	re, err := regexp.Compile(input.Pattern)
	if err != nil {
		return &mcp.CallToolResult{}, SearchFileOutput{
			Path:    input.Path,
			Pattern: input.Pattern,
			Error:   fmt.Sprintf("invalid regex pattern: %v", err),
		}, nil
	}

	file, err := os.Open(input.Path)
	if err != nil {
		return &mcp.CallToolResult{}, SearchFileOutput{
			Path:    input.Path,
			Pattern: input.Pattern,
			Error:   fmt.Sprintf("failed to open file: %v", err),
		}, nil
	}
	defer file.Close()

	var matches []MatchInfo
	lineNumber := 0
	truncated := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		if re.MatchString(line) {
			if len(matches) >= limit {
				truncated = true
				break
			}

			matches = append(matches, MatchInfo{
				LineNumber: lineNumber,
				Line:       line,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, SearchFileOutput{
			Path:    input.Path,
			Pattern: input.Pattern,
			Error:   fmt.Sprintf("error reading file: %v", err),
		}, nil
	}

	output := SearchFileOutput{
		Path:      input.Path,
		Pattern:   input.Pattern,
		Matches:   matches,
		Count:     len(matches),
		Truncated: truncated,
	}

	log.Printf("[search_file] Found %d matches in %s (truncated: %v)", len(matches), input.Path, truncated)

	return &mcp.CallToolResult{}, output, nil
}

func (t *SearchFileTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "search_file",
		Description: "Search for a pattern (regex) in a file. Returns matching lines with line numbers (max: 500 matches). Use for grep-style searches.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
