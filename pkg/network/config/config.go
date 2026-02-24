// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements network tracing configuration
package config

import (
	"slices"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	spNS  = "system_probe_config"
	netNS = "network_config"
	smNS  = "service_monitoring_config"
	evNS  = "event_monitoring_config"

	defaultUDPTimeoutSeconds       = 30
	defaultUDPStreamTimeoutSeconds = 120
)

// Config stores all flags used by the network eBPF tracer
type Config struct {
	ebpf.Config

	// NPMEnabled is whether the network performance monitoring feature is explicitly enabled or not
	NPMEnabled bool

	// CollectTCPv4Conns specifies whether the tracer should collect traffic statistics for TCPv4 connections
	CollectTCPv4Conns bool

	// CollectTCPv6Conns specifies whether the tracer should collect traffic statistics for TCPv6 connections
	CollectTCPv6Conns bool

	// CollectUDPv4Conns specifies whether the tracer should collect traffic statistics for UDPv4 connections
	CollectUDPv4Conns bool

	// CollectUDPv6Conns specifies whether the tracer should collect traffic statistics for UDPv6 connections
	CollectUDPv6Conns bool

	// CollectLocalDNS specifies whether the tracer should capture traffic for local DNS calls
	CollectLocalDNS bool

	// DNSInspection specifies whether the tracer should enhance connection data with domain names by inspecting DNS traffic
	// Notice this does *not* depend on CollectLocalDNS
	DNSInspection bool

	// CollectDNSStats specifies whether the tracer should enhance connection data with relevant DNS stats
	// It is relevant *only* when DNSInspection is enabled.
	CollectDNSStats bool

	// CollectDNSDomains specifies whether collected DNS stats would be scoped by domain
	// It is relevant *only* when DNSInspection and CollectDNSStats is enabled.
	CollectDNSDomains bool

	// DNSMonitoringPortList specifies the list of ports to monitor for DNS traffic
	DNSMonitoringPortList []int

	// DNSTimeout determines the length of time to wait before considering a DNS Query to have timed out
	DNSTimeout time.Duration

	// MaxDNSStats determines the number of separate DNS Stats objects DNSStatkeeper can have at any given time
	// These stats objects get flushed on every client request (default 30s check interval)
	MaxDNSStats int

	// Embedded USM configuration
	*USMConfig

	// UDPConnTimeout determines the length of traffic inactivity between two
	// (IP, port)-pairs before declaring a UDP connection as inactive. This is
	// set to /proc/sys/net/netfilter/nf_conntrack_udp_timeout on Linux by
	// default.
	UDPConnTimeout time.Duration

	// UDPStreamTimeout is the timeout for udp streams. This is set to
	// /proc/sys/net/netfilter/nf_conntrack_udp_timeout_stream on Linux by
	// default.
	UDPStreamTimeout time.Duration

	// TCPConnTimeout is like UDPConnTimeout, but for TCP connections. TCP connections are cleared when
	// the BPF module receives a tcp_close call, but TCP connections also age out to catch cases where
	// tcp_close is not intercepted for some reason.
	TCPConnTimeout time.Duration

	// MaxTrackedConnections specifies the maximum number of connections we can track. This determines the size of the eBPF Maps
	MaxTrackedConnections uint32

	// MaxClosedConnectionsBuffered represents the maximum number of closed connections we'll buffer in memory. These closed connections
	// get flushed on every client request (default 30s check interval)
	MaxClosedConnectionsBuffered uint32

	// MaxFailedConnectionsBuffered represents the maximum number of failed connections we'll buffer in memory. These connections will be
	// removed from memory as they are matched to closed connections
	MaxFailedConnectionsBuffered uint32

	// ClosedConnectionFlushThreshold represents the number of closed connections stored before signalling
	// the agent to flush the connections.  This value only valid on Windows
	ClosedConnectionFlushThreshold int

	// MaxDNSStatsBuffered represents the maximum number of DNS stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxDNSStatsBuffered int

	// MaxConnectionsStateBuffered represents the maximum number of state objects that we'll store in memory. These state objects store
	// the stats for a connection so we can accurately determine traffic change between client requests.
	MaxConnectionsStateBuffered int

	// ClientStateExpiry specifies the max time a client (e.g. process-agent)'s state will be stored in memory before being evicted.
	ClientStateExpiry time.Duration

	// EnableConntrack enables probing conntrack for network address translation
	EnableConntrack bool

	// IgnoreConntrackInitFailure will ignore any conntrack initialization failiures during system-probe load. If this is set to false, system-probe
	// will fail to start if there is a conntrack initialization failure.
	IgnoreConntrackInitFailure bool

	// ConntrackMaxStateSize specifies the maximum number of connections with NAT we can track
	ConntrackMaxStateSize int

	// ConntrackRateLimit specifies the maximum number of netlink messages *per second* that can be processed
	// Setting it to -1 disables the limit and can result in a high CPU usage.
	ConntrackRateLimit int

	// ConntrackRateLimitInterval specifies the interval at which the rate limiter is updated
	ConntrackRateLimitInterval time.Duration

	// ConntrackInitTimeout specifies how long we wait for conntrack to initialize before failing
	ConntrackInitTimeout time.Duration

	// EnableConntrackAllNamespaces enables network address translation via netlink for all namespaces that are peers of the root namespace.
	// default is true
	EnableConntrackAllNamespaces bool

	// EnableEbpfConntracker enables the ebpf based network conntracker
	EnableEbpfConntracker bool

	// EnableCiliumLBConntracker enables the cilium load balancer conntracker
	EnableCiliumLBConntracker bool

	// ClosedChannelSize specifies the size for closed channel for the tracer
	ClosedChannelSize int

	// ClosedBufferWakeupCount specifies the number of events that will buffer in a perf buffer before userspace is woken up.
	ClosedBufferWakeupCount int

	// ExcludedSourceConnections is a map of source connections to blacklist
	ExcludedSourceConnections map[string][]string

	// ExcludedDestinationConnections is a map of destination connections to blacklist
	ExcludedDestinationConnections map[string][]string

	// OffsetGuessThreshold is the size of the byte threshold we will iterate over when guessing offsets
	OffsetGuessThreshold uint64

	// EnableMonotonicCount (Windows only) determines if we will calculate send/recv bytes of connections with headers and retransmits
	EnableMonotonicCount bool

	// EnableGatewayLookup enables looking up gateway information for connection destinations
	EnableGatewayLookup bool

	// RecordedQueryTypes enables specific DNS query types to be recorded
	RecordedQueryTypes []string

	// EnableProcessEventMonitoring enables consuming CWS process monitoring events from the runtime security module
	EnableProcessEventMonitoring bool

	// MaxProcessesTracked is the maximum number of processes whose information is stored in the network module
	MaxProcessesTracked int

	// EnableContainerStore enables reading resolv.conf out of container filesystems. Requires EnableProcessEventMonitoring.
	EnableContainerStore bool

	// MaxContainersTracked is the maximum number of containers whose resolv.conf information is stored in the network module
	MaxContainersTracked int

	// EnableRootNetNs disables using the network namespace of the root process (1)
	// for things like creating netlink sockets for conntrack updates, etc.
	EnableRootNetNs bool

	// ProtocolClassificationEnabled specifies whether the tracer should enhance connection data with protocols names by
	// classifying the L7 protocols being used.
	ProtocolClassificationEnabled bool

	// TCPFailedConnectionsEnabled specifies whether the tracer will track & report TCP error codes
	TCPFailedConnectionsEnabled bool

	// EnableNPMConnectionRollup enables aggregating connections by rolling up ephemeral ports
	EnableNPMConnectionRollup bool

	// NPMRingbuffersEnabled specifies whether ringbuffers are enabled or not
	NPMRingbuffersEnabled bool

	// EnableEbpfless enables the use of network tracing without eBPF using packet capture.
	EnableEbpfless bool

	// EnableFentry enables the experimental fentry tracer (disabled by default)
	EnableFentry bool

	// CustomBatchingEnabled enables the use of custom batching for eBPF perf events with perf buffers
	CustomBatchingEnabled bool

	// ExpectedTagsDuration is the duration for which we add host and container tags to our payloads, to handle the race
	// in the backend for processing host/container tags and resolving them in our own pipelines.
	ExpectedTagsDuration time.Duration

	// EnableCertCollection enables the collection of TLS certificates via userspace probing
	EnableCertCollection bool

	// CertCollectionMapCleanerInterval is the interval between eBPF map cleaning for TLS cert collection
	CertCollectionMapCleanerInterval time.Duration

	// DirectSend controls whether we send payloads directly from system-probe or they are queried from process-agent.
	// Not supported on Windows
	DirectSend bool
}

// New creates a config for the network tracer
func New() *Config {
	cfg := pkgconfigsetup.SystemProbe()
	sysconfig.Adjust(cfg)

	c := &Config{
		Config: *ebpf.NewConfig(),

		NPMEnabled: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enabled")),

		CollectTCPv4Conns: cfg.GetBool(sysconfig.FullKeyPath(netNS, "collect_tcp_v4")),
		CollectTCPv6Conns: cfg.GetBool(sysconfig.FullKeyPath(netNS, "collect_tcp_v6")),
		TCPConnTimeout:    2 * time.Minute,

		CollectUDPv4Conns: cfg.GetBool(sysconfig.FullKeyPath(netNS, "collect_udp_v4")),
		CollectUDPv6Conns: cfg.GetBool(sysconfig.FullKeyPath(netNS, "collect_udp_v6")),
		UDPConnTimeout:    defaultUDPTimeoutSeconds * time.Second,
		UDPStreamTimeout:  defaultUDPStreamTimeoutSeconds * time.Second,

		OffsetGuessThreshold:           uint64(cfg.GetInt64(sysconfig.FullKeyPath(spNS, "offset_guess_threshold"))),
		ExcludedSourceConnections:      cfg.GetStringMapStringSlice(sysconfig.FullKeyPath(spNS, "source_excludes")),
		ExcludedDestinationConnections: cfg.GetStringMapStringSlice(sysconfig.FullKeyPath(spNS, "dest_excludes")),

		TCPFailedConnectionsEnabled:    cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_tcp_failed_connections")),
		MaxTrackedConnections:          uint32(cfg.GetInt64(sysconfig.FullKeyPath(spNS, "max_tracked_connections"))),
		MaxClosedConnectionsBuffered:   uint32(cfg.GetInt64(sysconfig.FullKeyPath(spNS, "max_closed_connections_buffered"))),
		MaxFailedConnectionsBuffered:   uint32(cfg.GetInt64(sysconfig.FullKeyPath(netNS, "max_failed_connections_buffered"))),
		ClosedConnectionFlushThreshold: cfg.GetInt(sysconfig.FullKeyPath(netNS, "closed_connection_flush_threshold")),
		ClosedChannelSize:              cfg.GetInt(sysconfig.FullKeyPath(netNS, "closed_channel_size")),
		ClosedBufferWakeupCount:        cfg.GetInt(sysconfig.FullKeyPath(netNS, "closed_buffer_wakeup_count")),
		MaxConnectionsStateBuffered:    cfg.GetInt(sysconfig.FullKeyPath(spNS, "max_connection_state_buffered")),
		ClientStateExpiry:              2 * time.Minute,

		DNSInspection:       !cfg.GetBool(sysconfig.FullKeyPath(spNS, "disable_dns_inspection")),
		CollectDNSStats:     cfg.GetBool(sysconfig.FullKeyPath(spNS, "collect_dns_stats")),
		CollectLocalDNS:     cfg.GetBool(sysconfig.FullKeyPath(spNS, "collect_local_dns")),
		CollectDNSDomains:   cfg.GetBool(sysconfig.FullKeyPath(spNS, "collect_dns_domains")),
		MaxDNSStats:         cfg.GetInt(sysconfig.FullKeyPath(spNS, "max_dns_stats")),
		MaxDNSStatsBuffered: 75000,
		DNSTimeout:          time.Duration(cfg.GetInt(sysconfig.FullKeyPath(spNS, "dns_timeout_in_s"))) * time.Second,

		ProtocolClassificationEnabled: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_protocol_classification")),

		NPMRingbuffersEnabled: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_ringbuffers")),
		CustomBatchingEnabled: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_custom_batching")),

		// Embed USM configuration
		USMConfig: NewUSMConfig(cfg),

		EnableConntrack:              cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_conntrack")),
		ConntrackMaxStateSize:        cfg.GetInt(sysconfig.FullKeyPath(spNS, "conntrack_max_state_size")),
		ConntrackRateLimit:           cfg.GetInt(sysconfig.FullKeyPath(spNS, "conntrack_rate_limit")),
		ConntrackRateLimitInterval:   3 * time.Second,
		EnableConntrackAllNamespaces: cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_conntrack_all_namespaces")),
		IgnoreConntrackInitFailure:   cfg.GetBool(sysconfig.FullKeyPath(netNS, "ignore_conntrack_init_failure")),
		ConntrackInitTimeout:         cfg.GetDuration(sysconfig.FullKeyPath(netNS, "conntrack_init_timeout")),
		EnableEbpfConntracker:        cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_ebpf_conntracker")),
		EnableCiliumLBConntracker:    cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_cilium_lb_conntracker")),

		EnableGatewayLookup: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_gateway_lookup")),

		EnableMonotonicCount: cfg.GetBool(sysconfig.FullKeyPath(spNS, "windows.enable_monotonic_count")),

		RecordedQueryTypes: cfg.GetStringSlice(sysconfig.FullKeyPath(netNS, "dns_recorded_query_types")),

		EnableProcessEventMonitoring: cfg.GetBool(sysconfig.FullKeyPath(evNS, "network_process", "enabled")),
		MaxProcessesTracked:          cfg.GetInt(sysconfig.FullKeyPath(evNS, "network_process", "max_processes_tracked")),

		EnableContainerStore: cfg.GetBool(sysconfig.FullKeyPath(evNS, "network_process", "container_store", "enabled")),
		MaxContainersTracked: cfg.GetInt(sysconfig.FullKeyPath(evNS, "network_process", "container_store", "max_containers_tracked")),

		EnableRootNetNs: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_root_netns")),

		EnableNPMConnectionRollup: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_connection_rollup")),

		EnableEbpfless: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_ebpfless")),
		EnableFentry:   cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_fentry")),

		ExpectedTagsDuration: cfg.GetDuration(sysconfig.FullKeyPath(spNS, "expected_tags_duration")),

		EnableCertCollection:             cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_cert_collection")),
		CertCollectionMapCleanerInterval: cfg.GetDuration(sysconfig.FullKeyPath(netNS, "cert_collection_map_cleaner_interval")),

		DirectSend: cfg.GetBool(sysconfig.FullKeyPath(netNS, "direct_send")),
	}

	if !c.CollectTCPv4Conns {
		log.Info("network tracer TCPv4 tracing disabled")
	}
	if !c.CollectUDPv4Conns {
		log.Info("network tracer UDPv4 tracing disabled")
	}
	if !c.CollectTCPv6Conns {
		log.Info("network tracer TCPv6 tracing disabled")
	}
	if !c.CollectUDPv6Conns {
		log.Info("network tracer UDPv6 tracing disabled")
	}
	if !c.DNSInspection {
		log.Info("network tracer DNS inspection disabled by configuration")
	}

	if err := structure.UnmarshalKey(cfg, sysconfig.FullKeyPath(netNS, "dns_monitoring_ports"), &c.DNSMonitoringPortList); err != nil {
		log.Warnf("failed to parse dns_monitoring_ports: %v", err)
	}

	if len(c.DNSMonitoringPortList) == 0 {
		c.DNSMonitoringPortList = []int{53}
	}

	c.DNSMonitoringPortList = slices.DeleteFunc(c.DNSMonitoringPortList, func(port int) bool {
		isHTTP := port == 80 || port == 443
		if isHTTP {
			log.Warnf("CNM detected and removed HTTP port %d from %s, which is unsupported due to the large volume of traffic it would capture", port, sysconfig.FullKeyPath(netNS, "dns_monitoring_ports"))
		}
		return isHTTP
	})

	if !c.EnableProcessEventMonitoring {
		log.Info("network process event monitoring disabled")
	}

	return c
}

// FailedConnectionsSupported returns true if the config & TCP v4 || v6 is enabled
func (c *Config) FailedConnectionsSupported() bool {
	if !c.TCPFailedConnectionsEnabled {
		return false
	}
	if !c.CollectTCPv4Conns && !c.CollectTCPv6Conns {
		return false
	}
	return true
}
