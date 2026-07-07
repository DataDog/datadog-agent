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
	procBindEnvAndSetDefault(config, "process_config.enabled", "false")

	procBindEnvAndSetDefault(config, "process_config.container_collection.enabled", true)
	procBindEnvAndSetDefault(config, "process_config.process_collection.enabled", false)

	config.BindEnvAndSetDefault("process_config.process_dd_url", "",
		"DD_PROCESS_CONFIG_PROCESS_DD_URL",
		"DD_PROCESS_AGENT_PROCESS_DD_URL",
		"DD_PROCESS_AGENT_URL",
		"DD_PROCESS_CONFIG_URL",
	)
	procBindEnvAndSetDefault(config, "process_config.dd_agent_env", "")
	procBindEnvAndSetDefault(config, "process_config.queue_size", DefaultProcessQueueSize)
	procBindEnvAndSetDefault(config, "process_config.process_queue_bytes", DefaultProcessQueueBytes)
	procBindEnvAndSetDefault(config, "process_config.rt_queue_size", DefaultProcessRTQueueSize)
	procBindEnvAndSetDefault(config, "process_config.max_per_message", DefaultProcessMaxPerMessage)
	procBindEnvAndSetDefault(config, "process_config.max_message_bytes", DefaultProcessMaxMessageBytes)
	procBindEnvAndSetDefault(config, "process_config.cmd_port", DefaultProcessCmdPort)
	procBindEnvAndSetDefault(config, "process_config.blacklist_patterns", []string{})

	// The interval, in seconds, at which we will run each check. If you want consistent
	// behavior between real-time you may set the Container/ProcessRT intervals to 10.
	// Defaults to 10s for normal checks and 2s for others.
	procBindEnvAndSetDefault(config, "process_config.intervals.process", 10)
	procBindEnvAndSetDefault(config, "process_config.intervals.process_realtime", 2)
	procBindEnvAndSetDefault(config, "process_config.intervals.container", 10)
	procBindEnvAndSetDefault(config, "process_config.intervals.container_realtime", 2)
	procBindEnvAndSetDefault(config, "process_config.intervals.connections", 30)

	procBindEnvAndSetDefault(config, "process_config.dd_agent_bin", GetPlatformDefault(map[string]interface{}{
		"linux":   "${install_path}/bin/agent/agent",
		"darwin":  "${install_path}/bin/agent/agent",
		"aix":     "${install_path}/bin/agent/agent",
		"windows": "${install_path}/bin/agent.exe",
	}))

	config.BindEnvAndSetDefault("process_config.custom_sensitive_words", []string{},
		"DD_CUSTOM_SENSITIVE_WORDS",
		"DD_PROCESS_CONFIG_CUSTOM_SENSITIVE_WORDS",
		"DD_PROCESS_AGENT_CUSTOM_SENSITIVE_WORDS")
	pkgconfighelper.ParseEnvJSONOrComma("process_config.custom_sensitive_words", config)

	config.BindEnvAndSetDefault("process_config.scrub_args", true,
		"DD_SCRUB_ARGS",
		"DD_PROCESS_CONFIG_SCRUB_ARGS",
		"DD_PROCESS_AGENT_SCRUB_ARGS")
	config.BindEnvAndSetDefault("process_config.strip_proc_arguments", false,
		"DD_STRIP_PROCESS_ARGS",
		"DD_PROCESS_CONFIG_STRIP_PROC_ARGUMENTS",
		"DD_PROCESS_AGENT_STRIP_PROC_ARGUMENTS")
	// Use PDH API to collect performance counter data for process check on Windows
	procBindEnvAndSetDefault(config, "process_config.windows.use_perf_counters", false)
	config.BindEnvAndSetDefault("process_config.additional_endpoints", make(map[string][]string),
		"DD_PROCESS_CONFIG_ADDITIONAL_ENDPOINTS",
		"DD_PROCESS_AGENT_ADDITIONAL_ENDPOINTS",
		"DD_PROCESS_ADDITIONAL_ENDPOINTS",
	)
	procBindEnvAndSetDefault(config, "process_config.expvar_port", DefaultProcessExpVarPort)
	procBindEnvAndSetDefault(config, "process_config.log_file", "${log_path}/process-agent.log")
	procBindEnvAndSetDefault(config, "process_config.internal_profiling.enabled", false)
	procBindEnvAndSetDefault(config, "process_config.grpc_connection_timeout_secs", DefaultGRPCConnectionTimeoutSecs)
	procBindEnvAndSetDefault(config, "process_config.disable_realtime_checks", false)
	procBindEnvAndSetDefault(config, "process_config.ignore_zombie_processes", false)

	// Process Discovery Check
	// We also bind old environment variables for this setting
	config.BindEnvAndSetDefault("process_config.process_discovery.enabled", true,
		"DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_CONFIG_DISCOVERY_ENABLED",
		"DD_PROCESS_AGENT_DISCOVERY_ENABLED",
	)
	procBindEnvAndSetDefault(config, "process_config.process_discovery.interval", 4*time.Hour)

	procBindEnvAndSetDefault(config, "process_config.process_discovery.hint_frequency", DefaultProcessDiscoveryHintFrequency)

	procBindEnvAndSetDefault(config, "process_config.drop_check_payloads", []string{})

	procBindEnvAndSetDefault(config, "process_config.cache_lookupid", false)

	procBindEnvAndSetDefault(config, "process_config.language_detection.grpc_port", DefaultProcessEntityStreamPort)
}
