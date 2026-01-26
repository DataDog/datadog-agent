package kernel

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetLoadedModulesInput defines input parameters
type GetLoadedModulesInput struct{}

// KernelModule represents a loaded kernel module
type KernelModule struct {
	Name         string   `json:"name"`
	Size         int64    `json:"size_bytes"`
	UsedCount    int      `json:"used_count"`
	Dependencies []string `json:"dependencies,omitempty"`
	State        string   `json:"state"`
}

// GetLoadedModulesOutput contains loaded kernel module information
type GetLoadedModulesOutput struct {
	Modules []KernelModule `json:"modules,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// GetLoadedModulesTool provides loaded kernel module information
type GetLoadedModulesTool struct{}

// NewGetLoadedModulesTool creates a new loaded modules tool
func NewGetLoadedModulesTool() *GetLoadedModulesTool {
	return &GetLoadedModulesTool{}
}

// Handler implements the loaded modules tool
func (t *GetLoadedModulesTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetLoadedModulesInput,
) (*mcp.CallToolResult, GetLoadedModulesOutput, error) {
	log.Printf("[get_loaded_modules] Reading loaded kernel modules")

	// Read /proc/modules
	file, err := os.Open("/proc/modules")
	if err != nil {
		return &mcp.CallToolResult{}, GetLoadedModulesOutput{
			Error: fmt.Sprintf("failed to open /proc/modules: %v", err),
		}, nil
	}
	defer file.Close()

	var modules []KernelModule
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 {
			continue
		}

		name := fields[0]
		sizeStr := fields[1]
		usedCountStr := fields[2]
		depsStr := fields[3]
		state := fields[4]

		// Parse size
		size, _ := strconv.ParseInt(sizeStr, 10, 64)

		// Parse used count
		usedCount, _ := strconv.Atoi(usedCountStr)

		// Parse dependencies
		var deps []string
		if depsStr != "-" {
			// Dependencies are comma-separated, may have trailing comma
			depList := strings.TrimSuffix(depsStr, ",")
			if depList != "" {
				deps = strings.Split(depList, ",")
			}
		}

		modules = append(modules, KernelModule{
			Name:         name,
			Size:         size,
			UsedCount:    usedCount,
			Dependencies: deps,
			State:        state,
		})

		// Limit to 500 modules
		if len(modules) >= 500 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetLoadedModulesOutput{
			Error: fmt.Sprintf("error reading /proc/modules: %v", err),
		}, nil
	}

	log.Printf("[get_loaded_modules] Found %d loaded kernel modules", len(modules))

	return &mcp.CallToolResult{}, GetLoadedModulesOutput{
		Modules: modules,
	}, nil
}

// Register registers the tool with the MCP server
func (t *GetLoadedModulesTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_loaded_modules",
		Description: "Get list of loaded kernel modules with name, size, usage count, dependencies, and state. Reads from /proc/modules.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
