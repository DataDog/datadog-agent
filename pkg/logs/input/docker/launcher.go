// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// A Launcher listens for stdout and stderr of containers
type Launcher struct {
	pipelineProvider pipeline.Provider
	sources          *config.LogSources
	activeSources    []*config.LogSource
	tailers          map[string]*Tailer
	cli              *client.Client
	registry         auditor.Registry
	stop             chan struct{}
}

// NewLauncher returns an initialized Launcher
func NewLauncher(sources *config.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry) (*Launcher, error) {
	launcher := &Launcher{
		pipelineProvider: pipelineProvider,
		sources:          sources,
		tailers:          make(map[string]*Tailer),
		registry:         registry,
		stop:             make(chan struct{}),
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

// run checks periodically which docker containers are running until stop
func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources.GetSourceStreamForType(config.DockerType):
			l.launch(source)
		case <-l.stop:
			// no docker container should be tailed anymore
			return
		}
	}
}

// launch launches a new tailer for the source.
func (l *Launcher) launch(source *config.LogSource) {
	// containerID := source.EntityID
	containerID := ""
	if _, isTailed := l.tailers[containerID]; isTailed {
		return
	}
	container, err := GetContainer(l.cli, containerID)
	if err != nil {
		log.Warnf("Could not inspect container %v: %v", containerID, err)
		return
	}
	if !NewContainer(container).IsMatch(source) {
		return
	}
	l.startTailer(source, container)
}

// startTailer starts a new tailer for the source and the container.
func (l *Launcher) startTailer(source *config.LogSource, container types.Container) {
	log.Infof("Detected container %v - %v", container.Image, ShortContainerID(container.ID))
	tailer := NewTailer(l.cli, container.ID, source, l.pipelineProvider.NextPipelineChan())

	since, err := Since(l.registry, tailer.Identifier(), true)
	if err != nil {
		log.Warnf("Could not recover tailing from last committed offset: %v", ShortContainerID(container.ID), err)
	}

	err = tailer.Start(since)
	if err != nil {
		log.Warnf("Could not start tailer: %v", container.ID, err)
	}

	l.tailers[container.ID] = tailer
}

// dismiss stops the tailer corresponding to the log source.
func (l *Launcher) dismiss(source *config.LogSource) {
	// containerID := source.EntityID
	containerID := ""
	tailer, isTailed := l.tailers[containerID]
	if !isTailed {
		return
	}
	go tailer.Stop()
	delete(l.tailers, tailer.ContainerID)
}
