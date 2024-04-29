// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	maxHTTPFrag = 512 // matches hard limit currently imposed in NPM driver
)

func adjustUSM(cfg config.Config) {
	if cfg.GetBool(smNS("enabled")) {
		applyDefault(cfg, netNS("enable_http_monitoring"), true)
		applyDefault(cfg, netNS("enable_https_monitoring"), true)
		applyDefault(cfg, spNS("enable_runtime_compiler"), true)
		applyDefault(cfg, spNS("enable_kernel_header_download"), true)
	}

	deprecateBool(cfg, netNS("enable_http_monitoring"), smNS("enable_http_monitoring"))
	deprecateBool(cfg, netNS("enable_https_monitoring"), smNS("tls", "native", "enabled"))
	deprecateBool(cfg, smNS("enable_go_tls_support"), smNS("tls", "go", "enabled"))
	deprecateGeneric(cfg, netNS("http_replace_rules"), smNS("http_replace_rules"))
	deprecateInt64(cfg, netNS("max_tracked_http_connections"), smNS("max_tracked_http_connections"))
	applyDefault(cfg, smNS("max_tracked_http_connections"), 1024)
	deprecateInt(cfg, netNS("max_http_stats_buffered"), smNS("max_http_stats_buffered"))
	applyDefault(cfg, smNS("max_http_stats_buffered"), 100000)
	deprecateInt(cfg, spNS("http_map_cleaner_interval_in_s"), smNS("http_map_cleaner_interval_in_s"))
	applyDefault(cfg, smNS("http_map_cleaner_interval_in_s"), 300)
	deprecateInt(cfg, spNS("http_idle_connection_ttl_in_s"), smNS("http_idle_connection_ttl_in_s"))
	applyDefault(cfg, smNS("http_idle_connection_ttl_in_s"), 30)
	deprecateInt64(cfg, netNS("http_notification_threshold"), smNS("http_notification_threshold"))
	applyDefault(cfg, smNS("http_notification_threshold"), 512)
	deprecateInt64(cfg, netNS("http_max_request_fragment"), smNS("http_max_request_fragment"))
	// set the default to be the max allowed by the driver.  So now the config will allow us to
	// shorten the allowed path, but not lengthen it.
	applyDefault(cfg, smNS("http_max_request_fragment"), maxHTTPFrag)
	applyDefault(cfg, smNS("max_concurrent_requests"), cfg.GetInt(spNS("max_tracked_connections")))
	deprecateBool(cfg, smNS("process_service_inference", "enabled"), spNS("process_service_inference", "enabled"))
	deprecateBool(cfg, smNS("process_service_inference", "use_windows_service_name"), spNS("process_service_inference", "use_windows_service_name"))
	applyDefault(cfg, spNS("process_service_inference", "enabled"), false)
	applyDefault(cfg, spNS("process_service_inference", "use_windows_service_name"), true)
	applyDefault(cfg, smNS("enable_ring_buffers"), true)

	validateInt(cfg, smNS("http_notification_threshold"), cfg.GetInt(smNS("max_tracked_http_connections"))/2, func(v int) error {
		limit := cfg.GetInt(smNS("max_tracked_http_connections"))
		if v >= limit {
			return fmt.Errorf("notification threshold %d set higher than tracked connections %d", v, limit)
		}
		return nil
	})

	limitMaxInt64(cfg, smNS("http_max_request_fragment"), maxHTTPFrag)
}
