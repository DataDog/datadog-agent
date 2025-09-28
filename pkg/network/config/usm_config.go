// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// USMConfig contains all configuration specific to Universal Service Monitoring (USM)
type USMConfig struct {
	// ========================================
	// Global USM Configuration
	// ========================================

	// ServiceMonitoringEnabled is whether the service monitoring feature is enabled or not
	ServiceMonitoringEnabled bool

	// MaxUSMConcurrentRequests represents the maximum number of requests (for a single protocol)
	// that can happen concurrently at a given point in time. This parameter is used for sizing our eBPF maps.
	MaxUSMConcurrentRequests uint32

	// EnableUSMQuantization enables endpoint quantization for USM programs
	EnableUSMQuantization bool

	// EnableUSMConnectionRollup enables the aggregation of connection data belonging to a same (client, server) pair
	EnableUSMConnectionRollup bool

	// EnableUSMRingBuffers enables the use of eBPF Ring Buffer types on supported kernels
	EnableUSMRingBuffers bool

	// EnableUSMEventStream enables USM to use the event stream instead of netlink for receiving process events
	EnableUSMEventStream bool

	// USMKernelBufferPages defines the number of pages to allocate for the USM kernel buffer
	USMKernelBufferPages int

	// USMDataChannelSize specifies the size of the data channel for USM
	USMDataChannelSize int

	// USMDirectBufferWakeupCount specifies the number of events that will buffer in a perf/ring buffer before userspace is woken up for USM direct consumer.
	USMDirectBufferWakeupCount int

	// USMDirectChannelSize specifies the channel buffer size multiplier for USM direct consumer.
	USMDirectChannelSize int

	// USMDirectPerfBufferSize specifies the perf buffer size for USM direct consumer.
	USMDirectPerfBufferSize int

	// USMDirectRingBufferSize specifies the ring buffer size for USM direct consumer.
	USMDirectRingBufferSize int

	// DisableMapPreallocation controls whether eBPF maps should disable preallocation (BPF_F_NO_PREALLOC flag).
	// When true, maps allocate entries on-demand instead of preallocating the full map size, improving memory efficiency.
	DisableMapPreallocation bool

	// ========================================
	// HTTP Protocol Configuration
	// ========================================

	// EnableHTTPMonitoring specifies whether the tracer should monitor HTTP traffic
	EnableHTTPMonitoring bool

	// MaxHTTPStatsBuffered represents the maximum number of HTTP stats we'll buffer in memory
	MaxHTTPStatsBuffered int

	// HTTPMapCleanerInterval is the interval to run the cleaner function
	HTTPMapCleanerInterval time.Duration

	// HTTPIdleConnectionTTL is the time an idle connection counted as "inactive" and should be deleted
	HTTPIdleConnectionTTL time.Duration

	// HTTPReplaceRules are rules for replacing HTTP path patterns
	HTTPReplaceRules []*ReplaceRule

	// HTTP Windows-specific Configuration
	// MaxTrackedHTTPConnections max number of http(s) flows that will be concurrently tracked (Windows only)
	MaxTrackedHTTPConnections int64

	// HTTPNotificationThreshold is the number of connections to hold in the kernel before signalling (Windows only)
	HTTPNotificationThreshold int64

	// HTTPMaxRequestFragment is the size of the HTTP path buffer to be retrieved (Windows only)
	HTTPMaxRequestFragment int64

	// ========================================
	// HTTP2 Protocol Configuration
	// ========================================

	// EnableHTTP2Monitoring specifies whether the tracer should monitor HTTP2 traffic
	EnableHTTP2Monitoring bool

	// HTTP2DynamicTableMapCleanerInterval is the interval to run the cleaner function
	HTTP2DynamicTableMapCleanerInterval time.Duration

	// ========================================
	// Kafka Protocol Configuration
	// ========================================

	// EnableKafkaMonitoring specifies whether the tracer should monitor Kafka traffic
	EnableKafkaMonitoring bool

	// MaxKafkaStatsBuffered represents the maximum number of Kafka stats we'll buffer in memory
	MaxKafkaStatsBuffered int

	// ========================================
	// Postgres Protocol Configuration
	// ========================================

	// EnablePostgresMonitoring specifies whether the tracer should monitor Postgres traffic
	EnablePostgresMonitoring bool

	// MaxPostgresStatsBuffered represents the maximum number of Postgres stats we'll buffer in memory
	MaxPostgresStatsBuffered int

	// MaxPostgresTelemetryBuffer represents the maximum size of the telemetry buffer size for Postgres
	MaxPostgresTelemetryBuffer int

	// ========================================
	// Redis Protocol Configuration
	// ========================================

	// EnableRedisMonitoring specifies whether the tracer should monitor Redis traffic
	EnableRedisMonitoring bool

	// RedisTrackResources specifies whether to track Redis resource names (keys) or only methods
	RedisTrackResources bool

	// MaxRedisStatsBuffered represents the maximum number of Redis stats we'll buffer in memory
	MaxRedisStatsBuffered int

	// ========================================
	// Native TLS Configuration (OpenSSL, GnuTLS, LibCrypto)
	// ========================================

	// EnableNativeTLSMonitoring specifies whether the USM should monitor HTTPS traffic via native libraries
	// Supported libraries: OpenSSL, GnuTLS, LibCrypto
	EnableNativeTLSMonitoring bool

	// ========================================
	// Go TLS Configuration
	// ========================================

	// EnableGoTLSSupport specifies whether the tracer should monitor HTTPS traffic done through Go's standard library
	EnableGoTLSSupport bool

	// GoTLSExcludeSelf specifies whether USM's GoTLS module should avoid hooking the system-probe test binary
	GoTLSExcludeSelf bool

	// ========================================
	// NodeJS TLS Configuration
	// ========================================

	// EnableNodeJSMonitoring specifies whether USM should monitor NodeJS TLS traffic
	EnableNodeJSMonitoring bool

	// ========================================
	// Istio Service Mesh TLS Configuration
	// ========================================

	// EnableIstioMonitoring specifies whether USM should monitor Istio traffic
	EnableIstioMonitoring bool

	// EnvoyPath specifies the envoy path to be used for Istio monitoring
	EnvoyPath string
}

// NewUSMConfig creates a new USM configuration from the system probe config
func NewUSMConfig(cfg model.Config) *USMConfig {
	usmConfig := &USMConfig{
		// Global USM Configuration
		ServiceMonitoringEnabled:   cfg.GetBool(sysconfig.FullKeyPath(smNS, "enabled")),
		MaxUSMConcurrentRequests:   uint32(cfg.GetInt(sysconfig.FullKeyPath(smNS, "max_concurrent_requests"))),
		EnableUSMQuantization:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_quantization")),
		EnableUSMConnectionRollup:  cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_connection_rollup")),
		EnableUSMRingBuffers:       cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_ring_buffers")),
		EnableUSMEventStream:       cfg.GetBool(sysconfig.FullKeyPath(smNS, "enable_event_stream")),
		USMKernelBufferPages:       cfg.GetInt(sysconfig.FullKeyPath(smNS, "kernel_buffer_pages")),
		USMDataChannelSize:         cfg.GetInt(sysconfig.FullKeyPath(smNS, "data_channel_size")),
		DisableMapPreallocation:    cfg.GetBool(sysconfig.FullKeyPath(smNS, "disable_map_preallocation")),
		USMDirectBufferWakeupCount: cfg.GetInt(sysconfig.FullKeyPath(smNS, "usm_direct_buffer_wakeup_count")),
		USMDirectChannelSize:       cfg.GetInt(sysconfig.FullKeyPath(smNS, "usm_direct_channel_size")),
		USMDirectPerfBufferSize:    cfg.GetInt(sysconfig.FullKeyPath(smNS, "usm_direct_perf_buffer_size")),
		USMDirectRingBufferSize:    cfg.GetInt(sysconfig.FullKeyPath(smNS, "usm_direct_ring_buffer_size")),

		// HTTP Protocol Configuration
		EnableHTTPMonitoring:      cfg.GetBool(sysconfig.FullKeyPath(smNS, "http", "enabled")),
		MaxHTTPStatsBuffered:      cfg.GetInt(sysconfig.FullKeyPath(smNS, "http", "max_stats_buffered")),
		HTTPMapCleanerInterval:    time.Duration(cfg.GetInt(sysconfig.FullKeyPath(smNS, "http", "map_cleaner_interval_seconds"))) * time.Second,
		HTTPIdleConnectionTTL:     time.Duration(cfg.GetInt(sysconfig.FullKeyPath(smNS, "http", "idle_connection_ttl_seconds"))) * time.Second,
		MaxTrackedHTTPConnections: cfg.GetInt64(sysconfig.FullKeyPath(smNS, "http", "max_tracked_connections")),
		HTTPNotificationThreshold: cfg.GetInt64(sysconfig.FullKeyPath(smNS, "http", "notification_threshold")),
		HTTPMaxRequestFragment:    cfg.GetInt64(sysconfig.FullKeyPath(smNS, "http", "max_request_fragment")),

		// HTTP2 Protocol Configuration
		EnableHTTP2Monitoring:               cfg.GetBool(sysconfig.FullKeyPath(smNS, "http2", "enabled")),
		HTTP2DynamicTableMapCleanerInterval: time.Duration(cfg.GetInt(sysconfig.FullKeyPath(smNS, "http2", "dynamic_table_map_cleaner_interval_seconds"))) * time.Second,

		// Kafka Protocol Configuration
		EnableKafkaMonitoring: cfg.GetBool(sysconfig.FullKeyPath(smNS, "kafka", "enabled")),
		MaxKafkaStatsBuffered: cfg.GetInt(sysconfig.FullKeyPath(smNS, "kafka", "max_stats_buffered")),

		// Postgres Protocol Configuration
		EnablePostgresMonitoring:   cfg.GetBool(sysconfig.FullKeyPath(smNS, "postgres", "enabled")),
		MaxPostgresStatsBuffered:   cfg.GetInt(sysconfig.FullKeyPath(smNS, "postgres", "max_stats_buffered")),
		MaxPostgresTelemetryBuffer: cfg.GetInt(sysconfig.FullKeyPath(smNS, "postgres", "max_telemetry_buffer")),

		// Redis Protocol Configuration
		EnableRedisMonitoring: cfg.GetBool(sysconfig.FullKeyPath(smNS, "redis", "enabled")),
		RedisTrackResources:   cfg.GetBool(sysconfig.FullKeyPath(smNS, "redis", "track_resources")),
		MaxRedisStatsBuffered: cfg.GetInt(sysconfig.FullKeyPath(smNS, "redis", "max_stats_buffered")),

		// TLS Configuration
		EnableNativeTLSMonitoring: cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "native", "enabled")),
		EnableGoTLSSupport:        cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "go", "enabled")),
		GoTLSExcludeSelf:          cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "go", "exclude_self")),
		EnableNodeJSMonitoring:    cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "nodejs", "enabled")),
		EnableIstioMonitoring:     cfg.GetBool(sysconfig.FullKeyPath(smNS, "tls", "istio", "enabled")),
		EnvoyPath:                 cfg.GetString(sysconfig.FullKeyPath(smNS, "tls", "istio", "envoy_path")),
	}

	// Parse HTTP Replace Rules
	httpRRKey := sysconfig.FullKeyPath(smNS, "http", "replace_rules")
	rr, err := parseReplaceRules(cfg, httpRRKey)
	if err != nil {
		log.Errorf("error parsing %q: %v", httpRRKey, err)
	} else {
		usmConfig.HTTPReplaceRules = rr
	}

	return usmConfig
}

// ReplaceRule specifies a replace rule.
type ReplaceRule struct {
	// Pattern specifies the regexp pattern to be used when replacing. It must compile.
	Pattern string `mapstructure:"pattern"`

	// Re holds the compiled Pattern and is only used internally.
	Re *regexp.Regexp `mapstructure:"-" json:"-"`

	// Repl specifies the replacement string to be used when Pattern matches.
	Repl string `mapstructure:"repl"`
}

func parseReplaceRules(cfg model.Config, key string) ([]*ReplaceRule, error) {
	if !pkgconfigsetup.SystemProbe().IsConfigured(key) {
		return nil, nil
	}

	rules := make([]*ReplaceRule, 0)
	if err := structure.UnmarshalKey(cfg, key, &rules); err != nil {
		return nil, fmt.Errorf("rules format should be of the form '[{\"pattern\":\"pattern\",\"repl\":\"replace_str\"}]', error: %w", err)
	}

	for _, r := range rules {
		if r.Pattern == "" {
			return nil, errors.New(`all rules must have a "pattern"`)
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %q: %s", r.Pattern, err)
		}
		r.Re = re
	}

	return rules, nil
}
