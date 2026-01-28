package kernel

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetKernelParametersInput defines input parameters
type GetKernelParametersInput struct {
	Pattern string `json:"pattern" jsonschema:"Optional glob pattern to filter parameters (e.g. 'net.*', 'vm.swappiness')"`
}

// GetKernelParametersOutput contains kernel parameter information
type GetKernelParametersOutput struct {
	Parameters map[string]string `json:"parameters,omitempty"`
	Truncated  bool              `json:"truncated,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// GetKernelParametersTool provides kernel parameter information
type GetKernelParametersTool struct{}

// NewGetKernelParametersTool creates a new kernel parameters tool
func NewGetKernelParametersTool() *GetKernelParametersTool {
	return &GetKernelParametersTool{}
}

// Handler implements the kernel parameters tool
func (t *GetKernelParametersTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetKernelParametersInput,
) (*mcp.CallToolResult, GetKernelParametersOutput, error) {
	log.Printf("[get_kernel_parameters] Reading kernel parameters with pattern: %s", input.Pattern)

	parameters := make(map[string]string)
	truncated := false

	// Base path for sysctl parameters
	basePath := "/proc/sys"

	// If no pattern, use a reasonable default
	pattern := input.Pattern
	if pattern == "" {
		pattern = "*"
	}

	// Walk the /proc/sys directory
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get the parameter name (relative to /proc/sys)
		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return nil
		}

		// Convert path separators to dots for sysctl-style naming
		paramName := strings.ReplaceAll(relPath, "/", ".")

		// Check if parameter matches pattern
		matched, err := filepath.Match(pattern, paramName)
		if err != nil {
			return nil
		}

		// Also support prefix matching for patterns like "net.*"
		if !matched && strings.HasSuffix(pattern, ".*") {
			prefix := strings.TrimSuffix(pattern, ".*")
			matched = strings.HasPrefix(paramName, prefix+".")
		}

		if !matched && pattern != "*" {
			return nil
		}

		// Read the parameter value
		value, err := os.ReadFile(path)
		if err != nil {
			// Skip unreadable parameters
			return nil
		}

		// Trim whitespace and newlines
		valueStr := strings.TrimSpace(string(value))

		parameters[paramName] = valueStr

		// Limit to 100 parameters
		if len(parameters) >= 100 {
			truncated = true
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil {
		return &mcp.CallToolResult{}, GetKernelParametersOutput{
			Error: fmt.Sprintf("error reading kernel parameters: %v", err),
		}, nil
	}

	log.Printf("[get_kernel_parameters] Found %d kernel parameters (truncated: %v)", len(parameters), truncated)

	return &mcp.CallToolResult{}, GetKernelParametersOutput{
		Parameters: parameters,
		Truncated:  truncated,
	}, nil
}

// Register registers the tool with the MCP server
func (t *GetKernelParametersTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_kernel_parameters",
		Description: "Get kernel parameters (sysctl values) from /proc/sys. Optionally filter with glob pattern (e.g., 'net.*', 'vm.swappiness'). Limited to 100 parameters.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
