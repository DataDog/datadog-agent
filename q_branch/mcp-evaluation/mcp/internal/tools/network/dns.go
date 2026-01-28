package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ResolveDNSInput defines input parameters
type ResolveDNSInput struct {
	Hostname   string `json:"hostname" jsonschema:"Hostname to resolve"`
	RecordType string `json:"record_type" jsonschema:"Record type: A (default), AAAA, CNAME, MX, TXT, NS"`
}

// DNSRecord represents a DNS resolution result
type DNSRecord struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ResolveDNSOutput contains DNS resolution results
type ResolveDNSOutput struct {
	Hostname     string      `json:"hostname"`
	Records      []DNSRecord `json:"records,omitempty"`
	ResponseTime int64       `json:"response_time_ms"`
	Error        string      `json:"error,omitempty"`
}

// ResolveDNSTool provides DNS resolution functionality
type ResolveDNSTool struct{}

// NewResolveDNSTool creates a new DNS resolution tool
func NewResolveDNSTool() *ResolveDNSTool {
	return &ResolveDNSTool{}
}

// Handler implements the DNS resolution tool
func (t *ResolveDNSTool) Handler(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input ResolveDNSInput,
) (*mcp.CallToolResult, ResolveDNSOutput, error) {
	log.Printf("[resolve_dns] Resolving %s (type: %s)", input.Hostname, input.RecordType)

	// Set default record type
	recordType := input.RecordType
	if recordType == "" {
		recordType = "A"
	}

	// Create resolver with timeout
	resolver := &net.Resolver{
		PreferGo: true,
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Track response time
	startTime := time.Now()

	var records []DNSRecord
	var lookupErr error

	// Perform lookup based on record type
	switch recordType {
	case "A":
		ips, err := resolver.LookupIP(lookupCtx, "ip4", input.Hostname)
		lookupErr = err
		for _, ip := range ips {
			records = append(records, DNSRecord{
				Type:  "A",
				Value: ip.String(),
			})
		}

	case "AAAA":
		ips, err := resolver.LookupIP(lookupCtx, "ip6", input.Hostname)
		lookupErr = err
		for _, ip := range ips {
			records = append(records, DNSRecord{
				Type:  "AAAA",
				Value: ip.String(),
			})
		}

	case "CNAME":
		cname, err := resolver.LookupCNAME(lookupCtx, input.Hostname)
		lookupErr = err
		if err == nil {
			records = append(records, DNSRecord{
				Type:  "CNAME",
				Value: cname,
			})
		}

	case "MX":
		mxRecords, err := resolver.LookupMX(lookupCtx, input.Hostname)
		lookupErr = err
		for _, mx := range mxRecords {
			records = append(records, DNSRecord{
				Type:  "MX",
				Value: fmt.Sprintf("%d %s", mx.Pref, mx.Host),
			})
		}

	case "TXT":
		txtRecords, err := resolver.LookupTXT(lookupCtx, input.Hostname)
		lookupErr = err
		for _, txt := range txtRecords {
			records = append(records, DNSRecord{
				Type:  "TXT",
				Value: txt,
			})
		}

	case "NS":
		nsRecords, err := resolver.LookupNS(lookupCtx, input.Hostname)
		lookupErr = err
		for _, ns := range nsRecords {
			records = append(records, DNSRecord{
				Type:  "NS",
				Value: ns.Host,
			})
		}

	default:
		return &mcp.CallToolResult{}, ResolveDNSOutput{
			Hostname: input.Hostname,
			Error:    fmt.Sprintf("unsupported record type: %s", recordType),
		}, nil
	}

	responseTime := time.Since(startTime).Milliseconds()

	if lookupErr != nil {
		return &mcp.CallToolResult{}, ResolveDNSOutput{
			Hostname:     input.Hostname,
			ResponseTime: responseTime,
			Error:        fmt.Sprintf("DNS lookup failed: %v", lookupErr),
		}, nil
	}

	log.Printf("[resolve_dns] Resolved %s: found %d records in %dms",
		input.Hostname, len(records), responseTime)

	return &mcp.CallToolResult{}, ResolveDNSOutput{
		Hostname:     input.Hostname,
		Records:      records,
		ResponseTime: responseTime,
	}, nil
}

// Register registers the tool with the MCP server
func (t *ResolveDNSTool) Register(server *mcp.Server) error {
	tool := &mcp.Tool{
		Name:        "resolve_dns",
		Description: "Resolve DNS records for a hostname including A, AAAA, CNAME, MX, TXT, and NS records. Uses Go's net.Resolver with 10s timeout.",
	}

	mcp.AddTool(server, tool, t.Handler)
	return nil
}
