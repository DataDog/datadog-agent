// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
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

// loadProcessTransforms loads transforms associated with process config settings.
func loadProcessTransforms(config pkgconfigmodel.Config) {
	if config.IsConfigured("process_config.enabled") {
		log.Warn("process_config.enabled is deprecated, use process_config.container_collection.enabled " +
			"and process_config.process_collection.enabled instead, " +
			"see https://docs.datadoghq.com/infrastructure/process#installation for more information")

		// The deprecated setting only fills in the settings that replaced it: user configuration
		// wins, while defaults and infra-mode values do not shadow it.
		setCollection := func(key string, enabled bool) {
			if config.GetSource(key).IsGreaterThan(pkgconfigmodel.SourceInfraMode) {
				return
			}
			config.Set(key, enabled, pkgconfigmodel.SourceAgentRuntime)
		}

		procConfigEnabled := strings.ToLower(config.GetString("process_config.enabled"))
		if procConfigEnabled == "disabled" {
			setCollection("process_config.process_collection.enabled", false)
			setCollection("process_config.container_collection.enabled", false)
		} else if enabled, _ := strconv.ParseBool(procConfigEnabled); enabled { // "true"
			setCollection("process_config.process_collection.enabled", true)
			setCollection("process_config.container_collection.enabled", false)
		} else { // "false"
			setCollection("process_config.process_collection.enabled", false)
			setCollection("process_config.container_collection.enabled", true)
		}

		// Normalize the deprecated setting to the process collection value it ended up with. Consumers
		// OR it with the two settings that replaced it, so leaving it enabled would keep process checks
		// running after the user turned process collection off.
		if config.GetBool("process_config.process_collection.enabled") {
			config.Set("process_config.enabled", "true", pkgconfigmodel.SourceAgentRuntime)
		} else {
			config.Set("process_config.enabled", "disabled", pkgconfigmodel.SourceAgentRuntime)
		}
	}
}
