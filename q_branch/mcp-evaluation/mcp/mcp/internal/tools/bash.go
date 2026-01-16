package tools

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BashExecuteInput defines the input schema for bash command execution
type BashExecuteInput struct {
	Command string `json:"command" jsonschema:"The bash command to execute"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"Optional timeout in seconds (default: 30)"`
}

// BashExecuteOutput defines the output structure for bash command execution
type BashExecuteOutput struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
}

// BashTool executes arbitrary bash commands
type BashTool struct {
	defaultTimeout time.Duration
}

// NewBashTool creates a new bash execution tool
func NewBashTool(timeout time.Duration) *BashTool {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &BashTool{
		defaultTimeout: timeout,
	}
}

// Handler executes the bash command and returns the result
func (t *BashTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input BashExecuteInput,
) (
	*mcp.CallToolResult,
	BashExecuteOutput,
	error,
) {
	if input.Command == "" {
		return nil, BashExecuteOutput{}, fmt.Errorf("missing 'command' parameter")
	}

	// Log the incoming request
	log.Printf("[bash_execute] Executing command: %q (timeout: %ds)", input.Command, input.Timeout)

	// Determine timeout
	timeout := t.defaultTimeout
	if input.Timeout > 0 {
		timeout = time.Duration(input.Timeout) * time.Second
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(
		ctx,
		timeout,
	)
	defer cancel()

	// Execute the command
	cmd := exec.CommandContext(
		execCtx,
		"bash",
		"-c",
		input.Command,
	)
	output, execErr := cmd.CombinedOutput()

	// Build result output
	result := BashExecuteOutput{
		Command:  input.Command,
		Output:   string(output),
		ExitCode: 0,
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if execErr != nil {
		result.Error = execErr.Error()
	}

	// Log the result
	log.Printf("[bash_execute] Command completed: exit_code=%d, output_length=%d bytes", result.ExitCode, len(result.Output))

	return &mcp.CallToolResult{}, result, nil
}

// Register registers the bash tool with the MCP server
func (t *BashTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "bash_execute",
		Description: "Execute arbitrary bash commands and return their output. Use this to run shell commands, inspect the system, or perform operations.",
	}

	mcp.AddTool(
		server,
		tool,
		t.Handler,
	)
	return nil
}
