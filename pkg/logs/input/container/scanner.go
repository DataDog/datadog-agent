// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/input/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewScanner returns a new container scanner,
// by default returns a docker scanner unless it could not be set up properly,
// in which case returns a kubernetes scanner if `logs_config.containers_all` is set to true.
func NewScanner(sources *config.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry) (restart.Restartable, error) {
	var scanner restart.Restartable
	var err error
	if config.LogsAgent.GetBool("logs_config.container_collect_all") {
		// attempt to initialize a docker scanner
		scanner, err = docker.NewScanner(sources, pipelineProvider, registry)
		if err == nil {
			source := config.NewLogSource("container_collect_all", &config.LogsConfig{
				Type:    config.DockerType,
				Service: "docker",
				Source:  "docker",
			})
			sources.AddSource(source)
			return scanner, nil
		}
		// attempt to initialize a kubernetes scanner
		log.Warnf("Could not setup the docker scanner, falling back to the kubernetes one: %v", err)
		scanner, err = kubernetes.NewScanner(sources)
		if err == nil {
			return scanner, nil
		}
		log.Warnf("Could not setup the kubernetes scanner: %v", err)
		return nil, err
	}
	scanner, err = docker.NewScanner(sources, pipelineProvider, registry)
	if err != nil {
		log.Warnf("Could not setup the docker scanner: %v", err)
		return nil, err
	}
	return scanner, nil
}
