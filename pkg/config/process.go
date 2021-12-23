// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"time"
)

const (
	// DefaultGRPCConnectionTimeoutSecs sets the default value for timeout when connecting to the agent
	DefaultGRPCConnectionTimeoutSecs = 60
)

func setupProcesses(config Config) {
	procBindEnvAndSetDefault := func(key string, val interface{}, env string) {
		config.BindEnvAndSetDefault(key, val, "DD_PROCESS_CONFIG_"+env, "DD_PROCESS_AGENT_"+env)
	}
	// Note that `BindEnvAndSetDefault` automatically creates environment variables with the `DD_PROCESS_CONFIG` prefix.
	// This means that we must manually add the same environment variable, but with the `DD_PROCESS_AGENT` prefix if we want to support that too.

	config.SetDefault("process_config.enabled", "false")
	// process_config.enabled is only used on Windows by the core agent to start the process agent service.
	// it can be set from file, but not from env. Override it with value from DD_PROCESS_AGENT_ENABLED.
	ddProcessAgentEnabled, found := os.LookupEnv("DD_PROCESS_AGENT_ENABLED")
	if found {
		AddOverride("process_config.enabled", ddProcessAgentEnabled)
	}

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
	procBindEnvAndSetDefault("process_config.dd_agent_bin", DefaultDDAgentBin, "DD_AGENT_BIN")
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
	procBindEnvAndSetDefault("process_config.log_file", DefaultProcessAgentLogFile, "LOG_FILE")
	config.SetKnown("process_config.internal_profiling.enabled")
	procBindEnvAndSetDefault("process_config.grpc_connection_timeout_secs", DefaultGRPCConnectionTimeoutSecs, "GRPC_CONNECTION_TIMEOUT_SECS")
	procBindEnvAndSetDefault("process_config.remote_tagger", true, "REMOTE_TAGGER")

	// Process Discovery Check
	procBindEnvAndSetDefault("process_config.process_discovery.enabled", false, "PROCESS_DISCOVERY_ENABLED")
	procBindEnvAndSetDefault("process_config.process_discovery.interval", 4*time.Hour, "PROCESS_DISCOVERY_INTERVAL")

	// Allows for enabling or disabling realtime mode
	procBindEnvAndSetDefault("process_config.allow_rt", true, "ALLOW_RT")
}
