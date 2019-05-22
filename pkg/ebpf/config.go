package ebpf

import (
	"time"
)

// Config stores all flags used by the eBPF tracer
type Config struct {
	// CollectTCPConns specifies whether the tracer should collect traffic statistics for TCP connections
	CollectTCPConns bool

	// CollectUDPConns specifies whether the tracer should collect traffic statistics for UDP connections
	CollectUDPConns bool

	// CollectIPv6Conns specifics whether the tracer should capture traffic for IPv6 TCP/UDP connections
	CollectIPv6Conns bool

	// CollectLocalDNS specifies whether the tracer should capture traffic for local DNS calls
	CollectLocalDNS bool

	// UDPConnTimeout determines the length of traffic inactivity between two (IP, port)-pairs before declaring a UDP
	// connection as inactive.
	// Note: As UDP traffic is technically "connection-less", for tracking, we consider a UDP connection to be traffic
	//       between a source and destination IP and port.
	UDPConnTimeout time.Duration

	// TCPConnTimeout is like UDPConnTimeout, but for TCP connections. TCP connections are cleared when
	// the BPF module receives a tcp_close call, but TCP connections also age out to catch cases where
	// tcp_close is not intercepted for some reason.
	TCPConnTimeout time.Duration

	// MaxTrackedConnections specifies the maximum number of connections we can track, this will be the size of the BPF maps
	MaxTrackedConnections uint

	// ProcRoot is the root path to the proc filesystem
	ProcRoot string

	// BPFDebug enables bpf debug logs
	BPFDebug bool

	// EnableConntrack enables probing conntrack for network address translation via netlink
	EnableConntrack bool

	// ConntrackShortTermBufferSize is the maximum number of short term conntracked connections that will
	// held in memory at once
	ConntrackShortTermBufferSize int

	// ExpVarPort specifies a port to run golang's expvar debug endpoint
	ExpVarPort int
}

// NewDefaultConfig enables traffic collection for all connection types
func NewDefaultConfig() *Config {
	return &Config{
		CollectTCPConns:       true,
		CollectUDPConns:       true,
		CollectIPv6Conns:      true,
		CollectLocalDNS:       false,
		UDPConnTimeout:        30 * time.Second,
		TCPConnTimeout:        2 * time.Minute,
		MaxTrackedConnections: 65536,
		ProcRoot:              "/proc",
		BPFDebug:              false,
		EnableConntrack:       true,
	}
}

// EnabledKProbes returns a map of kprobes that are enabled per config settings
func (c *Config) EnabledKProbes() map[KProbeName]struct{} {
	enabled := make(map[KProbeName]struct{}, 0)

	// Note: TCPv4Connect & TCPv4ConnectReturn are always included as they're needed for initialization
	// and can be disabled after field offset guessing has completed.
	enabled[TCPv4Connect] = struct{}{}
	enabled[TCPv4ConnectReturn] = struct{}{}

	if c.CollectTCPConns {
		enabled[TCPSendMsg] = struct{}{}
		enabled[TCPCleanupRBuf] = struct{}{}
		enabled[TCPClose] = struct{}{}
		enabled[TCPRetransmit] = struct{}{}
		enabled[InetCskAcceptReturn] = struct{}{}
		enabled[TCPv4DestroySock] = struct{}{}
	}

	if c.CollectUDPConns {
		enabled[UDPRecvMsgReturn] = struct{}{}
		enabled[UDPRecvMsg] = struct{}{}
		enabled[UDPSendMsg] = struct{}{}
	}

	if c.CollectIPv6Conns {
		enabled[TCPv6Connect] = struct{}{}
		enabled[TCPv6ConnectReturn] = struct{}{}
	}

	return enabled
}
