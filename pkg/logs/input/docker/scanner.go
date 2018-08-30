// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

const scanPeriod = 10 * time.Second

// A Scanner listens for stdout and stderr of containers
type Scanner struct {
	pipelineProvider pipeline.Provider
	sources          *config.LogSources
	activeSources    []*config.LogSource
	tailers          map[string]*Tailer
	cli              *client.Client
	registry         auditor.Registry
	stop             chan struct{}
}

// NewScanner returns an initialized Scanner
func NewScanner(sources *config.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry) (*Scanner, error) {
	scanner := &Scanner{
		pipelineProvider: pipelineProvider,
		sources:          sources,
		tailers:          make(map[string]*Tailer),
		registry:         registry,
		stop:             make(chan struct{}),
	}
	err := scanner.setup()
	if err != nil {
		scanner.reportErrorToAllSources(err)
		return nil, err
	}
	return scanner, nil
}

// setup initializes the docker client and the tagger,
// returns an error if it failed.
func (s *Scanner) setup() error {
	var err error
	// create a new docker client
	s.cli, err = NewClient()
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

// Start starts the Scanner
func (s *Scanner) Start() {
	go s.run()
}

// Stop stops the Scanner and its tailers in parallel,
// this call returns only when all the tailers are stopped
func (s *Scanner) Stop() {
	s.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, tailer := range s.tailers {
		stopper.Add(tailer)
		delete(s.tailers, tailer.ContainerID)
	}
	stopper.Stop()
}

// run checks periodically which docker containers are running until stop
func (s *Scanner) run() {
	scanTicker := time.NewTicker(scanPeriod)
	defer scanTicker.Stop()
	for {
		select {
		case source := <-s.sources.GetSourceStreamForType(config.DockerType):
			s.activeSources = append(s.activeSources, source)
			s.launchTailers(source)
		case <-scanTicker.C:
			// check all the containers running on the host and start new tailers if needed
			s.scan()
		case <-s.stop:
			// no docker container should be tailed anymore
			return
		}
	}
}

// scan checks for new containers we're expected to
// tail, as well as stopped containers or containers that
// restarted
func (s *Scanner) scan() {
	runningContainers := s.listContainers()
	containersToMonitor := make(map[string]bool)

	// monitor new containers, and restart tailers if needed
	for _, container := range runningContainers {
		tailer, isTailed := s.tailers[container.ID]
		if isTailed && tailer.shouldStop {
			continue
		}
		if !isTailed {
			// search the best source matching the container
			source := NewContainer(container).FindSource(s.activeSources)
			if source == nil {
				continue
			}
			// setup a new tailer
			succeeded := s.setupTailer(s.cli, container, source, true, s.pipelineProvider.NextPipelineChan())
			if !succeeded {
				// the setup failed, let's try to tail this container in the next scan
				continue
			}
		}
		containersToMonitor[container.ID] = true
	}

	// stop old containers
	for containerID, tailer := range s.tailers {
		_, shouldMonitor := containersToMonitor[containerID]
		if !shouldMonitor {
			s.dismissTailer(tailer)
		}
	}
}

// launch launches new tailers for a new source.
func (s *Scanner) launchTailers(source *config.LogSource) {
	for _, container := range s.listContainers() {
		if _, isTailed := s.tailers[container.ID]; isTailed {
			continue
		}
		c := NewContainer(container)
		if c.GetLabel() != "" {
			continue
		}
		if NewContainer(container).IsMatch(source) {
			s.setupTailer(s.cli, container, source, false, s.pipelineProvider.NextPipelineChan())
		}
	}
}

// setupTailer sets one tailer, making it tail from the beginning or the end,
// returns true if the setup succeeded, false otherwise
func (s *Scanner) setupTailer(cli *client.Client, container types.Container, source *config.LogSource, tailFromBeginning bool, outputChan chan message.Message) bool {
	log.Info("Detected container ", container.Image, " - ", ShortContainerID(container.ID))
	tailer := NewTailer(cli, container.ID, source, outputChan)

	since, err := Since(s.registry, tailer.Identifier(), tailFromBeginning)
	if err != nil {
		log.Warnf("Could not recover last committed offset for container %v: %v", ShortContainerID(container.ID), err)
	}

	err = tailer.Start(since)
	if err != nil {
		log.Warn(err)
		return false
	}

	s.tailers[container.ID] = tailer
	return true
}

// dismissTailer stops the tailer and removes it from the list of active tailers
func (s *Scanner) dismissTailer(tailer *Tailer) {
	// stop the tailer in another routine as we don't want to block here
	go tailer.Stop()
	delete(s.tailers, tailer.ContainerID)
}

func (s *Scanner) listContainers() []types.Container {
	containers, err := s.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Error("Can't tail containers, ", err)
		log.Error("Is datadog-agent part of docker user group?")
		s.reportErrorToAllSources(err)
		return []types.Container{}
	}
	return containers
}

// reportErrorToAllSources changes the status of all sources to Error with err
func (s *Scanner) reportErrorToAllSources(err error) {
	for _, source := range s.activeSources {
		source.Status.Error(err)
	}
}
