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

	// DefaultProcessQueueSize is the default max amount of process-agent checks that can be buffered in memory if the forwarder can't consume them fast enough (e.g. due to network disruption)
	// This can be fairly high as the input should get throttled by queue bytes first.
	// Assuming we generate ~8 checks/minute (for process/network), this should allow buffering of ~30 minutes of data assuming it fits within the queue bytes memory budget
	DefaultProcessQueueSize = 256

	// DefaultProcessRTQueueSize is the default max amount of process-agent realtime checks that can be buffered in memory
	// We set a small queue size for real-time message because they get staled very quickly, thus we only keep the latest several payloads
	DefaultProcessRTQueueSize = 5

	// DefaultProcessQueueBytes is the default amount of process-agent check data (in bytes) that can be buffered in memory
	// Allow buffering up to 60 megabytes of payload data in total
	DefaultProcessQueueBytes = 60 * 1000 * 1000
)

// setupProcesses is meant to be called multiple times for different configs, but overrides apply to all configs, so
// we need to make sure it is only applied once
var processesAddOverrideOnce sync.Once

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
	procBindEnvAndSetDefault(config, "process_config.queue_size", DefaultProcessQueueSize)
	procBindEnvAndSetDefault(config, "process_config.process_queue_bytes", DefaultProcessQueueBytes)
	procBindEnvAndSetDefault(config, "process_config.rt_queue_size", DefaultProcessRTQueueSize)
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
	// Use PDH API to collect performance counter data for process check on Windows
	procBindEnvAndSetDefault(config, "process_config.windows.use_perf_counters", false)
	config.SetKnown("process_config.additional_endpoints.*")
	config.SetKnown("process_config.container_source")
	config.SetKnown("process_config.intervals.connections")
	config.SetKnown("process_config.expvar_port")
	procBindEnvAndSetDefault(config, "process_config.log_file", DefaultProcessAgentLogFile)
	procBindEnvAndSetDefault(config, "process_config.internal_profiling.enabled", false)
	procBindEnvAndSetDefault(config, "process_config.grpc_connection_timeout_secs", DefaultGRPCConnectionTimeoutSecs)
	procBindEnvAndSetDefault(config, "process_config.remote_tagger", false)
	procBindEnvAndSetDefault(config, "process_config.disable_realtime_checks", false)

	// Process Discovery Check
	config.BindEnvAndSetDefault("process_config.process_discovery.enabled", true,
		"DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_CONFIG_DISCOVERY_ENABLED", // Also bind old environment variables
		"DD_PROCESS_AGENT_DISCOVERY_ENABLED",
	)
	procBindEnvAndSetDefault(config, "process_config.process_discovery.interval", 4*time.Hour)

	processesAddOverrideOnce.Do(func() {
		AddOverrideFunc(loadProcessTransforms)
	})
}

// loadProcessTransforms loads transforms associated with process config settings.
func loadProcessTransforms(config Config) {
	if config.IsSet("process_config.enabled") {
		log.Info("process_config.enabled is deprecated, use process_config.container_collection.enabled " +
			"and process_config.process_collection.enabled instead, " +
			"see https://docs.datadoghq.com/infrastructure/process#installation for more information")
		procConfigEnabled := strings.ToLower(config.GetString("process_config.enabled"))
		if procConfigEnabled == "disabled" {
			config.Set("process_config.process_collection.enabled", false)
			config.Set("process_config.container_collection.enabled", false)
		} else if enabled, _ := strconv.ParseBool(procConfigEnabled); enabled { // "true"
			config.Set("process_config.process_collection.enabled", true)
			config.Set("process_config.container_collection.enabled", false)
		} else { // "false"
			config.Set("process_config.process_collection.enabled", false)
			config.Set("process_config.container_collection.enabled", true)
		}
	}
}
