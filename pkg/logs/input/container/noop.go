// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// noopLauncher consumes docker sources and services to ensure that no deadlock occurs when
// the docker launcher or the kubernetes scanner could not be set up properly and autodiscovery
// emits new valid docker integration configs, see [logs-scheduler](https://github.com/DataDog/datadog-agent/blob/master/pkg/logs/scheduler/scheduler.go).
type noopLauncher struct {
	sources  *config.LogSources
	services *service.Services
	stop     chan struct{}
}

// NewNoopLauncher returns a new noopLauncher.
func NewNoopLauncher(sources *config.LogSources, services *service.Services) restart.Restartable {
	return &noopLauncher{
		sources:  sources,
		services: services,
		stop:     make(chan struct{}),
	}
}

// Start does nothing
func (l *noopLauncher) Start() {
	go l.run()
}

// Stop stops the noopLauncher scanner.
func (l *noopLauncher) Stop() {
	l.stop <- struct{}{}
}

// run consumes docker sources and services and drop them directly.
func (l *noopLauncher) run() {
	for {
		select {
		case <-l.services.GetAddedServices(service.Docker):
			continue
		case <-l.services.GetRemovedServices(service.Docker):
			continue
		case <-l.sources.GetSourceStreamForType(config.DockerType):
			continue
		case <-l.stop:
			return
		}
	}
}
