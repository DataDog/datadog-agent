// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package payload contains Network Path payload
package payload

import (
	"net"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/payload"
)

// Protocol defines supported network protocols
// Please define new protocols based on the Keyword from:
// https://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml
type Protocol string

const (
	// ProtocolTCP is the TCP protocol.
	ProtocolTCP Protocol = "TCP"
	// ProtocolUDP is the UDP protocol.
	ProtocolUDP Protocol = "UDP"
	// ProtocolICMP is the ICMP protocol.
	ProtocolICMP Protocol = "ICMP"
)

// TCPMethod is the method used to run a TCP traceroute.
type TCPMethod string

const (
	// TCPConfigSYN means to only perform SYN traceroutes
	TCPConfigSYN TCPMethod = "syn"
	// TCPConfigSACK means to only perform SACK traceroutes
	TCPConfigSACK TCPMethod = "sack"
	// TCPConfigPreferSACK means to try SACK, and fall back to SYN if the remote doesn't support SACK
	TCPConfigPreferSACK TCPMethod = "prefer_sack"
	// TCPConfigSYNSocket means to use a SYN with TCP socket options to perform the traceroute (windows only)
	TCPConfigSYNSocket TCPMethod = "syn_socket"
)

// TCPDefaultMethod is what method to use when nothing is specified
const TCPDefaultMethod TCPMethod = TCPConfigSYN

// MakeTCPMethod converts a TCP traceroute method from config into a TCPMethod
func MakeTCPMethod(method string) TCPMethod {
	return TCPMethod(strings.ToLower(method))
}

// ICMPMode determines whether dynamic paths will run ICMP traceroutes for TCP/UDP traffic
// (instead of TCP/UDP traceroutes)
type ICMPMode string

const (
	// ICMPModeNone means to never use ICMP in dynamic path
	ICMPModeNone ICMPMode = "none"
	// ICMPModeTCP means to replace TCP traceroutes with ICMP
	ICMPModeTCP ICMPMode = "tcp"
	// ICMPModeUDP means to replace UDP traceroutes with ICMP
	ICMPModeUDP ICMPMode = "udp"
	// ICMPModeAll means to replace all traceroutes with ICMP
	ICMPModeAll ICMPMode = "all"
)

// ICMPDefaultMode is what mode to use when nothing is specified
const ICMPDefaultMode ICMPMode = ICMPModeNone

// MakeICMPMode converts config strings into ICMPModes
func MakeICMPMode(method string) ICMPMode {
	return ICMPMode(strings.ToLower(method))
}

// ShouldUseICMP returns whether ICMP mode should overwrite the given protocol
func (m ICMPMode) ShouldUseICMP(protocol Protocol) bool {
	if protocol == ProtocolICMP {
		return true
	}
	switch m {
	case ICMPModeNone:
		return false
	case ICMPModeTCP:
		return protocol == ProtocolTCP
	case ICMPModeUDP:
		return protocol == ProtocolUDP
	case ICMPModeAll:
		return true
	default:
		// should not get here
		return false
	}
}

// PathOrigin origin of the path e.g. network_traffic, network_path_integration
type PathOrigin string

const (
	// PathOriginNetworkTraffic correspond to traffic from network traffic (NPM).
	PathOriginNetworkTraffic PathOrigin = "network_traffic"
	// PathOriginNetworkPathIntegration correspond to traffic from network_path integration.
	PathOriginNetworkPathIntegration PathOrigin = "network_path_integration"
	// PathOriginSynthetics correspond to traffic from synthetics.
	PathOriginSynthetics PathOrigin = "synthetics"
)

// TestRunType defines the type of test run
type TestRunType string

const (
	// TestRunTypeScheduled is a scheduled test run.
	TestRunTypeScheduled TestRunType = "scheduled"
	// TestRunTypeDynamic is a dynamic test run.
	TestRunTypeDynamic TestRunType = "dynamic"
	// TestRunTypeTriggered is a triggered test run.
	TestRunTypeTriggered TestRunType = "triggered"
)

// SourceProduct defines the product that originated the path
type SourceProduct string

const (
	// SourceProductNetworkPath is the network path product.
	SourceProductNetworkPath SourceProduct = "network_path"
	// SourceProductSynthetics is the synthetics product.
	SourceProductSynthetics SourceProduct = "synthetics"
	// SourceProductEndUserDevice is the end user device monitoring product.
	SourceProductEndUserDevice SourceProduct = "end_user_device"
)

// GetSourceProduct returns the appropriate SourceProduct based on infrastructure mode.
// If infraMode is "end_user_device", returns SourceProductEndUserDevice.
// Otherwise, returns SourceProductNetworkPath.
func GetSourceProduct(infraMode string) SourceProduct {
	if infraMode == "end_user_device" {
		return SourceProductEndUserDevice
	}
	return SourceProductNetworkPath
}

// CollectorType defines the type of collector
type CollectorType string

const (
	// CollectorTypeAgent is an agent collector.
	CollectorTypeAgent CollectorType = "agent"
	// CollectorTypeManagedLocation is a managed location collector.
	CollectorTypeManagedLocation CollectorType = "managed_location"
)

// NetworkPathSource encapsulates information
// about the source of a path
type NetworkPathSource struct {
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name"`
	Hostname    string       `json:"hostname"`
	Via         *payload.Via `json:"via,omitempty"`
	NetworkID   string       `json:"network_id,omitempty"` // Today this will be a VPC ID since we only resolve AWS resources
	Service     string       `json:"service,omitempty"`
	ContainerID string       `json:"container_id,omitempty"`
	PublicIP    string       `json:"public_ip,omitempty"`
}

// NetworkPathDestination encapsulates information
// about the destination of a path
type NetworkPathDestination struct {
	Hostname string `json:"hostname"`
	Port     uint16 `json:"port"`
	Service  string `json:"service,omitempty"`
}

// E2eProbe contains e2e probe results
type E2eProbe struct {
	RTTs                 []float64          `json:"rtts"` // ms
	PacketsSent          int                `json:"packets_sent"`
	PacketsReceived      int                `json:"packets_received"`
	PacketLossPercentage float32            `json:"packet_loss_percentage"`
	Jitter               float64            `json:"jitter"` // ms
	RTT                  E2eProbeRttLatency `json:"rtt"`    // ms
}

// E2eProbeRttLatency contains e2e latency stats
type E2eProbeRttLatency struct {
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// HopCountStats contains hop count stats
type HopCountStats struct {
	Avg float64 `json:"avg"`
	Min int     `json:"min"`
	Max int     `json:"max"`
}

// Traceroute contains traceroute results
type Traceroute struct {
	Runs     []TracerouteRun `json:"runs"`
	HopCount HopCountStats   `json:"hop_count"`
}

// TracerouteRun contains traceroute results for a single run
type TracerouteRun struct {
	RunID       string                `json:"run_id"`
	Source      TracerouteSource      `json:"source"`
	Destination TracerouteDestination `json:"destination"`
	Hops        []TracerouteHop       `json:"hops"`
}

// TracerouteHop encapsulates information about a single
// hop in a traceroute
type TracerouteHop struct {
	TTL        int      `json:"ttl"`
	IPAddress  net.IP   `json:"ip_address"`
	ReverseDNS []string `json:"reverse_dns,omitempty"`
	RTT        float64  `json:"rtt,omitempty"`
	Reachable  bool     `json:"reachable"`
}

// TracerouteSource contains result source info
type TracerouteSource struct {
	IPAddress net.IP `json:"ip_address"`
	Port      uint16 `json:"port"`
}

// TracerouteDestination contains result destination info
type TracerouteDestination struct {
	IPAddress  net.IP   `json:"ip_address"`
	Port       uint16   `json:"port"`
	ReverseDNS []string `json:"reverse_dns,omitempty"`
}

// NetworkPath encapsulates data that defines a
// path between two hosts as mapped by the agent
type NetworkPath struct {
	Timestamp     int64                  `json:"timestamp"`
	AgentVersion  string                 `json:"agent_version"`
	Namespace     string                 `json:"namespace"`      // namespace used to resolve NDM resources
	TestConfigID  string                 `json:"test_config_id"` // ID represent the test configuration created in UI/backend/Agent
	TestResultID  string                 `json:"test_result_id"` // ID of specific test result (test run)
	TestRunID     string                 `json:"test_run_id"`
	Origin        PathOrigin             `json:"origin"`
	TestRunType   TestRunType            `json:"test_run_type"`
	SourceProduct SourceProduct          `json:"source_product"`
	CollectorType CollectorType          `json:"collector_type"`
	Protocol      Protocol               `json:"protocol"`
	Source        NetworkPathSource      `json:"source"`
	Destination   NetworkPathDestination `json:"destination"`
	Traceroute    Traceroute             `json:"traceroute"`
	E2eProbe      E2eProbe               `json:"e2e_probe"`
	Tags          []string               `json:"tags,omitempty"`
}
