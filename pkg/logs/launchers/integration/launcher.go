// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integration

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

type Launcher struct {
	sources         chan *sources.LogSource
	piplineProvider pipeline.Provider
	registry        auditor.Registry
	addedSources    chan *sources.LogSource
	removedSources  chan *sources.LogSource
	stop            chan struct{}
	runPath         string
}

// NewLauncher returns a new launcher
func NewLauncher(runPath string) *Launcher {
	return &Launcher{
		runPath: runPath,
	}
}

// Start starts the launcher
func (s *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	s.piplineProvider = pipelineProvider
	s.addedSources, s.removedSources = sourceProvider.SubscribeForType(config.IntegrationType)
	s.registry = registry

	go s.run()
}

// Stop stops the scanner tailers
func (s *Launcher) Stop() {
	s.stop <- struct{}{}
	return
}

// run checks if there are new files to tail and tails them
func (s *Launcher) run() {
	scanTicker := time.NewTicker(time.Second * 1)
	// Add some functionality in here to detect when the agent is sent a log??
	// Maybe call addSource whenever log is sent?

	for {
		select {
		case source := <-s.addedSources:
			s.createFile(source)
		case <-s.stop:
			return
		case <-scanTicker.C:

		}
	}
}

// createFile creates a file for the logsource
func (s *Launcher) createFile(source *sources.LogSource) {
	name := []string{s.runPath, source.Config.Service, source.Config.Name, source.Config.Source}

	file, err := os.Create(strings.Join(name, "/"))
	defer file.Close()
	if err != nil {
		log.Fatal(err)
	}
}
