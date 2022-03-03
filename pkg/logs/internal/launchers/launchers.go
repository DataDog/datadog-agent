// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchers

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Launchers manages a collection of launchers.
type Launchers struct {
	// sourceProvider is the SourceProvider that will be given to launchers' Start method.
	sourceProvider SourceProvider

	// pipelineProvider will be given to launchers' Start method.
	pipelineProvider pipeline.Provider

	// registry will be given to launchers' Start method.
	registry auditor.Registry

	// launchers is the set of running launchers
	launchers []Launcher

	// started is true after Start
	started bool
}

// NewLaunchers creates a new, empty Launchers instance
func NewLaunchers(
	sources *logsConfig.LogSources,
	pipelineProvider pipeline.Provider,
	registry auditor.Registry,
) *Launchers {
	return &Launchers{
		sourceProvider:   sources,
		pipelineProvider: pipelineProvider,
		registry:         registry,
	}
}

// AddLauncher adds a launcher to the collection.  If called after Start(), then the
// launcher will be started immediately.
func (ls *Launchers) AddLauncher(launcher Launcher) {
	ls.launchers = append(ls.launchers, launcher)
	if ls.started {
		launcher.Start(ls.sourceProvider, ls.pipelineProvider, ls.registry)
	}
}

// Start starts all launchers in the collection.
func (ls *Launchers) Start() {
	for _, s := range ls.launchers {
		s.Start(ls.sourceProvider, ls.pipelineProvider, ls.registry)
	}
	ls.started = true
}

// Stop all launchers and wait until they are complete.
func (ls *Launchers) Stop() {
	var wg sync.WaitGroup
	for _, s := range ls.launchers {
		wg.Add(1)
		go func(s Launcher) {
			defer wg.Done()
			s.Stop()
		}(s)
	}
	wg.Wait()
}
