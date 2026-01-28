package network

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetRoutingTableInput defines input parameters
type GetRoutingTableInput struct{}

// Route represents a single routing entry
type Route struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"`
	Metric      int    `json:"metric"`
	Flags       string `json:"flags"`
}

// GetRoutingTableOutput contains routing table information
type GetRoutingTableOutput struct {
	Routes []Route `json:"routes,omitempty"`
	Error  string  `json:"error,omitempty"`
}

// GetRoutingTableTool provides routing table information
type GetRoutingTableTool struct{}

// NewGetRoutingTableTool creates a new routing table tool
func NewGetRoutingTableTool() *GetRoutingTableTool {
	return &GetRoutingTableTool{}
}

// Handler implements the routing table tool
func (t *GetRoutingTableTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input GetRoutingTableInput,
) (*mcp.CallToolResult, GetRoutingTableOutput, error) {
	log.Printf("[get_routing_table] Reading routing tables")

	var routes []Route

	// Parse IPv4 routes from /proc/net/route
	ipv4Routes, err := parseIPv4Routes()
	if err != nil {
		log.Printf("[get_routing_table] Warning: failed to parse IPv4 routes: %v", err)
	} else {
		routes = append(routes, ipv4Routes...)
	}

	// Parse IPv6 routes from /proc/net/ipv6_route
	ipv6Routes, err := parseIPv6Routes()
	if err != nil {
		log.Printf("[get_routing_table] Warning: failed to parse IPv6 routes: %v", err)
	} else {
		routes = append(routes, ipv6Routes...)
	}

	// Limit to 500 routes
	if len(routes) > 500 {
		routes = routes[:500]
	}

	log.Printf("[get_routing_table] Found %d routes", len(routes))

	return &mcp.CallToolResult{}, GetRoutingTableOutput{
		Routes: routes,
	}, nil
}

// parseIPv4Routes parses /proc/net/route for IPv4 routing entries
func parseIPv4Routes() ([]Route, error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/net/route: %v", err)
	}
	defer file.Close()

	var routes []Route
	scanner := bufio.NewScanner(file)

	// Skip header line
	scanner.Scan()

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}

		iface := fields[0]
		destHex := fields[1]
		gatewayHex := fields[2]
		flagsHex := fields[3]
		metricStr := fields[6]

		// Parse destination
		dest := hexToIP(destHex)
		if dest == "" {
			dest = "0.0.0.0"
		}

		// Parse gateway
		gateway := hexToIP(gatewayHex)
		if gateway == "" || gateway == "0.0.0.0" {
			gateway = "*"
		}

		// Parse metric
		metric, _ := strconv.Atoi(metricStr)

		// Parse flags
		flags := parseRouteFlags(flagsHex)

		routes = append(routes, Route{
			Destination: dest,
			Gateway:     gateway,
			Interface:   iface,
			Metric:      metric,
			Flags:       flags,
		})
	}

	return routes, scanner.Err()
}

// parseIPv6Routes parses /proc/net/ipv6_route for IPv6 routing entries
func parseIPv6Routes() ([]Route, error) {
	file, err := os.Open("/proc/net/ipv6_route")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/net/ipv6_route: %v", err)
	}
	defer file.Close()

	var routes []Route
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}

		destHex := fields[0]
		destPrefix := fields[1]
		gatewayHex := fields[4]
		metricStr := fields[5]
		iface := fields[9]

		// Parse destination with prefix
		dest := hexToIPv6(destHex)
		prefix, _ := strconv.Atoi(destPrefix)
		if dest != "" && prefix != 0 {
			dest = fmt.Sprintf("%s/%d", dest, prefix)
		} else if dest == "" {
			dest = "::/0"
		}

		// Parse gateway
		gateway := hexToIPv6(gatewayHex)
		if gateway == "" || gateway == "::" {
			gateway = "*"
		}

		// Parse metric
		metricHex := metricStr
		metric64, _ := strconv.ParseInt(metricHex, 16, 64)
		metric := int(metric64)

		routes = append(routes, Route{
			Destination: dest,
			Gateway:     gateway,
			Interface:   iface,
			Metric:      metric,
			Flags:       "U", // IPv6 routes don't have flag field in same format
		})
	}

	return routes, scanner.Err()
}

// hexToIP converts hex string to IPv4 address string
func hexToIP(hexStr string) string {
	// Parse hex value
	val, err := strconv.ParseUint(hexStr, 16, 32)
	if err != nil {
		return ""
	}

	// Convert to IP (little-endian format)
	ip := net.IPv4(
		byte(val&0xFF),
		byte((val>>8)&0xFF),
		byte((val>>16)&0xFF),
		byte((val>>24)&0xFF),
	)

	return ip.String()
}

// hexToIPv6 converts hex string to IPv6 address string
func hexToIPv6(hexStr string) string {
	if len(hexStr) != 32 {
		return ""
	}

	// Parse into 16 bytes
	ip := make(net.IP, 16)
	for i := 0; i < 16; i++ {
		val, err := strconv.ParseUint(hexStr[i*2:i*2+2], 16, 8)
		if err != nil {
			return ""
		}
		ip[i] = byte(val)
	}

	return ip.String()
}

// parseRouteFlags converts hex flags to string representation
func parseRouteFlags(flagsHex string) string {
	flags, err := strconv.ParseUint(flagsHex, 16, 32)
	if err != nil {
		return ""
	}

	var result []rune

	// RTF_UP
	if flags&0x0001 != 0 {
		result = append(result, 'U')
	}
	// RTF_GATEWAY
	if flags&0x0002 != 0 {
		result = append(result, 'G')
	}
	// RTF_HOST
	if flags&0x0004 != 0 {
		result = append(result, 'H')
	}

	return string(result)
}

// Register registers the tool with the MCP server
func (t *GetRoutingTableTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "get_routing_table",
		Description: "Get IPv4 and IPv6 routing table entries including destination, gateway, interface, and metrics. Reads from /proc/net/route and /proc/net/ipv6_route.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
