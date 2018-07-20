// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"
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
	pp        pipeline.Provider
	sources   []*config.LogSource
	tailers   map[string]*Tailer
	cli       *client.Client
	auditor   *auditor.Auditor
	isRunning bool
	stop      chan struct{}
}

// NewScanner returns an initialized Scanner
func NewScanner(sources []*config.LogSource, pp pipeline.Provider, a *auditor.Auditor) *Scanner {
	containerSources := []*config.LogSource{}
	for _, source := range sources {
		switch source.Config.Type {
		case config.DockerType:
			containerSources = append(containerSources, source)
		default:
		}
	}
	return &Scanner{
		pp:      pp,
		sources: containerSources,
		tailers: make(map[string]*Tailer),
		auditor: a,
		stop:    make(chan struct{}),
	}
}

// Start starts the Scanner
func (s *Scanner) Start() {
	err := s.setup()
	if err != nil {
		s.reportErrorToAllSources(err)
		return
	}
	go s.run()
	s.isRunning = true
}

// Stop stops the Scanner and its tailers in parallel,
// this call returns only when all the tailers are stopped
func (s *Scanner) Stop() {
	if !s.isRunning {
		// the scanner was not start, no need to stop anything
		return
	}
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
		case <-scanTicker.C:
			// check all the containers running on the host and start new tailers if needed
			s.scan(true)
		case <-s.stop:
			// no docker container should be tailed anymore
			return
		}
	}
}

// scan checks for new containers we're expected to
// tail, as well as stopped containers or containers that
// restarted
func (s *Scanner) scan(tailFromBeginning bool) {
	runningContainers := s.listContainers()
	containersToMonitor := make(map[string]bool)

	// monitor new containers, and restart tailers if needed
	for _, container := range runningContainers {
		source := NewContainer(container).findSource(s.sources)
		if source == nil {
			continue
		}
		tailer, isTailed := s.tailers[container.ID]
		if isTailed && tailer.shouldStop {
			continue
		}
		if !isTailed {
			// setup a new tailer
			succeeded := s.setupTailer(s.cli, container, source, tailFromBeginning, s.pp.NextPipelineChan())
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

// Start starts the Scanner
func (s *Scanner) setup() error {
	if len(s.sources) == 0 {
		return fmt.Errorf("No container source defined")
	}

	cli, err := NewClient()
	if err != nil {
		log.Error("Can't tail containers, ", err)
		return fmt.Errorf("Can't initialize client")
	}
	s.cli = cli

	// Initialize docker utils
	err = tagger.Init()
	if err != nil {
		log.Warn(err)
	}

	// Start tailing monitored containers
	s.scan(false)
	return nil
}

// setupTailer sets one tailer, making it tail from the beginning or the end,
// returns true if the setup succeeded, false otherwise
func (s *Scanner) setupTailer(cli *client.Client, container types.Container, source *config.LogSource, tailFromBeginning bool, outputChan chan message.Message) bool {
	log.Info("Detected container ", container.Image, " - ", s.humanReadableContainerID(container.ID))
	t := NewTailer(cli, container.ID, source, outputChan)
	var err error
	if tailFromBeginning {
		err = t.tailFromBeginning()
	} else {
		err = t.recoverTailing(s.auditor)
	}
	if err != nil {
		log.Warn(err)
		return false
	}
	s.tailers[container.ID] = t
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
	for _, source := range s.sources {
		source.Status.Error(err)
	}
}

func (s *Scanner) humanReadableContainerID(containerID string) string {
	return containerID[:12]
}
