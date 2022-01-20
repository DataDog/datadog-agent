// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	envs := append([]string{processConfigKey, processAgentKey})
	config.BindEnvAndSetDefault(key, val, envs...)
}

// procBindEnv is a helper function that generates both "DD_PROCESS_CONFIG_" and "DD_PROCESS_AGENT_" prefixes from a key, but does not set a default.
// We need this helper function because the standard BindEnv can only generate one prefix from a key.
func procBindEnv(config Config, key string) {
	processConfigKey := "DD_" + strings.Replace(strings.ToUpper(key), ".", "_", -1)
	processAgentKey := strings.Replace(processConfigKey, "PROCESS_CONFIG", "PROCESS_AGENT", 1)

	config.BindEnv(key, processConfigKey, processAgentKey)
}

func setupProcesses(config Config) {
	// "process_config.enabled" is deprecated. We must still be able to detect if it is present, to know if we should use it
	// or container_collection.enabled and process_collection.enabled.
	procBindEnv(config, "process_config.enabled")
	config.SetEnvKeyTransformer("process_config.enabled", func(val string) interface{} {
		// DD_PROCESS_AGENT_ENABLED: true - Process + Container checks enabled
		//                           false - No checks enabled
		//                           (unset) - Defaults are used, only container check is enabled
		if enabled, _ := strconv.ParseBool(val); enabled {
			return "true"
		}
		return "disabled"
	})
	procBindEnvAndSetDefault(config, "process_config.container_collection.enabled", true)
	procBindEnvAndSetDefault(config, "process_config.process_collection.enabled", false)

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

	// Process Discovery Check
	config.BindEnvAndSetDefault("process_config.process_discovery.enabled", true,
		"DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_CONFIG_DISCOVERY_ENABLED", // Also bind old environment variables
		"DD_PROCESS_AGENT_DISCOVERY_ENABLED",
	)
	procBindEnvAndSetDefault(config, "process_config.process_discovery.interval", 4*time.Hour)
}

var displayProcConfigEnabledWarningOnce sync.Once

// displayProcConfigEnabledDeprecationWarning displays a deprecation warning for process_config.enabled only once.
func displayProcConfigEnabledDeprecationWarning() {
	displayProcConfigEnabledWarningOnce.Do(func() {
		log.Debug("process_config.enabled is deprecated, use process_config.container_collection.enabled" +
			" and process_config.process_collection.enabled instead")
	})
	return
}

// GetContainerCollectionEnabled retrieves the value of process_config.container_collection.enabled.
// If process_config.enabled is set, we display a deprecation warning and use that value instead.
func GetContainerCollectionEnabled(config Config) bool {
	if config.IsSet("process_config.enabled") {
		displayProcConfigEnabledDeprecationWarning()

		procConfigEnabled := strings.ToLower(config.GetString("process_config.enabled"))
		if procConfigEnabled == "disabled" {
			return false
		}
		result, _ := strconv.ParseBool(procConfigEnabled)
		return !result
	}
	return config.GetBool("process_config.container_collection.enabled")
}

// GetProcessCollectionEnabled retrieves the value of process_config.process_collection.enabled.
// If process_config.enabled is set, we display a deprecation warning and use that value instead.
func GetProcessCollectionEnabled(config Config) bool {
	if config.IsSet("process_config.enabled") {
		displayProcConfigEnabledDeprecationWarning()

		procConfigEnabled := strings.ToLower(config.GetString("process_config.enabled"))
		if procConfigEnabled == "disabled" {
			return false
		}
		enabled, _ := strconv.ParseBool(procConfigEnabled)
		return enabled
	}
	return config.GetBool("process_config.process_collection.enabled")
}
