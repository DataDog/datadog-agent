package sysinfo

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetEnvironmentInput defines the input schema
type GetEnvironmentInput struct {
	PID *int `json:"pid,omitempty" jsonschema:"Process ID to get environment for (omit for current process)"`
}

// GetEnvironmentOutput defines the output structure
type GetEnvironmentOutput struct{
	PID         int               `json:"pid"`
	Environment map[string]string `json:"environment,omitempty"`
	Count       int               `json:"count"`
	Error       string            `json:"error,omitempty"`
}

// GetEnvironmentTool gets environment variables for a process
type GetEnvironmentTool struct{}

func NewGetEnvironmentTool() *GetEnvironmentTool {
	return &GetEnvironmentTool{}
}

func (t *GetEnvironmentTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetEnvironmentInput,
) (
	*mcp.CallToolResult,
	GetEnvironmentOutput,
	error,
) {
	var pid int
	var envData []byte
	var err error

	if input.PID == nil {
		// Get current process environment
		pid = os.Getpid()
		log.Printf("[get_environment] Getting environment for current process (PID: %d)", pid)

		// Use os.Environ() for current process
		output := GetEnvironmentOutput{
			PID:         pid,
			Environment: make(map[string]string),
		}

		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				output.Environment[parts[0]] = parts[1]
			}
		}

		output.Count = len(output.Environment)

		log.Printf("[get_environment] Retrieved %d environment variables", output.Count)
		return &mcp.CallToolResult{}, output, nil
	}

	pid = *input.PID
	log.Printf("[get_environment] Getting environment for PID %d", pid)

	// Read from /proc/<pid>/environ
	envPath := filepath.Join("/proc", strconv.Itoa(pid), "environ")
	envData, err = os.ReadFile(envPath)
	if err != nil {
		return &mcp.CallToolResult{}, GetEnvironmentOutput{
			PID:   pid,
			Error: fmt.Sprintf("failed to read process environment: %v", err),
		}, nil
	}

	// Parse null-separated environment variables
	output := GetEnvironmentOutput{
		PID:         pid,
		Environment: make(map[string]string),
	}

	envStr := string(envData)
	for _, env := range strings.Split(envStr, "\x00") {
		if env == "" {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			output.Environment[parts[0]] = parts[1]
		}
	}

	output.Count = len(output.Environment)

	log.Printf("[get_environment] Retrieved %d environment variables for PID %d", output.Count, pid)

	return &mcp.CallToolResult{}, output, nil
}

func (t *GetEnvironmentTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_environment",
		Description: "Get environment variables for a process by PID (omit PID for current process). Returns key-value map of environment variables.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
