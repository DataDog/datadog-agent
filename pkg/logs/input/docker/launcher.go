// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

const (
	backoffInitialDuration = 1 * time.Second
	backoffMaxDuration     = 60 * time.Second
)

// A Launcher starts and stops new tailers for every new containers discovered by autodiscovery.
type Launcher struct {
	pipelineProvider   pipeline.Provider
	sources            chan *config.LogSource
	addedServices      chan *service.Service
	removedServices    chan *service.Service
	activeSources      []*config.LogSource
	pendingContainers  map[string]*Container
	tailers            map[string]*Tailer
	cli                *client.Client
	registry           auditor.Registry
	stop               chan struct{}
	erroredContainerID chan string
	lock               *sync.Mutex
}

// NewLauncher returns a new launcher
func NewLauncher(sources *config.LogSources, services *service.Services, pipelineProvider pipeline.Provider, registry auditor.Registry) (*Launcher, error) {
	launcher := &Launcher{
		pipelineProvider:   pipelineProvider,
		tailers:            make(map[string]*Tailer),
		pendingContainers:  make(map[string]*Container),
		registry:           registry,
		stop:               make(chan struct{}),
		erroredContainerID: make(chan string),
		lock:               &sync.Mutex{},
	}
	err := launcher.setup()
	if err != nil {
		return nil, err
	}
	// Sources and services are added after the setup to avoid creating
	// a channel that will lock the scheduler in case of setup failure
	// FIXME(achntrl): Find a better way of choosing the right launcher
	// between Docker and Kubernetes
	launcher.sources = sources.GetSourceStreamForType(config.DockerType)
	launcher.addedServices = services.GetAddedServices(service.Docker)
	launcher.removedServices = services.GetRemovedServices(service.Docker)
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
}

// Stop stops the Launcher and its tailers in parallel,
// this call returns only when all the tailers are stopped.
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
		l.removeTailer(tailer.ContainerID)
	}
	stopper.Stop()
}

// run starts and stops new tailers when it receives a new source
// or a new service which is mapped to a container.
func (l *Launcher) run() {
	for {
		select {
		case service := <-l.addedServices:
			// detected a new container running on the host,
			dockerContainer, err := GetContainer(l.cli, service.Identifier)
			if err != nil {
				log.Warnf("Could not find container with id: %v", err)
				continue
			}
			container := NewContainer(dockerContainer, service)
			source := container.FindSource(l.activeSources)
			switch {
			case source != nil:
				// a source matches with the container, start a new tailer
				source.SetParser(parser.NewDockerParser())
				l.startTailer(container, source)
			default:
				// no source matches with the container but a matching source may not have been
				// emitted yet or the container may contain an autodiscovery identifier
				// so it's put in a cache until a matching source is found.
				l.pendingContainers[service.Identifier] = container
			}
		case source := <-l.sources:
			// detected a new source that has been created either from a configuration file,
			// a docker label or a pod annotation.
			l.activeSources = append(l.activeSources, source)
			pendingContainers := make(map[string]*Container)
			for _, container := range l.pendingContainers {
				if container.IsMatch(source) {
					// found a container matching the new source, start a new tailer
					source.SetParser(parser.NewDockerParser())
					l.startTailer(container, source)
				} else {
					// keep the container in cache until
					pendingContainers[container.service.Identifier] = container
				}
			}
			// keep the containers that have not found any source yet for next iterations
			l.pendingContainers = pendingContainers
		case service := <-l.removedServices:
			// detected that a container has been stopped.
			containerID := service.Identifier
			l.stopTailer(containerID)
			delete(l.pendingContainers, containerID)
		case containerId := <-l.erroredContainerID:
			go l.restartTailer(containerId)
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

	tailer := NewTailer(l.cli, containerID, source, l.pipelineProvider.NextPipelineChan(), l.erroredContainerID)

	// compute the offset to prevent from missing or duplicating logs
	since, err := Since(l.registry, tailer.Identifier(), container.service.CreationTime)
	if err != nil {
		log.Warnf("Could not recover tailing from last committed offset: %v", ShortContainerID(containerID), err)
	}

	// start the tailer
	err = tailer.Start(since)
	if err != nil {
		log.Warnf("Could not start tailer: %v", containerID, err)
		return
	}

	// keep the tailer in track to stop it later on
	l.addTailer(containerID, tailer)
}

// stopTailer stops the tailer matching the containerID.
func (l *Launcher) stopTailer(containerID string) {
	if tailer, isTailed := l.tailers[containerID]; isTailed {
		go tailer.Stop()
		l.removeTailer(containerID)
	}
}

func (l *Launcher) restartTailer(containerID string) {
	backoffDuration := backoffInitialDuration
	cumulatedBackoff := 0 * time.Second
	var source *config.LogSource

	oldTailer, exists := l.tailers[containerID]
	if exists {
		source = oldTailer.source
		oldTailer.Stop()
		l.removeTailer(containerID)
	}

	tailer := NewTailer(l.cli, containerID, source, l.pipelineProvider.NextPipelineChan(), l.erroredContainerID)

	// compute the offset to prevent from missing or duplicating logs
	since, err := Since(l.registry, tailer.Identifier(), service.Before)
	if err != nil {
		log.Warnf("Could not recover last committed offset for container %v: %v", ShortContainerID(containerID), err)
	}

	for {
		if backoffDuration > backoffMaxDuration {
			log.Warnf("Could not resume tailing container %v", ShortContainerID(containerID))
			return
		}

		// start the tailer
		err = tailer.Start(since)
		if err != nil {
			log.Warnf("Could not start tailer for container %v: %v", ShortContainerID(containerID), err)
			time.Sleep(backoffDuration)
			cumulatedBackoff += backoffDuration
			backoffDuration *= 2
			continue
		}
		// keep the tailer in track to stop it later on
		l.addTailer(containerID, tailer)
		return
	}
}

func (l *Launcher) addTailer(containerID string, tailer *Tailer) {
	l.lock.Lock()
	l.tailers[containerID] = tailer
	l.lock.Unlock()
}

func (l *Launcher) removeTailer(containerID string) {
	l.lock.Lock()
	delete(l.tailers, containerID)
	l.lock.Unlock()
}
