// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxHTTPFrag = 512 // matches hard limit currently imposed in NPM driver
)

// disableProtocolIfUnsupported disables a protocol monitoring feature if the kernel doesn't support it
func disableProtocolIfUnsupported(cfg model.Config, configKey string, isSupported bool, protocolName string) {
	if cfg.GetBool(configKey) && !isSupported {
		if flavor.GetFlavor() == flavor.SystemProbe {
			// Only log in system-probe, as we cannot reliably know this in the agent
			log.Warnf("disabling %s monitoring as it is not supported for this kernel version", protocolName)
		}
		cfg.Set(configKey, false, model.SourceAgentRuntime)
	}
}

func adjustUSM(cfg model.Config) {
	if cfg.GetBool(smNS("enabled")) {
		applyDefault(cfg, spNS("enable_runtime_compiler"), true)
		applyDefault(cfg, spNS("enable_kernel_header_download"), true)
	}

	// HTTP configuration migration to tree structure with backward compatibility
	// Each tree structure key is paired with its deprecated flat versions
	deprecateBool(cfg, smNS("enable_http_monitoring"), smNS("http", "enabled"))
	deprecateBool(cfg, netNS("enable_http_monitoring"), smNS("http", "enabled"))

	deprecateInt(cfg, smNS("max_http_stats_buffered"), smNS("http", "max_stats_buffered"))
	deprecateInt(cfg, netNS("max_http_stats_buffered"), smNS("http", "max_stats_buffered"))

	deprecateInt64(cfg, smNS("max_tracked_http_connections"), smNS("http", "max_tracked_connections"))
	deprecateInt64(cfg, netNS("max_tracked_http_connections"), smNS("http", "max_tracked_connections"))

	deprecateInt64(cfg, smNS("http_notification_threshold"), smNS("http", "notification_threshold"))
	deprecateInt64(cfg, netNS("http_notification_threshold"), smNS("http", "notification_threshold"))

	deprecateInt64(cfg, smNS("http_max_request_fragment"), smNS("http", "max_request_fragment"))
	deprecateInt64(cfg, netNS("http_max_request_fragment"), smNS("http", "max_request_fragment"))

	deprecateInt(cfg, smNS("http_map_cleaner_interval_in_s"), smNS("http", "map_cleaner_interval_seconds"))
	deprecateInt(cfg, spNS("http_map_cleaner_interval_in_s"), smNS("http", "map_cleaner_interval_seconds"))

	deprecateInt(cfg, smNS("http_idle_connection_ttl_in_s"), smNS("http", "idle_connection_ttl_seconds"))
	deprecateInt(cfg, spNS("http_idle_connection_ttl_in_s"), smNS("http", "idle_connection_ttl_seconds"))

	deprecateGeneric(cfg, smNS("http_replace_rules"), smNS("http", "replace_rules"))
	deprecateGeneric(cfg, netNS("http_replace_rules"), smNS("http", "replace_rules"))

	// Non-HTTP deprecations
	deprecateBool(cfg, netNS("enable_https_monitoring"), smNS("tls", "native", "enabled"))
	deprecateBool(cfg, smNS("enable_go_tls_support"), smNS("tls", "go", "enabled"))
	applyDefault(cfg, smNS("max_concurrent_requests"), cfg.GetInt(spNS("max_tracked_connections")))
	deprecateBool(cfg, smNS("enable_kafka_monitoring"), smNS("kafka", "enabled"))
	deprecateInt(cfg, smNS("max_kafka_stats_buffered"), smNS("kafka", "max_stats_buffered"))
	deprecateBool(cfg, smNS("process_service_inference", "enabled"), spNS("process_service_inference", "enabled"))
	deprecateBool(cfg, smNS("process_service_inference", "use_windows_service_name"), spNS("process_service_inference", "use_windows_service_name"))

	// HTTP/2 configuration migration
	deprecateBool(cfg, smNS("enable_http2_monitoring"), smNS("http2", "enabled"))
	deprecateInt(cfg, smNS("http2_dynamic_table_map_cleaner_interval_seconds"), smNS("http2", "dynamic_table_map_cleaner_interval_seconds"))

	// Redis configuration migration
	deprecateBool(cfg, smNS("enable_redis_monitoring"), smNS("redis", "enabled"))

	// Disable protocols if kernel version is not supported
	disableProtocolIfUnsupported(cfg, smNS("redis", "enabled"), RedisMonitoringSupported(), "Redis")
	disableProtocolIfUnsupported(cfg, smNS("http2", "enabled"), HTTP2MonitoringSupported(), "HTTP2")
	// Similar to the checkin in adjustNPM(). The process event data stream and USM have the same
	// minimum kernel version requirement, but USM's check for that is done
	// later.  This check here prevents the EventMonitorModule from getting
	// enabled on unsupported kernels by load() in config.go.
	disableProtocolIfUnsupported(cfg, smNS("enable_event_stream"), ProcessEventDataStreamSupported(), "USM event stream")

	validateInt(cfg, smNS("http", "notification_threshold"), cfg.GetInt(smNS("http", "max_tracked_connections"))/2, func(v int) error {
		limit := cfg.GetInt(smNS("http", "max_tracked_connections"))
		if v >= limit {
			return fmt.Errorf("notification threshold %d set higher than tracked connections %d", v, limit)
		}
		return nil
	})

	limitMaxInt64(cfg, smNS("http", "max_request_fragment"), maxHTTPFrag)

	if cfg.GetBool(smNS("disable_map_preallocation")) && !eBPFMapPreallocationSupported() {
		if flavor.GetFlavor() == flavor.SystemProbe {
			// Only log in system-probe, as we cannot reliably know this in the agent
			log.Warn("using preallocaed eBPF map for USM as it is supported for this kernel version")
		}
		cfg.Set(smNS("disable_map_preallocation"), false, model.SourceAgentRuntime)
	}
}
