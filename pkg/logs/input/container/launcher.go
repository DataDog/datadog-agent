// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package container

import (
	"github.com/StackVista/stackstate-agent/pkg/logs/auditor"
	"github.com/StackVista/stackstate-agent/pkg/logs/config"
	"github.com/StackVista/stackstate-agent/pkg/logs/input/docker"
	"github.com/StackVista/stackstate-agent/pkg/logs/input/kubernetes"
	"github.com/StackVista/stackstate-agent/pkg/logs/pipeline"
	"github.com/StackVista/stackstate-agent/pkg/logs/restart"
	"github.com/StackVista/stackstate-agent/pkg/logs/service"

	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// NewLauncher returns a new container launcher.
// By default, it returns a docker launcher that uses the docker socket to collect logs.
// The docker launcher can be used both on a non kubernetes and kubernetes environment.
// When a docker launcher cannot be initialized properly, the launcher will attempt to
// initialize a kubernetes launcher which will tail logs files (in '/var/log/pods') of all
// the containers running on the kubernetes cluster and matching the autodiscovery configuration.
func NewLauncher(collectAll bool, sources *config.LogSources, services *service.Services, pipelineProvider pipeline.Provider, registry auditor.Registry) restart.Restartable {
	// attempt to initialize a docker launcher
	log.Info("Trying to initialize docker launcher")
	launcher, err := docker.NewLauncher(sources, services, pipelineProvider, registry)
	if err == nil {
		log.Info("Docker launcher initialized")
		return launcher
	}
	log.Infof("Could not setup the docker launcher: %v", err)

	// attempt to initialize a kubernetes launcher
	log.Info("Trying to initialize kubernetes launcher")
	kubernetesLauncher, err := kubernetes.NewLauncher(sources, services, collectAll)
	if err == nil {
		log.Info("Kubernetes launcher initialized")
		return kubernetesLauncher
	}
	log.Infof("Could not setup the kubernetes launcher: %v", err)
	log.Infof("Container logs won't be collected")
	return NewNoopLauncher()
}
