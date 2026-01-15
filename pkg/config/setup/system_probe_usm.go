// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package setup

import (
	"encoding/json"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func initUSMSystemProbeConfig(cfg pkgconfigmodel.Setup) {
	// ========================================
	// General USM Configuration
	// ========================================
	cfg.BindEnvAndSetDefault("service_monitoring_config.enabled", false, "DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED")
	cfg.BindEnv("service_monitoring_config.max_concurrent_requests")  //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("service_monitoring_config.enable_quantization")      //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("service_monitoring_config.enable_connection_rollup") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("service_monitoring_config.enable_ring_buffers", true)
	cfg.BindEnvAndSetDefault("service_monitoring_config.enable_event_stream", true)
	// kernel_buffer_pages determines the number of pages allocated *per CPU*
	// for buffering kernel data, whether using a perf buffer or a ring buffer.
	cfg.BindEnvAndSetDefault("service_monitoring_config.kernel_buffer_pages", 16)
	// data_channel_size defines the size of the Go channel that buffers events.
	// Each event has a fixed size of approximately 4KB (sizeof(batch_data_t)).
	// By setting this value to 100, the channel will buffer up to ~400KB of data in the Go heap memory.
	cfg.BindEnvAndSetDefault("service_monitoring_config.data_channel_size", 100)
	cfg.BindEnvAndSetDefault("service_monitoring_config.disable_map_preallocation", true)
	cfg.BindEnvAndSetDefault("service_monitoring_config.direct_consumer.buffer_wakeup_count_per_cpu", 8)
	cfg.BindEnvAndSetDefault("service_monitoring_config.direct_consumer.channel_size", 1000)
	cfg.BindEnvAndSetDefault("service_monitoring_config.direct_consumer.kernel_buffer_size_per_cpu", 65536) // 64KB per CPU base size

	// ========================================
	// HTTP Protocol Configuration
	// ========================================
	// New tree structure with backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.http.enabled", true)
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.enable_http_monitoring", true)
	cfg.BindEnvAndSetDefault("network_config.enable_http_monitoring", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING")

	cfg.BindEnvAndSetDefault("service_monitoring_config.http.max_stats_buffered", 100000)
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.max_http_stats_buffered", 100000)
	cfg.BindEnvAndSetDefault("network_config.max_http_stats_buffered", 100000, "DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED")

	cfg.BindEnvAndSetDefault("service_monitoring_config.http.max_tracked_connections", 1024)
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.max_tracked_http_connections", 1024)
	cfg.BindEnvAndSetDefault("network_config.max_tracked_http_connections", 1024)

	cfg.BindEnvAndSetDefault("service_monitoring_config.http.notification_threshold", 512)
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.http_notification_threshold", 512)
	cfg.BindEnvAndSetDefault("network_config.http_notification_threshold", 512)

	cfg.BindEnvAndSetDefault("service_monitoring_config.http.max_request_fragment", 512) // matches hard limit currently imposed in NPM driver
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.http_max_request_fragment", 512)
	cfg.BindEnvAndSetDefault("network_config.http_max_request_fragment", 512)

	cfg.BindEnvAndSetDefault("service_monitoring_config.http.map_cleaner_interval_seconds", 300)
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.http_map_cleaner_interval_in_s", 300)
	cfg.BindEnvAndSetDefault("system_probe_config.http_map_cleaner_interval_in_s", 300)

	cfg.BindEnvAndSetDefault("service_monitoring_config.http.idle_connection_ttl_seconds", 30)
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.http_idle_connection_ttl_in_s", 30)
	cfg.BindEnvAndSetDefault("system_probe_config.http_idle_connection_ttl_in_s", 30)

	cfg.BindEnvAndSetDefault("service_monitoring_config.http.use_direct_consumer", false)

	// HTTP replace rules configuration
	cfg.BindEnvAndSetDefault("service_monitoring_config.http.replace_rules", nil)
	// Deprecated flat keys for backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.http_replace_rules", nil)
	cfg.BindEnvAndSetDefault("network_config.http_replace_rules", nil, "DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES")

	httpRulesTransformer := func(key string) transformerFunction {
		return func(in string) []map[string]string {
			var out []map[string]string
			if err := json.Unmarshal([]byte(in), &out); err != nil {
				log.Warnf(`%q can not be parsed: %v`, key, err)
			}
			return out
		}
	}
	replaceRules := []string{
		"service_monitoring_config.http.replace_rules",
		"service_monitoring_config.http_replace_rules",
		"network_config.http_replace_rules",
	}
	for _, rule := range replaceRules {
		cfg.ParseEnvAsSliceMapString(rule, httpRulesTransformer(rule))
	}

	// ========================================
	// HTTP/2 Protocol Configuration
	// ========================================
	// Tree structure
	cfg.BindEnvAndSetDefault("service_monitoring_config.http2.enabled", false)
	cfg.BindEnvAndSetDefault("service_monitoring_config.http2.dynamic_table_map_cleaner_interval_seconds", 30)

	// Legacy bindings for backward compatibility (deprecated)
	cfg.BindEnvAndSetDefault("service_monitoring_config.enable_http2_monitoring", false)
	cfg.BindEnvAndSetDefault("service_monitoring_config.http2_dynamic_table_map_cleaner_interval_seconds", 30)

	// ========================================
	// Kafka Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault("service_monitoring_config.kafka.enabled", false)
	// For backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.enable_kafka_monitoring", false)

	cfg.BindEnvAndSetDefault("service_monitoring_config.kafka.max_stats_buffered", 100000)
	// For backward compatibility
	cfg.BindEnvAndSetDefault("service_monitoring_config.max_kafka_stats_buffered", 100000)

	// ========================================
	// PostgreSQL Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault("service_monitoring_config.postgres.enabled", false)
	cfg.BindEnvAndSetDefault("service_monitoring_config.postgres.max_stats_buffered", 100000)
	cfg.BindEnvAndSetDefault("service_monitoring_config.postgres.max_telemetry_buffer", 160)

	// ========================================
	// Redis Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault("service_monitoring_config.redis.enabled", false)
	cfg.BindEnvAndSetDefault("service_monitoring_config.redis.track_resources", false)
	cfg.BindEnvAndSetDefault("service_monitoring_config.redis.max_stats_buffered", 100000)

	// ========================================
	// Native TLS Configuration
	// ========================================
	cfg.BindEnvAndSetDefault("service_monitoring_config.tls.native.enabled", true)
	// For backward compatibility
	cfg.BindEnv("network_config.enable_https_monitoring", "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	// ========================================
	// Go TLS Configuration
	// ========================================
	cfg.BindEnvAndSetDefault("service_monitoring_config.tls.go.enabled", true)
	// For backward compatibility
	cfg.BindEnv("service_monitoring_config.enable_go_tls_support") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("service_monitoring_config.tls.go.exclude_self", true)

	// ========================================
	// Istio TLS Configuration
	// ========================================
	cfg.BindEnvAndSetDefault("service_monitoring_config.tls.istio.enabled", true)
	cfg.BindEnvAndSetDefault("service_monitoring_config.tls.istio.envoy_path", defaultEnvoyPath)

	// ========================================
	// Node.js TLS Configuration
	// ========================================
	cfg.BindEnv("service_monitoring_config.tls.nodejs.enabled") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
}
