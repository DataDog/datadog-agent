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

// A Launcher listens for stdout and stderr of containers
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

// NewLauncher returns an initialized Launcher
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
// returns an error if it failed.
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
}

// Stop stops the Launcher and its tailers in parallel,
// this call returns only when all the tailers are stopped
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
		delete(l.tailers, tailer.ContainerID)
	}
	stopper.Stop()
}

// run starts and stops new tailers.
func (l *Launcher) run() {
	for {
		select {
		case newService := <-l.services.GetAddedServices(service.Docker):
			dockerContainer, err := GetContainer(l.cli, newService.Identifier)
			if err != nil {
				log.Warnf("Could not find container with id: %v", err)
				continue
			}
			container := NewContainer(dockerContainer, newService)
			if source := container.FindSource(l.activeSources); source != nil {
				l.startTailer(source, container)
			} else {
				l.pendingContainers[newService.Identifier] = container
			}
		case removedService := <-l.services.GetRemovedServices(service.Docker):
			containerID := removedService.Identifier
			l.stopTailer(containerID)
			if _, exists := l.pendingContainers[containerID]; exists {
				delete(l.pendingContainers, containerID)
			}
		case source := <-l.sources.GetSourceStreamForType(config.DockerType):
			l.activeSources = append(l.activeSources, source)
			pendingContainers := make(map[string]*Container)
			for _, container := range l.pendingContainers {
				if container.IsMatch(source) {
					l.startTailer(source, container)
				} else {
					pendingContainers[container.service.Identifier] = container
				}
			}
			l.pendingContainers = pendingContainers
		case <-l.stop:
			// no docker container should be tailed anymore
			return
		}
	}
}

// startTailer starts a new tailer for the source and the container.
func (l *Launcher) startTailer(source *config.LogSource, container *Container) {
	containerID := container.service.Identifier
	containerImage := container.container.Image

	log.Infof("Detected container %v - %v", containerImage, ShortContainerID(containerID))
	tailer := NewTailer(l.cli, containerID, source, l.pipelineProvider.NextPipelineChan())

	since, err := Since(l.registry, tailer.Identifier(), container.service.CreationTime)
	if err != nil {
		log.Warnf("Could not recover tailing from last committed offset: %v", ShortContainerID(containerID), err)
	}

	err = tailer.Start(since)
	if err != nil {
		log.Warnf("Could not start tailer: %v", containerID, err)
	}

	l.tailers[containerID] = tailer
}

// stopTailer stops the tailer corresponding to the log source.
func (l *Launcher) stopTailer(containerID string) {
	if tailer, isTailed := l.tailers[containerID]; isTailed {
		go tailer.Stop()
		delete(l.tailers, containerID)
	}
}
