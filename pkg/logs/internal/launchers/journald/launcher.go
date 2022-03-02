// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd
// +build systemd

package journald

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Launcher is in charge of starting and stopping new journald tailers
type Launcher struct {
	sources          chan *config.LogSource
	pipelineProvider pipeline.Provider
	registry         auditor.Registry
	tailers          map[string]*tailer.Tailer
	stop             chan struct{}
}

// NewLauncher returns a new Launcher.
func NewLauncher(sources *config.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry) *Launcher {
	return &Launcher{
		sources:          sources.GetAddedForType(config.JournaldType),
		pipelineProvider: pipelineProvider,
		registry:         registry,
		tailers:          make(map[string]*tailer.Tailer),
		stop:             make(chan struct{}),
	}
}

// Start starts the launcher.
func (l *Launcher) Start() {
	go l.run()
}

// run starts new tailers.
func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources:
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
func (l *Launcher) setupTailer(source *config.LogSource) (*tailer.Tailer, error) {
	tailer := tailer.NewTailer(source, l.pipelineProvider.NextPipelineChan())
	cursor := l.registry.GetOffset(tailer.Identifier())
	err := tailer.Start(cursor)
	if err != nil {
		return nil, err
	}
	return tailer, nil
}
