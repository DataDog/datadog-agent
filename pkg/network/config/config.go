// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	spNS  = "system_probe_config"
	netNS = "network_config"
	smNS  = "service_monitoring_config"

	defaultUDPTimeoutSeconds       = 30
	defaultUDPStreamTimeoutSeconds = 120

	defaultOffsetThreshold = 400
	maxOffsetThreshold     = 3000
)

// Config stores all flags used by the network eBPF tracer
type Config struct {
	ebpf.Config

	// NPMEnabled is whether the network performance monitoring feature is explicitly enabled or not
	NPMEnabled bool

	// ServiceMonitoringEnabled is whether the service monitoring feature is enabled or not
	ServiceMonitoringEnabled bool

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

	// EnableHTTPMonitoring specifies whether the tracer should monitor HTTPS traffic
	// Supported libraries: OpenSSL
	EnableHTTPSMonitoring bool

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

	// TCPClosedTimeout represents the maximum amount of time a closed TCP connection can remain buffered in eBPF before
	// being marked as idle and flushed to the perf ring.
	TCPClosedTimeout time.Duration

	// MaxTrackedConnections specifies the maximum number of connections we can track. This determines the size of the eBPF Maps
	MaxTrackedConnections uint

	// MaxClosedConnectionsBuffered represents the maximum number of closed connections we'll buffer in memory. These closed connections
	// get flushed on every client request (default 30s check interval)
	MaxClosedConnectionsBuffered int

	// MaxDNSStatsBuffered represents the maximum number of DNS stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxDNSStatsBuffered int

	// MaxHTTPStatsBuffered represents the maximum number of HTTP stats we'll buffer in memory. These stats
	// get flushed on every client request (default 30s check interval)
	MaxHTTPStatsBuffered int

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

	// ConntrackInitTimeout specifies how long we wait for conntrack to initialize before failing
	ConntrackInitTimeout time.Duration

	// EnableConntrackAllNamespaces enables network address translation via netlink for all namespaces that are peers of the root namespace.
	// default is true
	EnableConntrackAllNamespaces bool

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

	// EnableRootNetNs disables using the network namespace of the root process (1)
	// for things like creating netlink sockets for conntrack updates, etc.
	EnableRootNetNs bool

	// HTTPMapCleanerInterval is the interval to run the cleaner function.
	HTTPMapCleanerInterval time.Duration

	// HTTPIdleConnectionTTL is the time an idle connection counted as "inactive" and should be deleted.
	HTTPIdleConnectionTTL time.Duration
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// New creates a config for the network tracer
func New() *Config {
	cfg := ddconfig.Datadog
	ddconfig.InitSystemProbeConfig(cfg)

	c := &Config{
		Config: *ebpf.NewConfig(),

		NPMEnabled:               cfg.GetBool(join(netNS, "enabled")),
		ServiceMonitoringEnabled: cfg.GetBool(join(smNS, "enabled")),

		CollectTCPConns:  !cfg.GetBool(join(spNS, "disable_tcp")),
		TCPConnTimeout:   2 * time.Minute,
		TCPClosedTimeout: 1 * time.Second,

		CollectUDPConns:  !cfg.GetBool(join(spNS, "disable_udp")),
		UDPConnTimeout:   defaultUDPTimeoutSeconds * time.Second,
		UDPStreamTimeout: defaultUDPStreamTimeoutSeconds * time.Second,

		CollectIPv6Conns:               !cfg.GetBool(join(spNS, "disable_ipv6")),
		OffsetGuessThreshold:           uint64(cfg.GetInt64(join(spNS, "offset_guess_threshold"))),
		ExcludedSourceConnections:      cfg.GetStringMapStringSlice(join(spNS, "source_excludes")),
		ExcludedDestinationConnections: cfg.GetStringMapStringSlice(join(spNS, "dest_excludes")),

		MaxTrackedConnections:        uint(cfg.GetInt(join(spNS, "max_tracked_connections"))),
		MaxClosedConnectionsBuffered: cfg.GetInt(join(spNS, "max_closed_connections_buffered")),
		ClosedChannelSize:            cfg.GetInt(join(spNS, "closed_channel_size")),
		MaxConnectionsStateBuffered:  cfg.GetInt(join(spNS, "max_connection_state_buffered")),
		ClientStateExpiry:            2 * time.Minute,

		DNSInspection:       !cfg.GetBool(join(spNS, "disable_dns_inspection")),
		CollectDNSStats:     cfg.GetBool(join(spNS, "collect_dns_stats")),
		CollectLocalDNS:     cfg.GetBool(join(spNS, "collect_local_dns")),
		CollectDNSDomains:   cfg.GetBool(join(spNS, "collect_dns_domains")),
		MaxDNSStats:         cfg.GetInt(join(spNS, "max_dns_stats")),
		MaxDNSStatsBuffered: 75000,
		DNSTimeout:          time.Duration(cfg.GetInt(join(spNS, "dns_timeout_in_s"))) * time.Second,

		EnableHTTPMonitoring:  cfg.GetBool(join(netNS, "enable_http_monitoring")),
		EnableHTTPSMonitoring: cfg.GetBool(join(netNS, "enable_https_monitoring")),
		MaxHTTPStatsBuffered:  cfg.GetInt(join(netNS, "max_http_stats_buffered")),

		EnableConntrack:              cfg.GetBool(join(spNS, "enable_conntrack")),
		ConntrackMaxStateSize:        cfg.GetInt(join(spNS, "conntrack_max_state_size")),
		ConntrackRateLimit:           cfg.GetInt(join(spNS, "conntrack_rate_limit")),
		EnableConntrackAllNamespaces: cfg.GetBool(join(spNS, "enable_conntrack_all_namespaces")),
		IgnoreConntrackInitFailure:   cfg.GetBool(join(netNS, "ignore_conntrack_init_failure")),
		ConntrackInitTimeout:         cfg.GetDuration(join(netNS, "conntrack_init_timeout")),

		EnableGatewayLookup: cfg.GetBool(join(netNS, "enable_gateway_lookup")),

		EnableMonotonicCount: cfg.GetBool(join(spNS, "windows.enable_monotonic_count")),

		RecordedQueryTypes: cfg.GetStringSlice(join(netNS, "dns_recorded_query_types")),

		EnableRootNetNs: cfg.GetBool(join(netNS, "enable_root_netns")),

		HTTPMapCleanerInterval: time.Duration(cfg.GetInt(join(spNS, "http_map_cleaner_interval_in_s"))) * time.Second,
		HTTPIdleConnectionTTL:  time.Duration(cfg.GetInt(join(spNS, "http_idle_connection_ttl_in_s"))) * time.Second,
	}

	if !cfg.IsSet(join(spNS, "max_closed_connections_buffered")) {
		// make sure max_closed_connections_buffered is equal to
		// max_tracked_connections, since the former is not set.
		// this helps with lowering or eliminating dropped
		// closed connections in environments with mostly short-lived
		// connections
		c.MaxClosedConnectionsBuffered = int(c.MaxTrackedConnections)
	}

	httpRRKey := join(netNS, "http_replace_rules")
	rr, err := parseReplaceRules(cfg, httpRRKey)
	if err != nil {
		log.Errorf("error parsing %q: %v", httpRRKey, err)
	} else {
		c.HTTPReplaceRules = rr
	}

	if c.OffsetGuessThreshold > maxOffsetThreshold {
		log.Warn("offset_guess_threshold exceeds maximum of 3000. Setting it to the default of 400")
		c.OffsetGuessThreshold = defaultOffsetThreshold
	}

	if !kernel.IsIPv6Enabled() {
		c.CollectIPv6Conns = false
		log.Info("network tracer IPv6 tracing disabled by system")
	} else if !c.CollectIPv6Conns {
		log.Info("network tracer IPv6 tracing disabled by configuration")
	}

	if !c.CollectUDPConns {
		log.Info("network tracer UDP tracing disabled by configuration")
	}
	if !c.CollectTCPConns {
		log.Info("network tracer TCP tracing disabled by configuration")
	}
	if !c.DNSInspection {
		log.Info("network tracer DNS inspection disabled by configuration")
	}

	if c.ServiceMonitoringEnabled {
		cfg.Set(join(netNS, "enable_http_monitoring"), true)
		c.EnableHTTPMonitoring = true
		if !cfg.IsSet(join(netNS, "enable_https_monitoring")) {
			cfg.Set(join(netNS, "enable_https_monitoring"), true)
			c.EnableHTTPSMonitoring = true
		}

		if !cfg.IsSet(join(spNS, "enable_runtime_compiler")) {
			cfg.Set(join(spNS, "enable_runtime_compiler"), true)
			c.EnableRuntimeCompiler = true
		}

		if !cfg.IsSet(join(spNS, "enable_kernel_header_download")) {
			cfg.Set(join(spNS, "enable_kernel_header_download"), true)
			c.EnableKernelHeaderDownload = true
		}
	}

	if !c.EnableRootNetNs {
		c.EnableConntrackAllNamespaces = false
	}

	return c
}
