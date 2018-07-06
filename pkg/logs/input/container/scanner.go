// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// NewScanner returns a new container scanner.
func NewScanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) restart.Restartable {
	if config.LogsAgent.GetBool("logs_config.container_collect_all") {
		if scanner, err := NewKubeScanner(sources, pp, auditor); err == nil {
			// Fow now, avoid manually scanning docker containers when in a
			// kubernetes environment, and rely on Kubernetes API.
			return scanner
		}
		// Append a fake source to collect all logs from all containers.
		dockerConfig := &config.LogsConfig{
			Type:    config.DockerType,
			Service: "docker",
			Source:  "docker",
		}
		sources.AddSource(config.NewLogSource("container_collect_all", dockerConfig))
	}
	return NewDockerScanner(sources, pp, auditor)
}
