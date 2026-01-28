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

// GetListeningPortsInput defines the input schema
type GetListeningPortsInput struct{}

// ListeningPortInfo represents a listening port
type ListeningPortInfo struct {
	Protocol    string `json:"protocol"`
	LocalAddr   string `json:"local_addr"`
	LocalPort   int    `json:"local_port"`
	PID         int    `json:"pid,omitempty"`
	ProcessName string `json:"process_name,omitempty"`
}

// GetListeningPortsOutput defines the output structure
type GetListeningPortsOutput struct {
	Ports []ListeningPortInfo `json:"ports,omitempty"`
	Count int                 `json:"count"`
	Error string              `json:"error,omitempty"`
}

// GetListeningPortsTool gets listening network ports
type GetListeningPortsTool struct{}

func NewGetListeningPortsTool() *GetListeningPortsTool {
	return &GetListeningPortsTool{}
}

func parseHexAddr(hexAddr string) (string, int) {
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

func (t *GetListeningPortsTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetListeningPortsInput,
) (
	*mcp.CallToolResult,
	GetListeningPortsOutput,
	error,
) {
	log.Printf("[get_listening_ports] Getting listening ports")

	var ports []ListeningPortInfo

	// Parse TCP listening ports
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
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}

			// Field 3 is the state: 0A = LISTEN (TCP_LISTEN)
			state := fields[3]
			if state != "0A" {
				continue
			}

			localAddr, localPort := parseHexAddr(fields[1])

			port := ListeningPortInfo{
				Protocol:  "tcp",
				LocalAddr: localAddr,
				LocalPort: localPort,
			}

			ports = append(ports, port)
		}
	}

	// Parse UDP (all UDP sockets are "listening")
	udpFiles := []string{"/proc/net/udp", "/proc/net/udp6"}
	for _, filename := range udpFiles {
		file, err := os.Open(filename)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Scan() // Skip header

		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}

			localAddr, localPort := parseHexAddr(fields[1])

			port := ListeningPortInfo{
				Protocol:  "udp",
				LocalAddr: localAddr,
				LocalPort: localPort,
			}

			ports = append(ports, port)
		}
	}

	log.Printf("[get_listening_ports] Found %d listening ports", len(ports))

	return &mcp.CallToolResult{}, GetListeningPortsOutput{
		Ports: ports,
		Count: len(ports),
	}, nil
}

func (t *GetListeningPortsTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_listening_ports",
		Description: "Get listening network ports (TCP/UDP). Returns protocol, local address, and port. For complete connection info, use get_network_connections.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
