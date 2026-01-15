package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CheckConnectivityInput defines the input schema
type CheckConnectivityInput struct {
	Host    string  `json:"host" jsonschema:"Hostname or IP address to check connectivity to"`
	Port    *int    `json:"port,omitempty" jsonschema:"Port to check (default: 80 for HTTP)"`
	Timeout *int    `json:"timeout,omitempty" jsonschema:"Timeout in seconds (default: 5)"`
}

// CheckConnectivityOutput defines the output structure
type CheckConnectivityOutput struct {
	Host       string  `json:"host"`
	Port       int     `json:"port,omitempty"`
	Reachable  bool    `json:"reachable"`
	Latency    float64 `json:"latency_ms,omitempty"` // in milliseconds
	ResolvedIP string  `json:"resolved_ip,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// CheckConnectivityTool checks connectivity to a host
type CheckConnectivityTool struct{}

func NewCheckConnectivityTool() *CheckConnectivityTool {
	return &CheckConnectivityTool{}
}

func (t *CheckConnectivityTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input CheckConnectivityInput,
) (
	*mcp.CallToolResult,
	CheckConnectivityOutput,
	error,
) {
	port := 80
	if input.Port != nil {
		port = *input.Port
	}

	timeout := 5 * time.Second
	if input.Timeout != nil {
		timeout = time.Duration(*input.Timeout) * time.Second
	}

	log.Printf("[check_connectivity] Checking connectivity to %s:%d", input.Host, port)

	output := CheckConnectivityOutput{
		Host:      input.Host,
		Port:      port,
		Reachable: false,
	}

	// Try to resolve the hostname first
	ips, err := net.LookupIP(input.Host)
	if err != nil {
		output.Error = fmt.Sprintf("DNS resolution failed: %v", err)
		log.Printf("[check_connectivity] DNS resolution failed for %s: %v", input.Host, err)
		return &mcp.CallToolResult{}, output, nil
	}

	if len(ips) > 0 {
		output.ResolvedIP = ips[0].String()
	}

	// Try to connect
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", input.Host, port), timeout)
	latency := time.Since(start)

	if err != nil {
		output.Error = fmt.Sprintf("connection failed: %v", err)
		log.Printf("[check_connectivity] Connection to %s:%d failed: %v", input.Host, port, err)
		return &mcp.CallToolResult{}, output, nil
	}

	conn.Close()
	output.Reachable = true
	output.Latency = float64(latency.Microseconds()) / 1000.0 // Convert to milliseconds

	log.Printf("[check_connectivity] %s:%d is reachable (%.2fms)", input.Host, port, output.Latency)

	return &mcp.CallToolResult{}, output, nil
}

func (t *CheckConnectivityTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "check_connectivity",
		Description: "Check network connectivity to a host. Performs DNS resolution and TCP connection test. Returns reachability, latency, and resolved IP.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
