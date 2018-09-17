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
	"github.com/DataDog/datadog-agent/pkg/logs/service"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewLauncher returns a new container scanner,
// by default returns a docker launcher that uses the docker socket to collect logs.
// The docker launcher can be used both on a non kubernetes and kubernetes environment.
// When a docker launcher can not be initialized properly and when the log collection is enabled for all containers,
// the scanner will attempt to initialize a kubernetes scanner which will detect and tail all the logs files localized
// in '/var/log/pods' of all the containers running on the kubernetes cluster.
func NewLauncher(sources *config.LogSources, services *service.Services, pipelineProvider pipeline.Provider, registry auditor.Registry) restart.Restartable {
	switch {
	case config.LogsAgent.GetBool("logs_config.container_collect_all"):
		// attempt to initialize a docker scanner
		launcher, err := docker.NewLauncher(sources, services, pipelineProvider, registry)
		if err == nil {
			return launcher
		}
		// attempt to initialize a kubernetes scanner
		log.Warnf("Could not setup the docker scanner, falling back to the kubernetes one: %v", err)
		scanner, err := kubernetes.NewLauncher(sources, services)
		if err == nil {
			return scanner
		}
		log.Warnf("Could not setup the kubernetes scanner: %v", err)
	default:
		launcher, err := docker.NewLauncher(sources, services, pipelineProvider, registry)
		if err == nil {
			return launcher
		}
		log.Warnf("Could not setup the docker scanner: %v", err)
	}
	return NewNoopLauncher()
}
