package tools

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SafeShellExecuteInput defines the input schema for safe-shell command execution
type SafeShellExecuteInput struct {
	Command string `json:"command" jsonschema:"The bash command to execute in a sandboxed environment"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"Optional timeout in seconds (default: 30)"`
	User    string `json:"user,omitempty" jsonschema:"Optional user to run the command as (default: eval-user),default=eval-user"`
}

// SafeShellExecuteOutput defines the output structure for safe-shell command execution
type SafeShellExecuteOutput struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	Sandbox  string `json:"sandbox"`
}

// SafeShellTool executes bash commands in a kernel-enforced sandbox
type SafeShellTool struct {
	defaultTimeout   time.Duration
	safeShellBinPath string
}

// NewSafeShellTool creates a new safe-shell execution tool
func NewSafeShellTool(timeout time.Duration) (*SafeShellTool, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Find safe-shell binary in PATH
	safeShellPath, err := exec.LookPath("safe-shell")
	if err != nil {
		return nil, fmt.Errorf("safe-shell binary not found in PATH: %w", err)
	}

	log.Printf("[safe_shell_execute] Found safe-shell at: %s", safeShellPath)

	return &SafeShellTool{
		defaultTimeout:   timeout,
		safeShellBinPath: safeShellPath,
	}, nil
}

// Handler executes the bash command in a safe-shell sandbox and returns the result
func (t *SafeShellTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input SafeShellExecuteInput,
) (
	*mcp.CallToolResult,
	SafeShellExecuteOutput,
	error,
) {
	if input.Command == "" {
		return nil, SafeShellExecuteOutput{}, fmt.Errorf("missing 'command' parameter")
	}

	// Set default user if not specified
	user := input.User
	if user == "" {
		user = "eval-user"
	}

	// Log the incoming request
	log.Printf("[safe_shell_execute] Executing command: %q (user: %s, timeout: %ds)", input.Command, user, input.Timeout)

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

	// Execute command using safe-shell binary (absolute path)
	// Wrap with sudo -u to run as specified user
	var cmd *exec.Cmd
	cmd = exec.CommandContext(
		execCtx,
		"sudo", "-u", user,
		t.safeShellBinPath,
		input.Command,
	)
	output, execErr := cmd.CombinedOutput()

	// Build result output
	result := SafeShellExecuteOutput{
		Command:  input.Command,
		Output:   string(output),
		ExitCode: 0,
		Sandbox:  "safe-shell",
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if execErr != nil {
		result.Error = execErr.Error()
	}

	// Log the result
	log.Printf("[safe_shell_execute] Command completed: exit_code=%d, output_length=%d bytes, user=%s, sandbox=safe-shell",
		result.ExitCode, len(result.Output), user)

	return &mcp.CallToolResult{}, result, nil
}

// Register registers the safe-shell tool with the MCP server
func (t *SafeShellTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "safe_shell_execute",
		Description: "Execute bash commands in a kernel-enforced sandbox. Full POSIX shell with read-only filesystem, no network, and resource limits. Blocks writes, exfiltration, and privilege escalation while allowing system inspection.",
	}

	mcp.AddTool(
		server,
		tool,
		t.Handler,
	)
	return nil
}
