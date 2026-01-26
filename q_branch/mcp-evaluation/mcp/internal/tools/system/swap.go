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

// GetSwapDetailsInput defines input parameters
type GetSwapDetailsInput struct{}

// SwapDevice represents a swap device or file
type SwapDevice struct {
	Filename  string `json:"filename"`
	Type      string `json:"type"`
	SizeMB    int64  `json:"size_mb"`
	UsedMB    int64  `json:"used_mb"`
	Priority  int    `json:"priority"`
	UsedPercent float64 `json:"used_percent"`
}

// GetSwapDetailsOutput contains swap information
type GetSwapDetailsOutput struct {
	Devices       []SwapDevice `json:"devices,omitempty"`
	TotalSwapMB   int64        `json:"total_swap_mb"`
	UsedSwapMB    int64        `json:"used_swap_mb"`
	FreeSwapMB    int64        `json:"free_swap_mb"`
	SwapPercent   float64      `json:"swap_percent"`
	Error         string       `json:"error,omitempty"`
}

// GetSwapDetailsTool provides detailed swap usage information
type GetSwapDetailsTool struct{}

// NewGetSwapDetailsTool creates a new swap details tool
func NewGetSwapDetailsTool() *GetSwapDetailsTool {
	return &GetSwapDetailsTool{}
}

// Handler implements the swap details tool
func (t *GetSwapDetailsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetSwapDetailsInput,
) (*mcp.CallToolResult, GetSwapDetailsOutput, error) {
	log.Printf("[get_swap_details] Reading swap information")

	// Read /proc/swaps for device-level details
	swapFile, err := os.Open("/proc/swaps")
	if err != nil {
		return &mcp.CallToolResult{}, GetSwapDetailsOutput{
			Error: fmt.Sprintf("failed to open /proc/swaps: %v", err),
		}, nil
	}
	defer swapFile.Close()

	var devices []SwapDevice
	scanner := bufio.NewScanner(swapFile)

	// Skip header line
	scanner.Scan()

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}

		sizeKB, _ := strconv.ParseInt(fields[2], 10, 64)
		usedKB, _ := strconv.ParseInt(fields[3], 10, 64)
		priority, _ := strconv.Atoi(fields[4])

		sizeMB := sizeKB / 1024
		usedMB := usedKB / 1024
		usedPercent := 0.0
		if sizeMB > 0 {
			usedPercent = float64(usedMB) * 100.0 / float64(sizeMB)
		}

		devices = append(devices, SwapDevice{
			Filename:    fields[0],
			Type:        fields[1],
			SizeMB:      sizeMB,
			UsedMB:      usedMB,
			Priority:    priority,
			UsedPercent: usedPercent,
		})
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetSwapDetailsOutput{
			Error: fmt.Sprintf("error reading /proc/swaps: %v", err),
		}, nil
	}

	// Read /proc/meminfo for total swap statistics
	memFile, err := os.Open("/proc/meminfo")
	if err != nil {
		return &mcp.CallToolResult{}, GetSwapDetailsOutput{
			Error: fmt.Sprintf("failed to open /proc/meminfo: %v", err),
		}, nil
	}
	defer memFile.Close()

	var totalSwap, freeSwap int64
	scanner = bufio.NewScanner(memFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "SwapTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				totalSwap, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		} else if strings.HasPrefix(line, "SwapFree:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				freeSwap, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}

	totalSwapMB := totalSwap / 1024
	freeSwapMB := freeSwap / 1024
	usedSwapMB := totalSwapMB - freeSwapMB

	swapPercent := 0.0
	if totalSwapMB > 0 {
		swapPercent = float64(usedSwapMB) * 100.0 / float64(totalSwapMB)
	}

	log.Printf("[get_swap_details] Found %d swap devices, total: %d MB, used: %d MB (%.1f%%)",
		len(devices), totalSwapMB, usedSwapMB, swapPercent)

	return &mcp.CallToolResult{}, GetSwapDetailsOutput{
		Devices:       devices,
		TotalSwapMB:   totalSwapMB,
		UsedSwapMB:    usedSwapMB,
		FreeSwapMB:    freeSwapMB,
		SwapPercent:   swapPercent,
	}, nil
}

// Register registers the tool with the MCP server
func (t *GetSwapDetailsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_swap_details",
		Description: "Get detailed swap usage information including devices, sizes, and priorities. Reads from /proc/swaps and /proc/meminfo.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
