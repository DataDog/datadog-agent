package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetNetworkInterfacesInput defines the input schema
type GetNetworkInterfacesInput struct{}

// InterfaceInfo represents information about a network interface
type InterfaceInfo struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	IPAddresses  []string `json:"ip_addresses,omitempty"`
	MACAddress   string   `json:"mac_address,omitempty"`
	MTU          int      `json:"mtu"`
	RxBytes      int64    `json:"rx_bytes"`
	TxBytes      int64    `json:"tx_bytes"`
	RxPackets    int64    `json:"rx_packets"`
	TxPackets    int64    `json:"tx_packets"`
}

// GetNetworkInterfacesOutput defines the output structure
type GetNetworkInterfacesOutput struct {
	Interfaces []InterfaceInfo `json:"interfaces,omitempty"`
	Count      int             `json:"count"`
	Error      string          `json:"error,omitempty"`
}

// GetNetworkInterfacesTool gets network interface information
type GetNetworkInterfacesTool struct{}

func NewGetNetworkInterfacesTool() *GetNetworkInterfacesTool {
	return &GetNetworkInterfacesTool{}
}

func (t *GetNetworkInterfacesTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetNetworkInterfacesInput,
) (
	*mcp.CallToolResult,
	GetNetworkInterfacesOutput,
	error,
) {
	log.Printf("[get_network_interfaces] Getting network interface information")

	ifaces, err := net.Interfaces()
	if err != nil {
		return &mcp.CallToolResult{}, GetNetworkInterfacesOutput{
			Error: fmt.Sprintf("failed to get interfaces: %v", err),
		}, nil
	}

	var interfaces []InterfaceInfo

	for _, iface := range ifaces {
		info := InterfaceInfo{
			Name:       iface.Name,
			Status:     "down",
			MACAddress: iface.HardwareAddr.String(),
			MTU:        iface.MTU,
		}

		// Check if interface is up
		if iface.Flags&net.FlagUp != 0 {
			info.Status = "up"
		}

		// Get IP addresses
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				info.IPAddresses = append(info.IPAddresses, addr.String())
			}
		}

		// Read statistics from /sys/class/net
		statsPath := filepath.Join("/sys/class/net", iface.Name, "statistics")
		if rxBytes, err := os.ReadFile(filepath.Join(statsPath, "rx_bytes")); err == nil {
			info.RxBytes, _ = strconv.ParseInt(strings.TrimSpace(string(rxBytes)), 10, 64)
		}
		if txBytes, err := os.ReadFile(filepath.Join(statsPath, "tx_bytes")); err == nil {
			info.TxBytes, _ = strconv.ParseInt(strings.TrimSpace(string(txBytes)), 10, 64)
		}
		if rxPackets, err := os.ReadFile(filepath.Join(statsPath, "rx_packets")); err == nil {
			info.RxPackets, _ = strconv.ParseInt(strings.TrimSpace(string(rxPackets)), 10, 64)
		}
		if txPackets, err := os.ReadFile(filepath.Join(statsPath, "tx_packets")); err == nil {
			info.TxPackets, _ = strconv.ParseInt(strings.TrimSpace(string(txPackets)), 10, 64)
		}

		interfaces = append(interfaces, info)
	}

	log.Printf("[get_network_interfaces] Found %d interfaces", len(interfaces))

	return &mcp.CallToolResult{}, GetNetworkInterfacesOutput{
		Interfaces: interfaces,
		Count:      len(interfaces),
	}, nil
}

func (t *GetNetworkInterfacesTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_network_interfaces",
		Description: "Get network interface information including IP addresses, MAC, status, and statistics (bytes/packets sent/received).",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
