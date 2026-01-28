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

// GetIOStatsInput defines the input schema
type GetIOStatsInput struct{}

// DeviceIOStats represents I/O stats for a single device
type DeviceIOStats struct {
	Device       string `json:"device"`
	ReadsTotal   int64  `json:"reads_total"`
	WritesTotal  int64  `json:"writes_total"`
	ReadsMB      int64  `json:"reads_mb"`
	WritesMB     int64  `json:"writes_mb"`
}

// GetIOStatsOutput defines the output structure
type GetIOStatsOutput struct {
	Devices []DeviceIOStats `json:"devices,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// GetIOStatsTool reads disk I/O statistics
type GetIOStatsTool struct{}

func NewGetIOStatsTool() *GetIOStatsTool {
	return &GetIOStatsTool{}
}

func (t *GetIOStatsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetIOStatsInput,
) (
	*mcp.CallToolResult,
	GetIOStatsOutput,
	error,
) {
	log.Printf("[get_io_stats] Reading I/O statistics")

	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return &mcp.CallToolResult{}, GetIOStatsOutput{
			Error: fmt.Sprintf("failed to open /proc/diskstats: %v", err),
		}, nil
	}
	defer file.Close()

	var devices []DeviceIOStats
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// /proc/diskstats format: major minor name reads ... sectors_read ... writes ... sectors_written ...
		if len(fields) < 14 {
			continue
		}

		deviceName := fields[2]

		// Skip partitions, only show whole disks (simple heuristic: no digits at end or loop/ram devices)
		if len(deviceName) > 0 && (deviceName[len(deviceName)-1] >= '0' && deviceName[len(deviceName)-1] <= '9') {
			continue
		}
		if strings.HasPrefix(deviceName, "loop") || strings.HasPrefix(deviceName, "ram") {
			continue
		}

		readsCompleted, _ := strconv.ParseInt(fields[3], 10, 64)
		sectorsRead, _ := strconv.ParseInt(fields[5], 10, 64)
		writesCompleted, _ := strconv.ParseInt(fields[7], 10, 64)
		sectorsWritten, _ := strconv.ParseInt(fields[9], 10, 64)

		// Sectors are 512 bytes, convert to MB
		readsMB := sectorsRead * 512 / (1024 * 1024)
		writesMB := sectorsWritten * 512 / (1024 * 1024)

		// Only include devices with activity
		if readsCompleted > 0 || writesCompleted > 0 {
			devices = append(devices, DeviceIOStats{
				Device:       deviceName,
				ReadsTotal:   readsCompleted,
				WritesTotal:  writesCompleted,
				ReadsMB:      readsMB,
				WritesMB:     writesMB,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetIOStatsOutput{
			Error: fmt.Sprintf("error reading /proc/diskstats: %v", err),
		}, nil
	}

	log.Printf("[get_io_stats] Found %d active devices", len(devices))

	return &mcp.CallToolResult{}, GetIOStatsOutput{Devices: devices}, nil
}

func (t *GetIOStatsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_io_stats",
		Description: "Get disk I/O statistics for active devices. Returns cumulative reads/writes (count and MB) since boot for whole disks only. For complete I/O stats, use read_file on /proc/diskstats.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
