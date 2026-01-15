package system

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetCPUInfoInput defines the input schema
type GetCPUInfoInput struct{}

// GetCPUInfoOutput defines the output structure
type GetCPUInfoOutput struct {
	CPUCount    int     `json:"cpu_count"`
	ModelName   string  `json:"model_name,omitempty"`
	Load1Min    float64 `json:"load_1min"`
	Load5Min    float64 `json:"load_5min"`
	Load15Min   float64 `json:"load_15min"`
	LoadPercent float64 `json:"load_percent"` // load1/cpu_count * 100
	Error       string  `json:"error,omitempty"`
}

// GetCPUInfoTool reads CPU information
type GetCPUInfoTool struct{}

func NewGetCPUInfoTool() *GetCPUInfoTool {
	return &GetCPUInfoTool{}
}

func (t *GetCPUInfoTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetCPUInfoInput,
) (
	*mcp.CallToolResult,
	GetCPUInfoOutput,
	error,
) {
	log.Printf("[get_cpu_info] Reading CPU information")

	// Get CPU count
	cpuCount := runtime.NumCPU()

	// Read model name from /proc/cpuinfo
	modelName := ""
	if file, err := os.Open("/proc/cpuinfo"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					modelName = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// Read load averages from /proc/loadavg
	loadavgData, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return &mcp.CallToolResult{}, GetCPUInfoOutput{
			Error: fmt.Sprintf("failed to read /proc/loadavg: %v", err),
		}, nil
	}

	fields := strings.Fields(string(loadavgData))
	if len(fields) < 3 {
		return &mcp.CallToolResult{}, GetCPUInfoOutput{
			Error: "invalid /proc/loadavg format",
		}, nil
	}

	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)

	loadPercent := 0.0
	if cpuCount > 0 {
		loadPercent = (load1 / float64(cpuCount)) * 100
	}

	output := GetCPUInfoOutput{
		CPUCount:    cpuCount,
		ModelName:   modelName,
		Load1Min:    load1,
		Load5Min:    load5,
		Load15Min:   load15,
		LoadPercent: loadPercent,
	}

	log.Printf("[get_cpu_info] CPUs: %d, Load: %.2f/%.2f/%.2f (%.1f%%)",
		cpuCount, load1, load5, load15, loadPercent)

	return &mcp.CallToolResult{}, output, nil
}

func (t *GetCPUInfoTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_cpu_info",
		Description: "Get key CPU metrics: count, model, and load averages (1/5/15min). Returns only essential fields for quick assessment. For complete CPU details, use read_file on /proc/cpuinfo or /proc/loadavg.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
