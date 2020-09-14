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

	// DNSInspection specifies whether the tracer should enhance connection data with domain names by inspecting DNS traffic
	// Notice this does *not* depend on CollectLocalDNS
	DNSInspection bool

	// CollectDNSStats specifies whether the tracer should enhance connection data with relevant DNS stats
	// It is relevant *only* when DNSInspection is enabled.
	CollectDNSStats bool

	// DNSTimeout determines the length of time to wait before considering a DNS Query to have timed out
	DNSTimeout time.Duration

	// UDPConnTimeout determines the length of traffic inactivity between two (IP, port)-pairs before declaring a UDP
	// connection as inactive.
	// Note: As UDP traffic is technically "connection-less", for tracking, we consider a UDP connection to be traffic
	//       between a source and destination IP and port.
	UDPConnTimeout time.Duration

	// TCPConnTimeout is like UDPConnTimeout, but for TCP connections. TCP connections are cleared when
	// the BPF module receives a tcp_close call, but TCP connections also age out to catch cases where
	// tcp_close is not intercepted for some reason.
	TCPConnTimeout time.Duration

	// TCPClosedTimeout represents the maximum amount of time a closed TCP connection can remain buffered in eBPF before
	// being marked as idle and flushed to the perf ring.
	TCPClosedTimeout time.Duration

	// MaxTrackedConnections specifies the maximum number of connections we can track. This determines the size of the eBPF Maps
	MaxTrackedConnections uint

	// MaxClosedConnectionsBuffered represents the maximum number of closed connections we'll buffer in memory. These closed connections
	// get flushed on every client request (default 30s check interval)
	MaxClosedConnectionsBuffered int

	// MaxDNSStatsBufferred represents the maximum number of DNS stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxDNSStatsBufferred int

	// MaxConnectionsStateBuffered represents the maximum number of state objects that we'll store in memory. These state objects store
	// the stats for a connection so we can accurately determine traffic change between client requests.
	MaxConnectionsStateBuffered int

	// ClientStateExpiry specifies the max time a client (e.g. process-agent)'s state will be stored in memory before being evicted.
	ClientStateExpiry time.Duration

	// ProcRoot is the root path to the proc filesystem
	ProcRoot string

	// BPFDebug enables bpf debug logs
	BPFDebug bool

	// BPFDir is the directory to load the eBPF program from
	BPFDir string

	// EnableConntrack enables probing conntrack for network address translation via netlink
	EnableConntrack bool

	// ConntrackMaxStateSize specifies the maximum number of connections with NAT we can track
	ConntrackMaxStateSize int

	// ConntrackRateLimit specifies the maximum number of netlink messages *per second* that can be processed
	// Setting it to -1 disables the limit and can result in a high CPU usage.
	ConntrackRateLimit int

	// DebugPort specifies a port to run golang's expvar and pprof debug endpoint
	DebugPort int

	// ClosedChannelSize specifies the size for closed channel for the tracer
	ClosedChannelSize int

	// ExcludedSourceConnections is a map of source connections to blacklist
	ExcludedSourceConnections map[string][]string

	// ExcludedDestinationConnections is a map of destination connections to blacklist
	ExcludedDestinationConnections map[string][]string

	// OffsetGuessThreshold is the size of the byte threshold we will iterate over when guessing offsets
	OffsetGuessThreshold uint64

	// EnableTracepoints enables use of tracepoints instead of kprobes for probing syscalls (if available on system)
	EnableTracepoints bool

	// EnableMonotonicCount (Windows only) determines if we will calculate send/recv bytes of connections with headers and retransmits
	EnableMonotonicCount bool

	// DriverBufferSize (Windows only) determines the size (in bytes) of the buffer we pass to the driver when reading flows
	DriverBufferSize int
}

// NewDefaultConfig enables traffic collection for all connection types
func NewDefaultConfig() *Config {
	return &Config{
		CollectTCPConns:       true,
		CollectUDPConns:       true,
		CollectIPv6Conns:      true,
		CollectLocalDNS:       false,
		DNSInspection:         true,
		UDPConnTimeout:        30 * time.Second,
		TCPConnTimeout:        2 * time.Minute,
		TCPClosedTimeout:      time.Second,
		MaxTrackedConnections: 65536,
		ConntrackMaxStateSize: 65536,
		ConntrackRateLimit:    500,
		ProcRoot:              "/proc",
		BPFDebug:              false,
		EnableConntrack:       true,
		// With clients checking connection stats roughly every 30s, this gives us roughly ~1.6k + ~2.5k objects a second respectively.
		MaxClosedConnectionsBuffered: 50000,
		MaxConnectionsStateBuffered:  75000,
		MaxDNSStatsBufferred:         75000,
		ClientStateExpiry:            2 * time.Minute,
		ClosedChannelSize:            500,
		// DNS Stats related configurations
		CollectDNSStats:      false,
		DNSTimeout:           15 * time.Second,
		OffsetGuessThreshold: 400,
		EnableMonotonicCount: false,
	}
}
