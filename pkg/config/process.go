// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strings"
	"time"
)

const (
	// DefaultGRPCConnectionTimeoutSecs sets the default value for timeout when connecting to the agent
	DefaultGRPCConnectionTimeoutSecs = 60
)

// procBindEnvAndSetDefault is a helper function that generates both "DD_PROCESS_CONFIG_" and "DD_PROCESS_AGENT_" prefixes from a key.
// We need this helper function because the standard BindEnvAndSetDefault can only generate one prefix from a key.
func procBindEnvAndSetDefault(config Config, key string, val interface{}) {
	// Uppercase, replace "." with "_" and add "DD_" prefix to key so that we follow the same environment
	// variable convention as the core agent.
	processConfigKey := "DD_" + strings.Replace(strings.ToUpper(key), ".", "_", -1)
	processAgentKey := strings.Replace(processConfigKey, "PROCESS_CONFIG", "PROCESS_AGENT", 1)

	envs := []string{processConfigKey, processAgentKey}
	config.BindEnvAndSetDefault(key, val, envs...)
}

// procBindEnv is a helper function that does the same thing as procBindEnvAndSetDefault except it does not set a default.
func procBindEnv(config Config, key string) {
	processConfigKey := "DD_" + strings.Replace(strings.ToUpper(key), ".", "_", -1)
	processAgentKey := strings.Replace(processConfigKey, "PROCESS_CONFIG", "PROCESS_AGENT", 1)

	config.BindEnv(key, processConfigKey, processAgentKey)
}

func setupProcesses(config Config) {
	// process_config.enabled is only used on Windows by the core agent to start the process agent service.
	// it can be set from file, but not from env. Override it with value from DD_PROCESS_AGENT_ENABLED.
	procBindEnv(config, "process_config.enabled")
	config.BindEnv("process_config.process_dd_url", "")
	config.SetKnown("process_config.dd_agent_env")
	config.SetKnown("process_config.enabled")
	config.SetKnown("process_config.intervals.process_realtime")
	config.SetKnown("process_config.queue_size")
	config.SetKnown("process_config.rt_queue_size")
	config.SetKnown("process_config.max_per_message")
	config.SetKnown("process_config.max_ctr_procs_per_message")
	config.SetKnown("process_config.cmd_port")
	config.SetKnown("process_config.intervals.process")
	config.SetKnown("process_config.blacklist_patterns")
	config.SetKnown("process_config.intervals.container")
	config.SetKnown("process_config.intervals.container_realtime")
	procBindEnvAndSetDefault(config, "process_config.dd_agent_bin", DefaultDDAgentBin)
	config.SetKnown("process_config.custom_sensitive_words")
	config.SetKnown("process_config.scrub_args")
	config.SetKnown("process_config.strip_proc_arguments")
	config.SetKnown("process_config.windows.args_refresh_interval")
	config.SetKnown("process_config.windows.add_new_args")
	config.SetKnown("process_config.windows.use_perf_counters")
	config.SetKnown("process_config.additional_endpoints.*")
	config.SetKnown("process_config.container_source")
	config.SetKnown("process_config.intervals.connections")
	config.SetKnown("process_config.expvar_port")
	procBindEnvAndSetDefault(config, "process_config.log_file", DefaultProcessAgentLogFile)
	config.SetKnown("process_config.internal_profiling.enabled")
	procBindEnvAndSetDefault(config, "process_config.grpc_connection_timeout_secs", DefaultGRPCConnectionTimeoutSecs)
	procBindEnvAndSetDefault(config, "process_config.remote_tagger", true)
	procBindEnvAndSetDefault(config, "process_config.disable_realtime_checks", false)
	procBindEnvAndSetDefault(config, "process_config.process_collection.enabled", false)
	procBindEnvAndSetDefault(config, "process_config.container_collection.enabled", true)

	// Process Discovery Check
	config.BindEnvAndSetDefault("process_config.process_discovery.enabled", true,
		"DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_CONFIG_DISCOVERY_ENABLED", // Also bind old environment variables
		"DD_PROCESS_AGENT_DISCOVERY_ENABLED",
	)
	procBindEnvAndSetDefault(config, "process_config.process_discovery.interval", 4*time.Hour)
}

// GetProcessCollectionEnabled returns the value of process_collection.enabled.
// If the legacy process_config.enabled setting is set, we display a warning and use that value instead.
func GetProcessCollectionEnabled() bool {
	if Datadog.IsSet("process_config.enabled") {
		_ = log.Warn("process_config.enabled is deprecated, please use process_config.process_collection.enabled instead")

		return Datadog.GetString("process_config.enabled") == "true"
	}

	return Datadog.GetBool("process_config.process_collection.enabled")
}

// GetContainerCollectionEnabled returns the value of container_collection.enabled, keeping in mind that a user may have set
// the deprecated process_config.enabled setting.
func GetContainerCollectionEnabled() bool {
	if Datadog.IsSet("process_config.enabled") {
		_ = log.Warn("process_config.enabled is deprecated, please use process_config.container_collection.enabled instead")

		procConfigEnabled := Datadog.GetString("process_config.enabled")
		return procConfigEnabled == "true" || procConfigEnabled == "false"
	}
	return Datadog.GetBool("process_config.container_collection.enabled")
}
