// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// A Launcher starts and stops new tailers for every new containers discovered by autodiscovery.
type Launcher struct {
	pipelineProvider  pipeline.Provider
	sources           *config.LogSources
	services          *service.Services
	activeSources     []*config.LogSource
	pendingContainers map[string]*Container
	tailers           map[string]*Tailer
	cli               *client.Client
	registry          auditor.Registry
	stop              chan struct{}
}

// NewLauncher returns a new launcher
func NewLauncher(sources *config.LogSources, services *service.Services, pipelineProvider pipeline.Provider, registry auditor.Registry) (*Launcher, error) {
	launcher := &Launcher{
		pipelineProvider:  pipelineProvider,
		sources:           sources,
		services:          services,
		tailers:           make(map[string]*Tailer),
		pendingContainers: make(map[string]*Container),
		registry:          registry,
		stop:              make(chan struct{}),
	}
	err := launcher.setup()
	if err != nil {
		return nil, err
	}
	return launcher, nil
}

// setup initializes the docker client and the tagger,
// returns an error if it fails.
func (l *Launcher) setup() error {
	var err error
	// create a new docker client
	l.cli, err = NewClient()
	if err != nil {
		return err
	}
	// initialize the tagger
	err = tagger.Init()
	if err != nil {
		return err
	}
	return nil
}

// Start starts the Launcher
func (l *Launcher) Start() {
	go l.run()

	if config.LogsAgent.GetBool("logs_config.container_collect_all") {
		// append a new source to collect all logs from all containers
		log.Infof("Will collect all logs from all containers")
		source := config.NewLogSource("container_collect_all", &config.LogsConfig{
			Type:    config.DockerType,
			Service: "docker",
			Source:  "docker",
		})
		l.sources.AddSource(source)
	}
}

// Stop stops the Launcher and its tailers in parallel,
// this call returns only when all the tailers are stopped.
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
		delete(l.tailers, tailer.ContainerID)
	}
	stopper.Stop()
}

// run starts and stops new tailers when it receives a new source
// or a new service which is mapped to a container.
func (l *Launcher) run() {
	for {
		select {
		case newService := <-l.services.GetAddedServices(service.Docker):
			// detected a new container running on the host,
			dockerContainer, err := GetContainer(l.cli, newService.Identifier)
			if err != nil {
				log.Warnf("Could not find container with id: %v", err)
				continue
			}
			container := NewContainer(dockerContainer, newService)
			source := container.FindSource(l.activeSources)
			switch {
			case source != nil:
				// a source matches with the container, start a new tailer
				l.startTailer(container, source)
			default:
				// no source matches with the container but a matching source may not have been
				// emitted yet or the container may contain an autodiscovery identifier
				// so it's put in a cache until a matching source is found.
				l.pendingContainers[newService.Identifier] = container
			}
		case source := <-l.sources.GetSourceStreamForType(config.DockerType):
			// detected a new source that has been created either from a configuration file,
			// a docker label or a pod annotation.
			l.activeSources = append(l.activeSources, source)
			pendingContainers := make(map[string]*Container)
			for _, container := range l.pendingContainers {
				if container.IsMatch(source) {
					// found a container matching the new source, start a new tailer
					l.startTailer(container, source)
				} else {
					// keep the container in cache until
					pendingContainers[container.service.Identifier] = container
				}
			}
			// keep the containers that have not found any source yet for next iterations
			l.pendingContainers = pendingContainers
		case removedService := <-l.services.GetRemovedServices(service.Docker):
			// detected that a container has been stopped.
			containerID := removedService.Identifier
			l.stopTailer(containerID)
			delete(l.pendingContainers, containerID)
		case <-l.stop:
			// no docker container should be tailed anymore
			return
		}
	}
}

// startTailer starts a new tailer for the container matching with the source.
func (l *Launcher) startTailer(container *Container, source *config.LogSource) {
	containerID := container.service.Identifier
	if _, isTailed := l.tailers[containerID]; isTailed {
		log.Warnf("Can't tail twice the same container: %v", ShortContainerID(containerID))
		return
	}

	tailer := NewTailer(l.cli, containerID, source, l.pipelineProvider.NextPipelineChan())

	// conpute the offset to prevent from missing or duplicating logs
	since, err := Since(l.registry, tailer.Identifier(), container.service.CreationTime)
	if err != nil {
		log.Warnf("Could not recover tailing from last committed offset: %v", ShortContainerID(containerID), err)
	}

	// start the tailer
	err = tailer.Start(since)
	if err != nil {
		log.Warnf("Could not start tailer: %v", containerID, err)
	}

	// keep the tailer in track to stop it later on
	l.tailers[containerID] = tailer
}

// stopTailer stops the tailer matching the containerID.
func (l *Launcher) stopTailer(containerID string) {
	if tailer, isTailed := l.tailers[containerID]; isTailed {
		go tailer.Stop()
		delete(l.tailers, containerID)
	}
}
