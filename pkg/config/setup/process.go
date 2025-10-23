// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"encoding/json"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
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

	// DefaultProcessMaxPerMessage is the default maximum number of processes, or containers per message. Note: Only change if the defaults are causing issues.
	DefaultProcessMaxPerMessage = 100

	// ProcessMaxPerMessageLimit is the maximum allowed value for maximum number of processes, or containers per message.
	ProcessMaxPerMessageLimit = 10000

	// DefaultProcessMaxMessageBytes is the default max for size of a message containing processes or container data. Note: Only change if the defaults are causing issues.
	DefaultProcessMaxMessageBytes = 1000000

	// ProcessMaxMessageBytesLimit is the maximum allowed value for the maximum size of a message containing processes or container data.
	ProcessMaxMessageBytesLimit = 4000000

	// DefaultProcessExpVarPort is the default port used by the process-agent expvar server
	DefaultProcessExpVarPort = 6062

	// DefaultProcessCmdPort is the default port used by process-agent to run a runtime settings server
	DefaultProcessCmdPort = 6162

	// DefaultProcessEntityStreamPort is the default port used by the process-agent to expose Process Entities
	DefaultProcessEntityStreamPort = 6262

	// DefaultProcessEndpoint is the default endpoint for the process agent to send payloads to
	DefaultProcessEndpoint = "https://process.datadoghq.com."

	// DefaultProcessEventsEndpoint is the default endpoint for the process agent to send event payloads to
	DefaultProcessEventsEndpoint = "https://process-events.datadoghq.com."

	// DefaultProcessEventStoreMaxItems is the default maximum amount of events that can be stored in the Event Store
	DefaultProcessEventStoreMaxItems = 200

	// DefaultProcessEventStoreMaxPendingPushes is the default amount of pending push operations can be handled by the Event Store
	DefaultProcessEventStoreMaxPendingPushes = 10

	// DefaultProcessEventStoreMaxPendingPulls is the default amount of pending pull operations can be handled by the Event Store
	DefaultProcessEventStoreMaxPendingPulls = 10

	// DefaultProcessEventStoreStatsInterval is the default frequency at which the event store sends stats about expired events, in seconds
	DefaultProcessEventStoreStatsInterval = 20

	// DefaultProcessEventsMinCheckInterval is the minimum interval allowed for the process_events check
	DefaultProcessEventsMinCheckInterval = time.Second

	// DefaultProcessEventsCheckInterval is the default interval used by the process_events check
	DefaultProcessEventsCheckInterval = 10 * time.Second

	// DefaultProcessDiscoveryHintFrequency is the default frequency in terms of number of checks which we send a process discovery hint
	DefaultProcessDiscoveryHintFrequency = 60
)

// setupProcesses is meant to be called multiple times for different configs, but overrides apply to all configs, so
// we need to make sure it is only applied once
var processesAddOverrideOnce sync.Once

// procBindEnvAndSetDefault is a helper function that generates both "DD_PROCESS_CONFIG_" and "DD_PROCESS_AGENT_" prefixes from a key.
// We need this helper function because the standard BindEnvAndSetDefault can only generate one prefix from a key.
func procBindEnvAndSetDefault(config pkgconfigmodel.Setup, key string, val interface{}) {
	// Uppercase, replace "." with "_" and add "DD_" prefix to key so that we follow the same environment
	// variable convention as the core agent.
	processConfigKey := "DD_" + strings.ReplaceAll(strings.ToUpper(key), ".", "_")
	processAgentKey := strings.Replace(processConfigKey, "PROCESS_CONFIG", "PROCESS_AGENT", 1)

	envs := []string{processConfigKey, processAgentKey}
	config.BindEnvAndSetDefault(key, val, envs...)
}

// procBindEnv is a helper function that generates both "DD_PROCESS_CONFIG_" and "DD_PROCESS_AGENT_" prefixes from a key, but does not set a default.
// We need this helper function because the standard BindEnv can only generate one prefix from a key.
func procBindEnv(config pkgconfigmodel.Setup, key string) {
	processConfigKey := "DD_" + strings.ReplaceAll(strings.ToUpper(key), ".", "_")
	processAgentKey := strings.Replace(processConfigKey, "PROCESS_CONFIG", "PROCESS_AGENT", 1)

	config.BindEnv(key, processConfigKey, processAgentKey) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
}

func setupProcesses(config pkgconfigmodel.Setup) {
	// "process_config.enabled" is deprecated. We must still be able to detect if it is present, to know if we should use it
	// or container_collection.enabled and process_collection.enabled.
	procBindEnv(config, "process_config.enabled")
	procBindEnvAndSetDefault(config, "process_config.container_collection.enabled", true)
	procBindEnvAndSetDefault(config, "process_config.process_collection.enabled", false)
	procBindEnvAndSetDefault(config, "process_config.process_collection.use_wlm", runtime.GOOS == "linux")

	// This allows for the process check to run in the core agent but is for linux only
	procBindEnvAndSetDefault(config, "process_config.run_in_core_agent.enabled", runtime.GOOS == "linux")

	config.BindEnv("process_config.process_dd_url", //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
		"DD_PROCESS_CONFIG_PROCESS_DD_URL",
		"DD_PROCESS_AGENT_PROCESS_DD_URL",
		"DD_PROCESS_AGENT_URL",
		"DD_PROCESS_CONFIG_URL",
	)
	procBindEnv(config, "process_config.events_dd_url")
	procBindEnv(config, "process_config.dd_agent_env")
	procBindEnv(config, "process_config.intervals.process_realtime")
	procBindEnvAndSetDefault(config, "process_config.queue_size", DefaultProcessQueueSize)
	procBindEnvAndSetDefault(config, "process_config.process_queue_bytes", DefaultProcessQueueBytes)
	procBindEnvAndSetDefault(config, "process_config.rt_queue_size", DefaultProcessRTQueueSize)
	procBindEnvAndSetDefault(config, "process_config.max_per_message", DefaultProcessMaxPerMessage)
	procBindEnvAndSetDefault(config, "process_config.max_message_bytes", DefaultProcessMaxMessageBytes)
	procBindEnvAndSetDefault(config, "process_config.cmd_port", DefaultProcessCmdPort)
	procBindEnv(config, "process_config.intervals.process")
	procBindEnv(config, "process_config.blacklist_patterns")
	procBindEnv(config, "process_config.intervals.container")
	procBindEnv(config, "process_config.intervals.container_realtime")
	procBindEnvAndSetDefault(config, "process_config.dd_agent_bin", DefaultDDAgentBin)
	config.BindEnv("process_config.custom_sensitive_words", //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
		"DD_CUSTOM_SENSITIVE_WORDS",
		"DD_PROCESS_CONFIG_CUSTOM_SENSITIVE_WORDS",
		"DD_PROCESS_AGENT_CUSTOM_SENSITIVE_WORDS")
	config.ParseEnvAsStringSlice("process_config.custom_sensitive_words", func(val string) []string {
		// historically we accept DD_CUSTOM_SENSITIVE_WORDS as "w1,w2,..." but Viper expects the user to set a list as ["w1","w2",...]
		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			res := []string{}
			if err := json.Unmarshal([]byte(val), &res); err != nil {
				log.Errorf("Error parsing JSON value for 'process_config.custom_sensitive_words' from env vars: %s", err)
				return nil
			}
			return res
		}

		return strings.Split(val, ",")
	})
	config.BindEnv("process_config.scrub_args", //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
		"DD_SCRUB_ARGS",
		"DD_PROCESS_CONFIG_SCRUB_ARGS",
		"DD_PROCESS_AGENT_SCRUB_ARGS")
	config.BindEnv("process_config.strip_proc_arguments", //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
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
	procBindEnvAndSetDefault(config, "process_config.events_additional_endpoints", make(map[string][]string))
	procBindEnv(config, "process_config.intervals.connections")
	procBindEnvAndSetDefault(config, "process_config.expvar_port", DefaultProcessExpVarPort)
	procBindEnvAndSetDefault(config, "process_config.log_file", DefaultProcessAgentLogFile)
	procBindEnvAndSetDefault(config, "process_config.internal_profiling.enabled", false)
	procBindEnvAndSetDefault(config, "process_config.grpc_connection_timeout_secs", DefaultGRPCConnectionTimeoutSecs)
	procBindEnvAndSetDefault(config, "process_config.disable_realtime_checks", false)
	procBindEnvAndSetDefault(config, "process_config.ignore_zombie_processes", false)

	// Process Discovery Check
	config.BindEnvAndSetDefault("process_config.process_discovery.enabled", true,
		"DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED",
		"DD_PROCESS_CONFIG_DISCOVERY_ENABLED", // Also bind old environment variables
		"DD_PROCESS_AGENT_DISCOVERY_ENABLED",
	)
	procBindEnvAndSetDefault(config, "process_config.process_discovery.interval", 4*time.Hour)

	procBindEnvAndSetDefault(config, "process_config.process_discovery.hint_frequency", DefaultProcessDiscoveryHintFrequency)

	procBindEnvAndSetDefault(config, "process_config.drop_check_payloads", []string{})

	// Process Lifecycle Events
	procBindEnvAndSetDefault(config, "process_config.event_collection.store.max_items", DefaultProcessEventStoreMaxItems)
	procBindEnvAndSetDefault(config, "process_config.event_collection.store.max_pending_pushes", DefaultProcessEventStoreMaxPendingPushes)
	procBindEnvAndSetDefault(config, "process_config.event_collection.store.max_pending_pulls", DefaultProcessEventStoreMaxPendingPulls)
	procBindEnvAndSetDefault(config, "process_config.event_collection.store.stats_interval", DefaultProcessEventStoreStatsInterval)
	procBindEnvAndSetDefault(config, "process_config.event_collection.enabled", false)
	procBindEnvAndSetDefault(config, "process_config.event_collection.interval", DefaultProcessEventsCheckInterval)

	procBindEnvAndSetDefault(config, "process_config.cache_lookupid", false)

	procBindEnvAndSetDefault(config, "process_config.language_detection.grpc_port", DefaultProcessEntityStreamPort)

	processesAddOverrideOnce.Do(func() {
		pkgconfigmodel.AddOverrideFunc(loadProcessTransforms)
		pkgconfigmodel.AddOverrideFunc(overrideRunInCoreAgentConfig)
	})
}

// overrideRunInCoreAgentConfig sets the process_config.run_in_core_agent.enabled to false in non-Linux environments.
// Otherwise, it is a no-op.
func overrideRunInCoreAgentConfig(config pkgconfigmodel.Config) {
	if runtime.GOOS != "linux" {
		config.Set("process_config.run_in_core_agent.enabled", false, pkgconfigmodel.SourceAgentRuntime)
	}
}

// loadProcessTransforms loads transforms associated with process config settings.
func loadProcessTransforms(config pkgconfigmodel.Config) {
	if config.IsSet("process_config.enabled") {
		log.Warn("process_config.enabled is deprecated, use process_config.container_collection.enabled " +
			"and process_config.process_collection.enabled instead, " +
			"see https://docs.datadoghq.com/infrastructure/process#installation for more information")
		procConfigEnabled := strings.ToLower(config.GetString("process_config.enabled"))
		if procConfigEnabled == "disabled" {
			config.Set("process_config.process_collection.enabled", false, pkgconfigmodel.SourceAgentRuntime)
			config.Set("process_config.container_collection.enabled", false, pkgconfigmodel.SourceAgentRuntime)
		} else if enabled, _ := strconv.ParseBool(procConfigEnabled); enabled { // "true"
			config.Set("process_config.process_collection.enabled", true, pkgconfigmodel.SourceAgentRuntime)
			config.Set("process_config.container_collection.enabled", false, pkgconfigmodel.SourceAgentRuntime)
			config.Set("process_config.enabled", "true", pkgconfigmodel.SourceAgentRuntime)
		} else { // "false"
			config.Set("process_config.process_collection.enabled", false, pkgconfigmodel.SourceAgentRuntime)
			config.Set("process_config.container_collection.enabled", true, pkgconfigmodel.SourceAgentRuntime)
			config.Set("process_config.enabled", "disabled", pkgconfigmodel.SourceAgentRuntime)
		}
	}
}

// GetProcessAPIAddressPort returns the API endpoint of the process agent
func GetProcessAPIAddressPort(config pkgconfigmodel.Reader) (string, error) {
	address, err := GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	port := config.GetInt("process_config.cmd_port")
	if port <= 0 {
		log.Warnf("Invalid process_config.cmd_port -- %d, using default port %d", port, DefaultProcessCmdPort)
		port = DefaultProcessCmdPort
	}

	addrPort := net.JoinHostPort(address, strconv.Itoa(port))
	return addrPort, nil
}
