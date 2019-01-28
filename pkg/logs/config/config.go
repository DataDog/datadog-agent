// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
)

// ContainerCollectAll is the name of the docker integration that collect logs from all containers
const ContainerCollectAll = "container_collect_all"

// DefaultSources returns the default log sources that can be directly set from the datadog.yaml or through environment variables.
func DefaultSources() []*LogSource {
	var sources []*LogSource

	if coreConfig.Datadog.GetBool("logs_config.container_collect_all") {
		// append a new source to collect all logs from all containers
		source := NewLogSource(ContainerCollectAll, &LogsConfig{
			Type:    DockerType,
			Service: "docker",
			Source:  "docker",
		})
		sources = append(sources, source)
	}

	return sources
}
