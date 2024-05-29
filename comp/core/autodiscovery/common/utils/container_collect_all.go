// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// AddContainerCollectAllConfigs adds a config template containing an empty
// LogsConfig when `logs_config.container_collect_all` is set.  This config
// will be filtered out during config resolution if another config template
// also has logs configuration.
func AddContainerCollectAllConfigs(configs []integration.Config, adIdentifier string) []integration.Config {
	if !config.Datadog().GetBool("logs_config.container_collect_all") {
		return configs
	}

	// create an almost-empty config which just says "please log this
	// container" to the logs agent.
	configs = append(configs, integration.Config{
		Name:          "container_collect_all",
		ADIdentifiers: []string{adIdentifier},
		LogsConfig:    []byte("[{}]"),
	})

	return configs
}
