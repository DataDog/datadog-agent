// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxHTTPFrag = 512 // matches hard limit currently imposed in NPM driver
)

func adjustUSM(cfg model.Config) {
	if cfg.GetBool(smNS("enabled")) {
		applyDefault(cfg, spNS("enable_runtime_compiler"), true)
		applyDefault(cfg, spNS("enable_kernel_header_download"), true)
	}

	deprecateBool(cfg, netNS("enable_http_monitoring"), smNS("enable_http_monitoring"))
	applyDefault(cfg, smNS("enable_http_monitoring"), true)
	deprecateBool(cfg, netNS("enable_https_monitoring"), smNS("tls", "native", "enabled"))
	applyDefault(cfg, smNS("tls", "native", "enabled"), true)
	deprecateBool(cfg, smNS("enable_go_tls_support"), smNS("tls", "go", "enabled"))
	applyDefault(cfg, smNS("tls", "go", "enabled"), true)
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

	// default on windows is now enabled; default on linux is still disabled
	if runtime.GOOS == "windows" {
		applyDefault(cfg, spNS("process_service_inference", "enabled"), true)
	} else {
		applyDefault(cfg, spNS("process_service_inference", "enabled"), false)
	}

	// Similar to the checkin in adjustNPM(). The process event data stream and USM have the same
	// minimum kernel version requirement, but USM's check for that is done
	// later.  This check here prevents the EventMonitorModule from getting
	// enabled on unsupported kernels by load() in config.go.
	if cfg.GetBool(smNS("enable_event_stream")) && !ProcessEventDataStreamSupported() {
		log.Warn("disabling USM event stream as it is not supported for this kernel version")
		cfg.Set(smNS("enable_event_stream"), false, model.SourceAgentRuntime)
	}

	applyDefault(cfg, spNS("process_service_inference", "use_windows_service_name"), true)
	applyDefault(cfg, smNS("enable_ring_buffers"), true)
	applyDefault(cfg, smNS("max_postgres_stats_buffered"), 100000)
	applyDefault(cfg, smNS("max_redis_stats_buffered"), 100000)

	// kernel_buffer_pages determines the number of pages allocated *per CPU*
	// for buffering kernel data, whether using a perf buffer or a ring buffer.
	applyDefault(cfg, smNS("kernel_buffer_pages"), 16)

	// data_channel_size defines the size of the Go channel that buffers events.
	// Each event has a fixed size of approximately 4KB (sizeof(batch_data_t)).
	// By setting this value to 100, the channel will buffer up to ~400KB of data in the Go heap memory.
	applyDefault(cfg, smNS("data_channel_size"), 100)

	validateInt(cfg, smNS("http_notification_threshold"), cfg.GetInt(smNS("max_tracked_http_connections"))/2, func(v int) error {
		limit := cfg.GetInt(smNS("max_tracked_http_connections"))
		if v >= limit {
			return fmt.Errorf("notification threshold %d set higher than tracked connections %d", v, limit)
		}
		return nil
	})

	limitMaxInt64(cfg, smNS("http_max_request_fragment"), maxHTTPFrag)
}
