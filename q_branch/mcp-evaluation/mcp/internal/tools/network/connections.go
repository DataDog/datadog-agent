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

// parseHexAddr parses hex-encoded address from /proc/net files
func parseHexAddrConn(hexAddr string) (string, int) {
	parts := strings.Split(hexAddr, ":")
	if len(parts) != 2 {
		return "", 0
	}

	// Parse IP (hex format, reversed bytes)
	ipHex := parts[0]
	var ip []byte
	for i := len(ipHex); i > 0; i -= 2 {
		b, _ := strconv.ParseUint(ipHex[i-2:i], 16, 8)
		ip = append(ip, byte(b))
	}

	ipStr := fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
	port, _ := strconv.ParseInt(parts[1], 16, 32)

	return ipStr, int(port)
}

// GetNetworkConnectionsInput defines the input schema
type GetNetworkConnectionsInput struct {
	Limit *int `json:"limit,omitempty" jsonschema:"Maximum number of connections to return (default: 100, max: 500)"`
}

// ConnectionInfo represents a network connection
type ConnectionInfo struct {
	Protocol   string `json:"protocol"`
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr"`
	RemotePort int    `json:"remote_port"`
	State      string `json:"state"`
}

// GetNetworkConnectionsOutput defines the output structure
type GetNetworkConnectionsOutput struct {
	Connections []ConnectionInfo `json:"connections,omitempty"`
	Count       int              `json:"count"`
	Error       string           `json:"error,omitempty"`
}

// GetNetworkConnectionsTool gets active network connections
type GetNetworkConnectionsTool struct{}

func NewGetNetworkConnectionsTool() *GetNetworkConnectionsTool {
	return &GetNetworkConnectionsTool{}
}

var tcpStates = map[string]string{
	"01": "ESTABLISHED",
	"02": "SYN_SENT",
	"03": "SYN_RECV",
	"04": "FIN_WAIT1",
	"05": "FIN_WAIT2",
	"06": "TIME_WAIT",
	"07": "CLOSE",
	"08": "CLOSE_WAIT",
	"09": "LAST_ACK",
	"0A": "LISTEN",
	"0B": "CLOSING",
}

func (t *GetNetworkConnectionsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetNetworkConnectionsInput,
) (
	*mcp.CallToolResult,
	GetNetworkConnectionsOutput,
	error,
) {
	limit := 100
	if input.Limit != nil {
		limit = *input.Limit
		if limit > 500 {
			limit = 500
		}
		if limit < 1 {
			limit = 1
		}
	}

	log.Printf("[get_network_connections] Getting network connections (limit: %d)", limit)

	var connections []ConnectionInfo

	// Parse TCP connections
	tcpFiles := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	for _, filename := range tcpFiles {
		file, err := os.Open(filename)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Scan() // Skip header

		for scanner.Scan() {
			if len(connections) >= limit {
				break
			}

			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}

			localAddr, localPort := parseHexAddrConn(fields[1])
			remoteAddr, remotePort := parseHexAddrConn(fields[2])
			stateHex := fields[3]
			state := tcpStates[stateHex]
			if state == "" {
				state = stateHex
			}

			conn := ConnectionInfo{
				Protocol:   "tcp",
				LocalAddr:  localAddr,
				LocalPort:  localPort,
				RemoteAddr: remoteAddr,
				RemotePort: remotePort,
				State:      state,
			}

			connections = append(connections, conn)
		}
	}

	log.Printf("[get_network_connections] Found %d connections", len(connections))

	return &mcp.CallToolResult{}, GetNetworkConnectionsOutput{
		Connections: connections,
		Count:       len(connections),
	}, nil
}

func (t *GetNetworkConnectionsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_network_connections",
		Description: "Get active network connections. Returns protocol, local/remote addresses, ports, and connection state. For listening ports only, use get_listening_ports.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
