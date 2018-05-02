// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package journald

import (
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// Launcher is in charge of starting and stopping new journald tailers
type Launcher struct {
	sources          []*config.LogSource
	pipelineProvider pipeline.Provider
	auditor          *auditor.Auditor
	tailers          map[string]*Tailer
	tailErrors       chan TailError
}

// New returns a new Launcher.
func New(sources []*config.LogSource, pipelineProvider pipeline.Provider, auditor *auditor.Auditor) *Launcher {
	journaldSources := []*config.LogSource{}
	for _, source := range sources {
		if source.Config.Type == config.JournaldType {
			journaldSources = append(journaldSources, source)
		}
	}
	return &Launcher{
		sources:          journaldSources,
		pipelineProvider: pipelineProvider,
		auditor:          auditor,
		tailers:          make(map[string]*Tailer),
		tailErrors:       make(chan TailError),
	}
}

// Start starts new tailers.
func (l *Launcher) Start() {
	for _, source := range l.sources {
		identifier := source.Config.Path
		if _, exists := l.tailers[identifier]; exists {
			// set up only one tailer per journal
			continue
		}
		tailer, err := l.setupTailer(source)
		if err != nil {
			log.Warn("Could not set up journald tailer: ", err)
		} else {
			l.tailers[identifier] = tailer
		}
	}
	go l.run()
}

// Stop stops all active tailers
func (l *Launcher) Stop() {
	stopper := restart.NewParallelStopper()
	for identifier, tailer := range l.tailers {
		stopper.Add(tailer)
		delete(l.tailers, identifier)
	}
	stopper.Stop()
	close(l.tailErrors)
}

// run keeps all tailers alive, restarting them when a fatal error occurs.
func (l *Launcher) run() {
	for tailError := range l.tailErrors {
		log.Error(tailError.err)
		if tailer, exists := l.tailers[tailError.journalID]; exists {
			// safely stop and restart the tailer from its last committed cursor
			tailer.Stop()
			err := tailer.Start(l.auditor.GetLastCommittedCursor(tailer.Identifier()))
			if err != nil {
				log.Warn("Could not restart journald tailer: ", err)
			}
		}
	}
}

// setupTailer configures and starts a new tailer,
// returns the tailer or an error.
func (l *Launcher) setupTailer(source *config.LogSource) (*Tailer, error) {
	var includeUnits []string
	var excludeUnits []string
	if source.Config.IncludeUnits != "" {
		includeUnits = strings.Split(source.Config.IncludeUnits, ",")
	}
	if source.Config.ExcludeUnits != "" {
		excludeUnits = strings.Split(source.Config.ExcludeUnits, ",")
	}
	config := JournalConfig{
		IncludeUnits: includeUnits,
		ExcludeUnits: excludeUnits,
		Path:         source.Config.Path,
	}
	tailer := NewTailer(config, source, l.pipelineProvider.NextPipelineChan(), l.tailErrors)
	err := tailer.Start(l.auditor.GetLastCommittedCursor(tailer.Identifier()))
	if err != nil {
		return nil, err
	}
	return tailer, nil
}
