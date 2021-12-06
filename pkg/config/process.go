package config

import (
	"os"
	"time"
)

func setupProcesses(config Config) {
	// Simple macro that allows env variables to be assigned to a prefix
	prEnv := func(env string) []string {
		return []string{"DD_PROCESS_CONFIG_" + env, "DD_PROCESS_AGENT_" + env}
	}

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
	config.BindEnvAndSetDefault("process_config.dd_agent_bin", defaultDDAgentBin, prEnv("DD_AGENT_BIN")...)
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
	config.BindEnvAndSetDefault("process_config.log_file", defaultProcessAgentLogFile, prEnv("LOG_FILE")...)
	config.SetKnown("process_config.internal_profiling.enabled")

	config.BindEnvAndSetDefault("process_config.remote_tagger", true)

	// Process Discovery Check
	config.BindEnvAndSetDefault("process_config.process_discovery.enabled", false)
	config.BindEnvAndSetDefault("process_config.process_discovery.interval", 4*time.Hour)
}
