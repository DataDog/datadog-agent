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
	cfg.BindEnvAndSetDefault(join(smNS, "enabled"), false, "DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED")
	cfg.BindEnv(join(smNS, "max_concurrent_requests"))
	cfg.BindEnv(join(smNS, "enable_quantization"))
	cfg.BindEnv(join(smNS, "enable_connection_rollup"))
	cfg.BindEnvAndSetDefault(join(smNS, "enable_ring_buffers"), true)
	cfg.BindEnvAndSetDefault(join(smNS, "enable_event_stream"), true)
	// kernel_buffer_pages determines the number of pages allocated *per CPU*
	// for buffering kernel data, whether using a perf buffer or a ring buffer.
	cfg.BindEnvAndSetDefault(join(smNS, "kernel_buffer_pages"), 16)
	// data_channel_size defines the size of the Go channel that buffers events.
	// Each event has a fixed size of approximately 4KB (sizeof(batch_data_t)).
	// By setting this value to 100, the channel will buffer up to ~400KB of data in the Go heap memory.
	cfg.BindEnvAndSetDefault(join(smNS, "data_channel_size"), 100)
	cfg.BindEnvAndSetDefault(join(smNS, "disable_map_preallocation"), true)

	// ========================================
	// HTTP Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "enable_http_monitoring"), true)
	// For backward compatibility
	cfg.BindEnv(join(netNS, "enable_http_monitoring"), "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING")

	cfg.BindEnvAndSetDefault(join(smNS, "max_http_stats_buffered"), 100000)
	// For backward compatibility
	cfg.BindEnv(join(netNS, "max_http_stats_buffered"), "DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED")

	cfg.BindEnvAndSetDefault(join(smNS, "max_tracked_http_connections"), 1024)
	// For backward compatibility
	cfg.BindEnv(join(netNS, "max_tracked_http_connections"))

	cfg.BindEnvAndSetDefault(join(smNS, "http_notification_threshold"), 512)
	// For backward compatibility
	cfg.BindEnv(join(netNS, "http_notification_threshold"))

	// Default value (512) is set in `adjustUSM`, to avoid having "deprecation warning", due to the default value.
	cfg.BindEnv(join(smNS, "http_max_request_fragment"))
	// For backward compatibility
	cfg.BindEnv(join(netNS, "http_max_request_fragment"))

	cfg.BindEnvAndSetDefault(join(smNS, "http_map_cleaner_interval_in_s"), 300)
	// For backward compatibility
	cfg.BindEnv(join(spNS, "http_map_cleaner_interval_in_s"))

	cfg.BindEnvAndSetDefault(join(smNS, "http_idle_connection_ttl_in_s"), 30)
	// For backward compatibility
	cfg.BindEnv(join(spNS, "http_idle_connection_ttl_in_s"))

	oldHTTPRules := join(netNS, "http_replace_rules")
	newHTTPRules := join(smNS, "http_replace_rules")
	cfg.BindEnv(newHTTPRules)
	// For backward compatibility
	cfg.BindEnv(oldHTTPRules, "DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES")

	httpRulesTransformer := func(key string) transformerFunction {
		return func(in string) []map[string]string {
			var out []map[string]string
			if err := json.Unmarshal([]byte(in), &out); err != nil {
				log.Warnf(`%q can not be parsed: %v`, key, err)
			}
			return out
		}
	}
	cfg.ParseEnvAsSliceMapString(oldHTTPRules, httpRulesTransformer(oldHTTPRules))
	cfg.ParseEnvAsSliceMapString(newHTTPRules, httpRulesTransformer(newHTTPRules))

	// ========================================
	// HTTP/2 Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "enable_http2_monitoring"), false)
	cfg.BindEnvAndSetDefault(join(smNS, "http2_dynamic_table_map_cleaner_interval_seconds"), 30)

	// ========================================
	// Kafka Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "enable_kafka_monitoring"), false)
	cfg.BindEnvAndSetDefault(join(smNS, "max_kafka_stats_buffered"), 100000)

	// ========================================
	// PostgreSQL Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "postgres", "enabled"), false)
	cfg.BindEnvAndSetDefault(join(smNS, "postgres", "max_stats_buffered"), 100000)
	cfg.BindEnvAndSetDefault(join(smNS, "postgres", "max_telemetry_buffer"), 160)

	// ========================================
	// Redis Protocol Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "redis", "enabled"), false)
	cfg.BindEnvAndSetDefault(join(smNS, "redis", "track_resources"), false)
	cfg.BindEnvAndSetDefault(join(smNS, "redis", "max_stats_buffered"), 100000)

	// ========================================
	// Native TLS Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "tls", "native", "enabled"), true)
	// For backward compatibility
	cfg.BindEnv(join(netNS, "enable_https_monitoring"), "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING")

	// ========================================
	// Go TLS Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "tls", "go", "enabled"), true)
	// For backward compatibility
	cfg.BindEnv(join(smNS, "enable_go_tls_support"))
	cfg.BindEnvAndSetDefault(join(smNS, "tls", "go", "exclude_self"), true)

	// ========================================
	// Istio TLS Configuration
	// ========================================
	cfg.BindEnvAndSetDefault(join(smNS, "tls", "istio", "enabled"), true)
	cfg.BindEnvAndSetDefault(join(smNS, "tls", "istio", "envoy_path"), defaultEnvoyPath)

	// ========================================
	// Node.js TLS Configuration
	// ========================================
	cfg.BindEnv(join(smNS, "tls", "nodejs", "enabled"))
}
