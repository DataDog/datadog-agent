package system

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

// GetMemoryInfoInput defines the input schema
type GetMemoryInfoInput struct{}

// GetMemoryInfoOutput defines the output structure
type GetMemoryInfoOutput struct {
	TotalMB     int64   `json:"total_mb"`
	AvailableMB int64   `json:"available_mb"`
	UsedMB      int64   `json:"used_mb"`
	UsedPercent float64 `json:"used_percent"`
	BuffersMB   *int64  `json:"buffers_mb,omitempty"`
	CachedMB    *int64  `json:"cached_mb,omitempty"`
	SwapTotalMB *int64  `json:"swap_total_mb,omitempty"`
	SwapUsedMB  *int64  `json:"swap_used_mb,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// GetMemoryInfoTool reads memory information from /proc/meminfo
type GetMemoryInfoTool struct{}

func NewGetMemoryInfoTool() *GetMemoryInfoTool {
	return &GetMemoryInfoTool{}
}

func (t *GetMemoryInfoTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetMemoryInfoInput,
) (
	*mcp.CallToolResult,
	GetMemoryInfoOutput,
	error,
) {
	log.Printf("[get_memory_info] Reading memory information")

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return &mcp.CallToolResult{}, GetMemoryInfoOutput{
			Error: fmt.Sprintf("failed to open /proc/meminfo: %v", err),
		}, nil
	}
	defer file.Close()

	memInfo := make(map[string]int64)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			value, err := strconv.ParseInt(fields[1], 10, 64)
			if err == nil {
				// Values in /proc/meminfo are in kB, convert to MB
				memInfo[key] = value / 1024
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetMemoryInfoOutput{
			Error: fmt.Sprintf("error reading /proc/meminfo: %v", err),
		}, nil
	}

	totalMB := memInfo["MemTotal"]
	availableMB := memInfo["MemAvailable"]
	usedMB := totalMB - availableMB
	usedPercent := 0.0
	if totalMB > 0 {
		usedPercent = float64(usedMB) / float64(totalMB) * 100
	}

	output := GetMemoryInfoOutput{
		TotalMB:     totalMB,
		AvailableMB: availableMB,
		UsedMB:      usedMB,
		UsedPercent: usedPercent,
	}

	// Optional fields - use pointers so 0 values are included but missing values are omitted
	if buffers, ok := memInfo["Buffers"]; ok {
		output.BuffersMB = &buffers
	}
	if cached, ok := memInfo["Cached"]; ok {
		output.CachedMB = &cached
	}
	if swapTotal, ok := memInfo["SwapTotal"]; ok {
		output.SwapTotalMB = &swapTotal
		swapFree := memInfo["SwapFree"]
		swapUsed := swapTotal - swapFree
		output.SwapUsedMB = &swapUsed
	}

	log.Printf("[get_memory_info] Memory: %d/%d MB used (%.1f%%)", usedMB, totalMB, usedPercent)

	return &mcp.CallToolResult{}, output, nil
}

func (t *GetMemoryInfoTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_memory_info",
		Description: "Get key memory metrics from /proc/meminfo (total, available, used, buffers, cache, swap). Returns only the most important fields for quick assessment. For complete /proc/meminfo data, use read_file tool.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
