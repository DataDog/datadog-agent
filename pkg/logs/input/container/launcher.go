// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

// NewLauncher returns a new container launcher depending on the environment.
// As a user can run on Kubernetes and mount both '/var/log/pods'
// and the docker socket to collect metrics,
// we first attempt to initialize the kubernetes launcher
// and fallback to the docker launcher if the initialization failed.
func NewLauncher(collectAll bool, sources *config.LogSources, services *service.Services, pipelineProvider pipeline.Provider, registry auditor.Registry) restart.Restartable {
	// attempt to initialize a kubernetes launcher
	kubernetesLauncher, err := kubernetes.NewLauncher(sources, services, collectAll)
	if err == nil {
		log.Info("Kubernetes launcher initialized")
		return kubernetesLauncher
	}
	log.Infof("Could not setup the kubernetes launcher: %v", err)

	// attempt to initialize a docker launcher
	launcher, err := docker.NewLauncher(sources, services, pipelineProvider, registry)
	if err == nil {
		log.Info("Docker launcher initialized")
		return launcher
	}
	log.Infof("Could not setup the docker launcher: %v", err)

	log.Infof("Container logs won't be collected")
	return NewNoopLauncher()
}
