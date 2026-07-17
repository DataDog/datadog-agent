// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"net"
	"strconv"
	"strings"
	"sync"

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

	// DefaultProcessDiscoveryHintFrequency is the default frequency in terms of number of checks which we send a process discovery hint
	DefaultProcessDiscoveryHintFrequency = 60
)

// setupProcesses is meant to be called multiple times for different configs, but overrides apply to all configs, so
// we need to make sure it is only applied once
var processesAddOverrideOnce sync.Once

// procBindEnvAndSetDefault is a helper function that generates both "DD_PROCESS_CONFIG_" and "DD_PROCESS_AGENT_" prefixes from a key.
// We need this helper function because the standard BindEnvAndSetDefault can only generate one prefix from a key.
func procBindEnvAndSetDefault(config pkgconfigmodel.Setup, key string, val interface{}) {
	// Env var names from here are mirrored in pkg/procmgr/rust/src/config_gate/env_bindings.rs.
	// Uppercase, replace "." with "_" and add "DD_" prefix to key so that we follow the same environment
	// variable convention as the core agent.
	processConfigKey := "DD_" + strings.ReplaceAll(strings.ToUpper(key), ".", "_")
	processAgentKey := strings.Replace(processConfigKey, "PROCESS_CONFIG", "PROCESS_AGENT", 1)

	envs := []string{processConfigKey, processAgentKey}
	config.BindEnvAndSetDefault(key, val, envs...)
}

// loadProcessTransforms loads transforms associated with process config settings.
func loadProcessTransforms(config pkgconfigmodel.Config) {
	if config.IsConfigured("process_config.enabled") {
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
