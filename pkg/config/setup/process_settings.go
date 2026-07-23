// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"time"

	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupProcesses(config pkgconfigmodel.Setup) {
	// "process_config.enabled" is deprecated. We must still be able to detect if it is present, to know if we should use it
	// or container_collection.enabled and process_collection.enabled.
	//
	// It's a string as the possible values are "disabled", "false", "true"...
	config.BindEnvAndSetDefault("process_config.enabled", "false", "DD_PROCESS_CONFIG_ENABLED", "DD_PROCESS_AGENT_ENABLED")
	config.BindEnvAndSetDefault("process_config.container_collection.enabled", true, "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "DD_PROCESS_AGENT_CONTAINER_COLLECTION_ENABLED")
	config.BindEnvAndSetDefault("process_config.process_collection.enabled", false, "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "DD_PROCESS_AGENT_PROCESS_COLLECTION_ENABLED")
	config.BindEnvAndSetDefault("process_config.process_dd_url", "", "DD_PROCESS_CONFIG_PROCESS_DD_URL", "DD_PROCESS_AGENT_PROCESS_DD_URL", "DD_PROCESS_AGENT_URL", "DD_PROCESS_CONFIG_URL")
	config.BindEnvAndSetDefault("process_config.dd_agent_env", "", "DD_PROCESS_CONFIG_DD_AGENT_ENV", "DD_PROCESS_AGENT_DD_AGENT_ENV")
	config.BindEnvAndSetDefault("process_config.queue_size", DefaultProcessQueueSize, "DD_PROCESS_CONFIG_QUEUE_SIZE", "DD_PROCESS_AGENT_QUEUE_SIZE")
	config.BindEnvAndSetDefault("process_config.process_queue_bytes", DefaultProcessQueueBytes, "DD_PROCESS_CONFIG_PROCESS_QUEUE_BYTES", "DD_PROCESS_AGENT_PROCESS_QUEUE_BYTES")
	config.BindEnvAndSetDefault("process_config.rt_queue_size", DefaultProcessRTQueueSize, "DD_PROCESS_CONFIG_RT_QUEUE_SIZE", "DD_PROCESS_AGENT_RT_QUEUE_SIZE")
	config.BindEnvAndSetDefault("process_config.max_per_message", DefaultProcessMaxPerMessage, "DD_PROCESS_CONFIG_MAX_PER_MESSAGE", "DD_PROCESS_AGENT_MAX_PER_MESSAGE")
	config.BindEnvAndSetDefault("process_config.max_message_bytes", DefaultProcessMaxMessageBytes, "DD_PROCESS_CONFIG_MAX_MESSAGE_BYTES", "DD_PROCESS_AGENT_MAX_MESSAGE_BYTES")
	config.BindEnvAndSetDefault("process_config.cmd_port", DefaultProcessCmdPort, "DD_PROCESS_CONFIG_CMD_PORT", "DD_PROCESS_AGENT_CMD_PORT")
	config.BindEnvAndSetDefault("process_config.blacklist_patterns", []string{}, "DD_PROCESS_CONFIG_BLACKLIST_PATTERNS", "DD_PROCESS_AGENT_BLACKLIST_PATTERNS")

	// The interval, in seconds, at which we will run each check. If you want consistent
	// behavior between real-time you may set the Container/ProcessRT intervals to 10.
	// Defaults to 10s for normal checks and 2s for others.
	config.BindEnvAndSetDefault("process_config.intervals.process", 10, "DD_PROCESS_CONFIG_INTERVALS_PROCESS", "DD_PROCESS_AGENT_INTERVALS_PROCESS")
	config.BindEnvAndSetDefault("process_config.intervals.process_realtime", 2, "DD_PROCESS_CONFIG_INTERVALS_PROCESS_REALTIME", "DD_PROCESS_AGENT_INTERVALS_PROCESS_REALTIME")
	config.BindEnvAndSetDefault("process_config.intervals.container", 10, "DD_PROCESS_CONFIG_INTERVALS_CONTAINER", "DD_PROCESS_AGENT_INTERVALS_CONTAINER")
	config.BindEnvAndSetDefault("process_config.intervals.container_realtime", 2, "DD_PROCESS_CONFIG_INTERVALS_CONTAINER_REALTIME", "DD_PROCESS_AGENT_INTERVALS_CONTAINER_REALTIME")
	config.BindEnvAndSetDefault("process_config.intervals.connections", 30, "DD_PROCESS_CONFIG_INTERVALS_CONNECTIONS", "DD_PROCESS_AGENT_INTERVALS_CONNECTIONS")
	config.BindEnvAndSetDefault("process_config.dd_agent_bin", GetPlatformDefault(map[string]interface{}{
		"linux":   "${install_path}/bin/agent/agent",
		"darwin":  "${install_path}/bin/agent/agent",
		"aix":     "${install_path}/bin/agent/agent",
		"windows": "${install_path}/bin/agent.exe",
	}),
		"DD_PROCESS_CONFIG_DD_AGENT_BIN", "DD_PROCESS_AGENT_DD_AGENT_BIN")
	config.BindEnvAndSetDefault("process_config.custom_sensitive_words", []string{}, "DD_CUSTOM_SENSITIVE_WORDS", "DD_PROCESS_CONFIG_CUSTOM_SENSITIVE_WORDS", "DD_PROCESS_AGENT_CUSTOM_SENSITIVE_WORDS")
	pkgconfighelper.ParseEnvJSONOrComma("process_config.custom_sensitive_words", config)
	config.BindEnvAndSetDefault("process_config.scrub_args", true, "DD_SCRUB_ARGS", "DD_PROCESS_CONFIG_SCRUB_ARGS", "DD_PROCESS_AGENT_SCRUB_ARGS")
	config.BindEnvAndSetDefault("process_config.strip_proc_arguments", false, "DD_STRIP_PROCESS_ARGS", "DD_PROCESS_CONFIG_STRIP_PROC_ARGUMENTS", "DD_PROCESS_AGENT_STRIP_PROC_ARGUMENTS")
	// Use PDH API to collect performance counter data for process check on Windows
	config.BindEnvAndSetDefault("process_config.windows.use_perf_counters", false, "DD_PROCESS_CONFIG_WINDOWS_USE_PERF_COUNTERS", "DD_PROCESS_AGENT_WINDOWS_USE_PERF_COUNTERS")
	config.BindEnvAndSetDefault("process_config.additional_endpoints", map[string][]string{}, "DD_PROCESS_CONFIG_ADDITIONAL_ENDPOINTS", "DD_PROCESS_AGENT_ADDITIONAL_ENDPOINTS", "DD_PROCESS_ADDITIONAL_ENDPOINTS")
	config.BindEnvAndSetDefault("process_config.expvar_port", DefaultProcessExpVarPort, "DD_PROCESS_CONFIG_EXPVAR_PORT", "DD_PROCESS_AGENT_EXPVAR_PORT")
	config.BindEnvAndSetDefault("process_config.log_file", "${log_path}/process-agent.log", "DD_PROCESS_CONFIG_LOG_FILE", "DD_PROCESS_AGENT_LOG_FILE")
	config.BindEnvAndSetDefault("process_config.internal_profiling.enabled", false, "DD_PROCESS_CONFIG_INTERNAL_PROFILING_ENABLED", "DD_PROCESS_AGENT_INTERNAL_PROFILING_ENABLED")
	config.BindEnvAndSetDefault("process_config.grpc_connection_timeout_secs", DefaultGRPCConnectionTimeoutSecs, "DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS", "DD_PROCESS_AGENT_GRPC_CONNECTION_TIMEOUT_SECS")
	config.BindEnvAndSetDefault("process_config.disable_realtime_checks", false, "DD_PROCESS_CONFIG_DISABLE_REALTIME_CHECKS", "DD_PROCESS_AGENT_DISABLE_REALTIME_CHECKS")
	config.BindEnvAndSetDefault("process_config.ignore_zombie_processes", false, "DD_PROCESS_CONFIG_IGNORE_ZOMBIE_PROCESSES", "DD_PROCESS_AGENT_IGNORE_ZOMBIE_PROCESSES")
	config.BindEnvAndSetDefault("process_config.process_discovery.enabled", true, "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED", "DD_PROCESS_CONFIG_DISCOVERY_ENABLED", "DD_PROCESS_AGENT_DISCOVERY_ENABLED")
	config.BindEnvAndSetDefault("process_config.process_discovery.interval", 4*time.Hour, "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_INTERVAL", "DD_PROCESS_AGENT_PROCESS_DISCOVERY_INTERVAL")
	config.BindEnvAndSetDefault("process_config.process_discovery.hint_frequency", DefaultProcessDiscoveryHintFrequency, "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_HINT_FREQUENCY", "DD_PROCESS_AGENT_PROCESS_DISCOVERY_HINT_FREQUENCY")
	config.BindEnvAndSetDefault("process_config.drop_check_payloads", []string{}, "DD_PROCESS_CONFIG_DROP_CHECK_PAYLOADS", "DD_PROCESS_AGENT_DROP_CHECK_PAYLOADS")
	config.BindEnvAndSetDefault("process_config.cache_lookupid", false, "DD_PROCESS_CONFIG_CACHE_LOOKUPID", "DD_PROCESS_AGENT_CACHE_LOOKUPID")
	config.BindEnvAndSetDefault("process_config.language_detection.grpc_port", DefaultProcessEntityStreamPort, "DD_PROCESS_CONFIG_LANGUAGE_DETECTION_GRPC_PORT", "DD_PROCESS_AGENT_LANGUAGE_DETECTION_GRPC_PORT")
}
