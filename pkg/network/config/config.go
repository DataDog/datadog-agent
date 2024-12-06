// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements network tracing configuration
package config

import (
	"time"

	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
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

	// ServiceMonitoringEnabled is whether the service monitoring feature is enabled or not
	ServiceMonitoringEnabled bool

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

	// DNSTimeout determines the length of time to wait before considering a DNS Query to have timed out
	DNSTimeout time.Duration

	// MaxDNSStats determines the number of separate DNS Stats objects DNSStatkeeper can have at any given time
	// These stats objects get flushed on every client request (default 30s check interval)
	MaxDNSStats int

	// EnableHTTPMonitoring specifies whether the tracer should monitor HTTP traffic
	EnableHTTPMonitoring bool

	// EnableHTTP2Monitoring specifies whether the tracer should monitor HTTP2 traffic
	EnableHTTP2Monitoring bool

	// EnableKafkaMonitoring specifies whether the tracer should monitor Kafka traffic
	EnableKafkaMonitoring bool

	// EnablePostgresMonitoring specifies whether the tracer should monitor Postgres traffic.
	EnablePostgresMonitoring bool

	// EnableRedisMonitoring specifies whether the tracer should monitor Redis traffic.
	EnableRedisMonitoring bool

	// EnableNativeTLSMonitoring specifies whether the USM should monitor HTTPS traffic via native libraries.
	// Supported libraries: OpenSSL, GnuTLS, LibCrypto.
	EnableNativeTLSMonitoring bool

	// EnableIstioMonitoring specifies whether USM should monitor Istio traffic
	EnableIstioMonitoring bool

	// EnvoyPath specifies the envoy path to be used for Istio monitoring
	EnvoyPath string

	// EnableNodeJSMonitoring specifies whether USM should monitor NodeJS TLS traffic
	EnableNodeJSMonitoring bool

	// EnableGoTLSSupport specifies whether the tracer should monitor HTTPS
	// traffic done through Go's standard library's TLS implementation
	EnableGoTLSSupport bool

	// GoTLSExcludeSelf specifies whether USM's GoTLS module should avoid
	// hooking the system-probe test binary. Defaults to true.
	GoTLSExcludeSelf bool

	// MaxTrackedHTTPConnections max number of http(s) flows that will be concurrently tracked.
	// value is currently Windows only
	MaxTrackedHTTPConnections int64

	// HTTPNotificationThreshold is the number of connections to hold in the kernel before signalling
	// to be retrieved.  Currently Windows only
	HTTPNotificationThreshold int64

	// HTTPMaxRequestFragment is the size of the HTTP path buffer to be retrieved.
	// Currently Windows only
	HTTPMaxRequestFragment int64

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

	// MaxUSMConcurrentRequests represents the maximum number of requests (for a single protocol)
	// that can happen concurrently at a given point in time. This parameter is used for sizing our eBPF maps.
	MaxUSMConcurrentRequests uint32

	// MaxHTTPStatsBuffered represents the maximum number of HTTP stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxHTTPStatsBuffered int

	// MaxKafkaStatsBuffered represents the maximum number of Kafka stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxKafkaStatsBuffered int

	// MaxPostgresTelemetryBuffer represents the maximum size of the telemetry buffer size for Postgres.
	MaxPostgresTelemetryBuffer int

	// MaxPostgresStatsBuffered represents the maximum number of Postgres stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxPostgresStatsBuffered int

	// MaxRedisStatsBuffered represents the maximum number of Redis stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxRedisStatsBuffered int

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

	// EnableEbpfConntracker enables the ebpf based network conntracker. Used only for testing at the moment
	EnableEbpfConntracker bool

	// ClosedChannelSize specifies the size for closed channel for the tracer
	ClosedChannelSize int

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

	// HTTP replace rules
	HTTPReplaceRules []*ReplaceRule

	// EnableProcessEventMonitoring enables consuming CWS process monitoring events from the runtime security module
	EnableProcessEventMonitoring bool

	// MaxProcessesTracked is the maximum number of processes whose information is stored in the network module
	MaxProcessesTracked int

	// EnableRootNetNs disables using the network namespace of the root process (1)
	// for things like creating netlink sockets for conntrack updates, etc.
	EnableRootNetNs bool

	// HTTP2DynamicTableMapCleanerInterval is the interval to run the cleaner function.
	HTTP2DynamicTableMapCleanerInterval time.Duration

	// HTTPMapCleanerInterval is the interval to run the cleaner function.
	HTTPMapCleanerInterval time.Duration

	// HTTPIdleConnectionTTL is the time an idle connection counted as "inactive" and should be deleted.
	HTTPIdleConnectionTTL time.Duration

	// ProtocolClassificationEnabled specifies whether the tracer should enhance connection data with protocols names by
	// classifying the L7 protocols being used.
	ProtocolClassificationEnabled bool

	// TCPFailedConnectionsEnabled specifies whether the tracer will track & report TCP error codes
	TCPFailedConnectionsEnabled bool

	// EnableNPMConnectionRollup enables aggregating connections by rolling up ephemeral ports
	EnableNPMConnectionRollup bool

	// EnableUSMQuantization enables endpoint quantization for USM programs
	EnableUSMQuantization bool

	// NPMRingbuffersEnabled specifies whether ringbuffers are enabled or not
	NPMRingbuffersEnabled bool

	// EnableUSMConnectionRollup enables the aggregation of connection data belonging to a same (client, server) pair
	EnableUSMConnectionRollup bool

	// EnableUSMRingBuffers enables the use of eBPF Ring Buffer types on
	// supported kernels.
	// Defaults to true. Setting this to false on a Kernel that supports ring
	// buffers (>=5.8) will result in forcing the use of Perf Maps instead.
	EnableUSMRingBuffers bool

	EnableEbpfless bool

	// EnableUSMEventStream enables USM to use the event stream instead
	// of netlink for receiving process events.
	EnableUSMEventStream bool
}

// New creates a config for the network tracer
func New() *Config {
	cfg := pkgconfigsetup.SystemProbe()
	sysconfig.Adjust(cfg)

	c := &Config{
		Config: *ebpf.NewConfig(),

		NPMEnabled:               cfg.GetBool(sysconfig.FullKeyPath(netNS, "enabled")),
		ServiceMonitoringEnabled: cfg.GetBool(sysconfig.FullKeyPath(smNS, "enabled")),

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
		ClosedConnectionFlushThreshold: cfg.GetInt(sysconfig.FullKeyPath(spNS, "closed_connection_flush_threshold")),
		ClosedChannelSize:              cfg.GetInt(sysconfig.FullKeyPath(spNS, "closed_channel_size")),
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

		EnableHTTPMonitoring:       cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_http_monitoring")),
		EnableHTTP2Monitoring:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_http2_monitoring")),
		EnableKafkaMonitoring:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_kafka_monitoring")),
		EnablePostgresMonitoring:   cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_postgres_monitoring")),
		EnableRedisMonitoring:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_redis_monitoring")),
		EnableNativeTLSMonitoring:  cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "native", "enabled")),
		EnableIstioMonitoring:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "istio", "enabled")),
		EnvoyPath:                  cfg.GetString(sysconfig.FullKeyPath(smNS, "tls", "istio", "envoy_path")),
		EnableNodeJSMonitoring:     cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "nodejs", "enabled")),
		MaxUSMConcurrentRequests:   uint32(cfg.GetInt(sysconfig.FullKeyPath(smNS, "max_concurrent_requests"))),
		MaxHTTPStatsBuffered:       cfg.GetInt(sysconfig.FullKeyPath(smNS, "max_http_stats_buffered")),
		MaxKafkaStatsBuffered:      cfg.GetInt(sysconfig.FullKeyPath(smNS, "max_kafka_stats_buffered")),
		MaxPostgresStatsBuffered:   cfg.GetInt(sysconfig.FullKeyPath(smNS, "max_postgres_stats_buffered")),
		MaxPostgresTelemetryBuffer: cfg.GetInt(sysconfig.FullKeyPath(smNS, "max_postgres_telemetry_buffer")),
		MaxRedisStatsBuffered:      cfg.GetInt(sysconfig.FullKeyPath(smNS, "max_redis_stats_buffered")),

		MaxTrackedHTTPConnections: cfg.GetInt64(sysconfig.FullKeyPath(smNS, "max_tracked_http_connections")),
		HTTPNotificationThreshold: cfg.GetInt64(sysconfig.FullKeyPath(smNS, "http_notification_threshold")),
		HTTPMaxRequestFragment:    cfg.GetInt64(sysconfig.FullKeyPath(smNS, "http_max_request_fragment")),

		EnableConntrack:              cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_conntrack")),
		ConntrackMaxStateSize:        cfg.GetInt(sysconfig.FullKeyPath(spNS, "conntrack_max_state_size")),
		ConntrackRateLimit:           cfg.GetInt(sysconfig.FullKeyPath(spNS, "conntrack_rate_limit")),
		ConntrackRateLimitInterval:   3 * time.Second,
		EnableConntrackAllNamespaces: cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_conntrack_all_namespaces")),
		IgnoreConntrackInitFailure:   cfg.GetBool(sysconfig.FullKeyPath(netNS, "ignore_conntrack_init_failure")),
		ConntrackInitTimeout:         cfg.GetDuration(sysconfig.FullKeyPath(netNS, "conntrack_init_timeout")),
		EnableEbpfConntracker:        cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_ebpf_conntracker")),

		EnableGatewayLookup: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_gateway_lookup")),

		EnableMonotonicCount: cfg.GetBool(sysconfig.FullKeyPath(spNS, "windows.enable_monotonic_count")),

		RecordedQueryTypes: cfg.GetStringSlice(sysconfig.FullKeyPath(netNS, "dns_recorded_query_types")),

		EnableProcessEventMonitoring: cfg.GetBool(sysconfig.FullKeyPath(evNS, "network_process", "enabled")),
		MaxProcessesTracked:          cfg.GetInt(sysconfig.FullKeyPath(evNS, "network_process", "max_processes_tracked")),

		EnableRootNetNs: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_root_netns")),

		HTTP2DynamicTableMapCleanerInterval: time.Duration(cfg.GetInt(sysconfig.FullKeyPath(smNS, "http2_dynamic_table_map_cleaner_interval_seconds"))) * time.Second,

		HTTPMapCleanerInterval: time.Duration(cfg.GetInt(sysconfig.FullKeyPath(smNS, "http_map_cleaner_interval_in_s"))) * time.Second,
		HTTPIdleConnectionTTL:  time.Duration(cfg.GetInt(sysconfig.FullKeyPath(smNS, "http_idle_connection_ttl_in_s"))) * time.Second,

		EnableNPMConnectionRollup: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_connection_rollup")),

		EnableEbpfless: cfg.GetBool(sysconfig.FullKeyPath(netNS, "enable_ebpfless")),

		// Service Monitoring
		EnableGoTLSSupport:        cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "go", "enabled")),
		GoTLSExcludeSelf:          cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "go", "exclude_self")),
		EnableUSMQuantization:     cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_quantization")),
		EnableUSMConnectionRollup: cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_connection_rollup")),
		EnableUSMRingBuffers:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_ring_buffers")),
		EnableUSMEventStream:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_event_stream")),
	}

	httpRRKey := sysconfig.FullKeyPath(smNS, "http_replace_rules")
	rr, err := parseReplaceRules(cfg, httpRRKey)
	if err != nil {
		log.Errorf("error parsing %q: %v", httpRRKey, err)
	} else {
		c.HTTPReplaceRules = rr
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

	if c.EnableProcessEventMonitoring {
		log.Info("network process event monitoring enabled")
	}
	return c
}

// RingBufferSupportedNPM returns true if the kernel supports ring buffers and the config enables them
func (c *Config) RingBufferSupportedNPM() bool {
	return (features.HaveMapType(cebpf.RingBuf) == nil) && c.NPMRingbuffersEnabled
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
