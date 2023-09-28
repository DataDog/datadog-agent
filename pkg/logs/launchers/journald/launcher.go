// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package journald

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/journald"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/coreos/go-systemd/sdjournal"
)

// SDJournalFactory is a JournalFactory implementation that produces sdjournal instances
type SDJournalFactory struct{}

func (s *SDJournalFactory) NewJournal() (tailer.Journal, error) {
	return sdjournal.NewJournal()
}

func (s *SDJournalFactory) NewJournalFromPath(path string) (tailer.Journal, error) {
	return sdjournal.NewJournalFromDir(path)
}

// Launcher is in charge of starting and stopping new journald tailers
type Launcher struct {
	sources          chan *sources.LogSource
	pipelineProvider pipeline.Provider
	registry         auditor.Registry
	tailers          map[string]*tailer.Tailer
	stop             chan struct{}
	journalFactory   tailer.JournalFactory
}

// NewLauncher returns a new Launcher.
func NewLauncher() *Launcher {
	return NewLauncherWithFactory(&SDJournalFactory{})
}

// NewLauncherWithFactory returns a new Launcher.
func NewLauncherWithFactory(journalFactory tailer.JournalFactory) *Launcher {
	return &Launcher{
		tailers:        make(map[string]*tailer.Tailer),
		stop:           make(chan struct{}),
		journalFactory: journalFactory,
	}
}

// Start starts the launcher.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	l.sources = sourceProvider.GetAddedForType(config.JournaldType)
	l.pipelineProvider = pipelineProvider
	l.registry = registry
	go l.run()
}

// run starts new tailers.
func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources:
			identifier := tailer.Identifier(source.Config)
			if _, exists := l.tailers[identifier]; exists {
				log.Warn(identifier, " is already tailed. Use config_id to tail the same journal more than once")
				continue
			}
			tailer, err := l.setupTailer(source)
			if err != nil {
				log.Warn("Could not set up journald tailer: ", err)
			} else {
				l.tailers[identifier] = tailer
			}
		case <-l.stop:
			return
		}
	}
}

// Stop stops all active tailers
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := startstop.NewParallelStopper()
	for identifier, tailer := range l.tailers {
		stopper.Add(tailer)
		delete(l.tailers, identifier)
	}
	stopper.Stop()
}

// setupTailer configures and starts a new tailer,
// returns the tailer or an error.
func (l *Launcher) setupTailer(source *sources.LogSource) (*tailer.Tailer, error) {
	var journal tailer.Journal
	var err error

	if source.Config.Path == "" {
		// open the default journal
		journal, err = l.journalFactory.NewJournal()
	} else {
		journal, err = l.journalFactory.NewJournalFromPath(source.Config.Path)
	}
	if err != nil {
		return nil, err
	}

	tailer := tailer.NewTailer(source, l.pipelineProvider.NextPipelineChan(), journal, source.Config.V1Behavior)
	cursor := l.registry.GetOffset(tailer.Identifier())

	err = tailer.Start(cursor)
	if err != nil {
		return nil, err
	}
	return tailer, nil
}
