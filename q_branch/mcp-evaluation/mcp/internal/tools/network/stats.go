package network

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

// GetNetworkStatsInput defines input parameters
type GetNetworkStatsInput struct{}

// NetworkInterfaceStats represents statistics for a network interface
type NetworkInterfaceStats struct {
	Interface  string `json:"interface"`
	RxBytes    uint64 `json:"rx_bytes"`
	RxPackets  uint64 `json:"rx_packets"`
	RxErrors   uint64 `json:"rx_errors"`
	RxDropped  uint64 `json:"rx_dropped"`
	TxBytes    uint64 `json:"tx_bytes"`
	TxPackets  uint64 `json:"tx_packets"`
	TxErrors   uint64 `json:"tx_errors"`
	TxDropped  uint64 `json:"tx_dropped"`
}

// GetNetworkStatsOutput contains network interface statistics
type GetNetworkStatsOutput struct {
	Interfaces []NetworkInterfaceStats `json:"interfaces,omitempty"`
	Error      string                  `json:"error,omitempty"`
}

// GetNetworkStatsTool provides network interface statistics
type GetNetworkStatsTool struct{}

// NewGetNetworkStatsTool creates a new network stats tool
func NewGetNetworkStatsTool() *GetNetworkStatsTool {
	return &GetNetworkStatsTool{}
}

// Handler implements the network stats tool
func (t *GetNetworkStatsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetNetworkStatsInput,
) (*mcp.CallToolResult, GetNetworkStatsOutput, error) {
	log.Printf("[get_network_stats] Reading network interface statistics")

	// Read /proc/net/dev
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return &mcp.CallToolResult{}, GetNetworkStatsOutput{
			Error: fmt.Sprintf("failed to open /proc/net/dev: %v", err),
		}, nil
	}
	defer file.Close()

	var interfaces []NetworkInterfaceStats
	scanner := bufio.NewScanner(file)

	// Skip first two header lines
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()

		// Split on colon to separate interface name from stats
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		interfaceName := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])

		if len(fields) < 16 {
			continue
		}

		// Parse receive stats
		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
		rxErrors, _ := strconv.ParseUint(fields[2], 10, 64)
		rxDropped, _ := strconv.ParseUint(fields[3], 10, 64)

		// Parse transmit stats (offset by 8)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[9], 10, 64)
		txErrors, _ := strconv.ParseUint(fields[10], 10, 64)
		txDropped, _ := strconv.ParseUint(fields[11], 10, 64)

		interfaces = append(interfaces, NetworkInterfaceStats{
			Interface: interfaceName,
			RxBytes:   rxBytes,
			RxPackets: rxPackets,
			RxErrors:  rxErrors,
			RxDropped: rxDropped,
			TxBytes:   txBytes,
			TxPackets: txPackets,
			TxErrors:  txErrors,
			TxDropped: txDropped,
		})

		// Limit to 100 interfaces
		if len(interfaces) >= 100 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return &mcp.CallToolResult{}, GetNetworkStatsOutput{
			Error: fmt.Sprintf("error reading /proc/net/dev: %v", err),
		}, nil
	}

	log.Printf("[get_network_stats] Found %d network interfaces", len(interfaces))

	return &mcp.CallToolResult{}, GetNetworkStatsOutput{
		Interfaces: interfaces,
	}, nil
}

// Register registers the tool with the MCP server
func (t *GetNetworkStatsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_network_stats",
		Description: "Get network interface statistics including bytes, packets, errors, and drops for both receive and transmit. Reads from /proc/net/dev.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
