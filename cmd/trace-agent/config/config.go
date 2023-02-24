// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// setMaxMemCPU sets watchdog's max_memory and max_cpu_percent parameters.
// If the agent is containerized, max_memory and max_cpu_percent are disabled by default.
// Resource limits are better handled by container runtimes and orchestrators.
func setMaxMemCPU(c *config.AgentConfig, isContainerized bool) {
	if coreconfig.Datadog.IsSet("apm_config.max_cpu_percent") {
		c.MaxCPU = coreconfig.Datadog.GetFloat64("apm_config.max_cpu_percent") / 100
	} else if isContainerized {
		log.Debug("Running in a container and apm_config.max_cpu_percent is not set, setting it to 0")
		c.MaxCPU = 0
	}

	if coreconfig.Datadog.IsSet("apm_config.max_memory") {
		c.MaxMemory = coreconfig.Datadog.GetFloat64("apm_config.max_memory")
	} else if isContainerized {
		log.Debug("Running in a container and apm_config.max_memory is not set, setting it to 0")
		c.MaxMemory = 0
	}
}
