package process

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

// GetProcessInfoInput defines the input schema
type GetProcessInfoInput struct {
	PID int `json:"pid" jsonschema:"Process ID to get information for"`
}

// ProcessDetails represents detailed process information
type ProcessDetails struct {
	PID       int               `json:"pid"`
	Name      string            `json:"name"`
	Cmdline   string            `json:"cmdline"`
	State     string            `json:"state"`
	PPID      int               `json:"ppid"`
	UID       int               `json:"uid"`
	GID       int               `json:"gid"`
	Threads   int               `json:"threads"`
	FDCount   int               `json:"fd_count"`
	VMSize    int64             `json:"vmsize_mb"`
	VMRSS     int64             `json:"vmrss_mb"`
	Limits    map[string]string `json:"limits,omitempty"`
}

// GetProcessInfoOutput defines the output structure
type GetProcessInfoOutput struct {
	Process *ProcessDetails `json:"process,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// GetProcessInfoTool gets detailed information about a specific process
type GetProcessInfoTool struct{}

func NewGetProcessInfoTool() *GetProcessInfoTool {
	return &GetProcessInfoTool{}
}

func (t *GetProcessInfoTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetProcessInfoInput,
) (
	*mcp.CallToolResult,
	GetProcessInfoOutput,
	error,
) {
	log.Printf("[get_process_info] Getting info for PID %d", input.PID)

	procDir := filepath.Join("/proc", strconv.Itoa(input.PID))

	// Check if process exists
	if _, err := os.Stat(procDir); os.IsNotExist(err) {
		return &mcp.CallToolResult{}, GetProcessInfoOutput{
			Error: fmt.Sprintf("process %d does not exist", input.PID),
		}, nil
	}

	proc := &ProcessDetails{PID: input.PID}

	// Read status
	statusData, err := os.ReadFile(filepath.Join(procDir, "status"))
	if err != nil {
		return &mcp.CallToolResult{}, GetProcessInfoOutput{
			Error: fmt.Sprintf("failed to read process status: %v", err),
		}, nil
	}

	lines := strings.Split(string(statusData), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Name":
			proc.Name = value
		case "State":
			proc.State = value
		case "PPid":
			proc.PPID, _ = strconv.Atoi(value)
		case "Uid":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				proc.UID, _ = strconv.Atoi(fields[0])
			}
		case "Gid":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				proc.GID, _ = strconv.Atoi(fields[0])
			}
		case "Threads":
			proc.Threads, _ = strconv.Atoi(value)
		case "VmSize":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				vmsizeKB, _ := strconv.ParseInt(fields[0], 10, 64)
				proc.VMSize = vmsizeKB / 1024
			}
		case "VmRSS":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				vmrssKB, _ := strconv.ParseInt(fields[0], 10, 64)
				proc.VMRSS = vmrssKB / 1024
			}
		}
	}

	// Read cmdline
	cmdlineData, err := os.ReadFile(filepath.Join(procDir, "cmdline"))
	if err == nil {
		// cmdline has null-separated arguments
		proc.Cmdline = strings.ReplaceAll(string(cmdlineData), "\x00", " ")
		proc.Cmdline = strings.TrimSpace(proc.Cmdline)
	}

	// Count file descriptors
	fdDir := filepath.Join(procDir, "fd")
	if entries, err := os.ReadDir(fdDir); err == nil {
		proc.FDCount = len(entries)
	}

	// Read key limits
	limitsData, err := os.ReadFile(filepath.Join(procDir, "limits"))
	if err == nil {
		proc.Limits = make(map[string]string)
		limitLines := strings.Split(string(limitsData), "\n")
		for _, line := range limitLines {
			if strings.Contains(line, "Max open files") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					proc.Limits["max_open_files"] = fields[3]
				}
			} else if strings.Contains(line, "Max processes") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					proc.Limits["max_processes"] = fields[3]
				}
			}
		}
	}

	log.Printf("[get_process_info] PID %d: %s (%d MB RSS)", input.PID, proc.Name, proc.VMRSS)

	return &mcp.CallToolResult{}, GetProcessInfoOutput{Process: proc}, nil
}

func (t *GetProcessInfoTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_process_info",
		Description: "Get detailed information about a specific process by PID. Returns name, cmdline, state, memory, file descriptors, and key limits.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
