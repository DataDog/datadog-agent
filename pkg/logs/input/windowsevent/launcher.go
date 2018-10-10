// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package windowsevent

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// Launcher is in charge of starting and stopping windows event logs tailers
type Launcher struct {
	sources          chan *config.LogSource
	pipelineProvider pipeline.Provider
	tailers          map[string]*Tailer
	stop             chan struct{}
}

// NewLauncher returns a new Launcher.
func NewLauncher(sources *config.LogSources, pipelineProvider pipeline.Provider) *Launcher {
	return &Launcher{
		sources:          sources.GetAddedForType(config.WindowsEventType),
		pipelineProvider: pipelineProvider,
		tailers:          make(map[string]*Tailer),
		stop:             make(chan struct{}),
	}
}

// Start starts the launcher.
func (l *Launcher) Start() {
	availableChannels, err := EnumerateChannels()
	if err != nil {
		log.Debug("Could not list windows event log channels: ", err)
	} else {
		log.Debug("Found available windows event log channels: ", availableChannels)
	}
	go l.run()
}

// run starts new tailers.
func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources:
			identifier := Identifier(source.Config.ChannelPath, source.Config.Query)
			if _, exists := l.tailers[identifier]; exists {
				// tailer already setup
				continue
			}
			tailer, err := l.setupTailer(source)
			if err != nil {
				log.Info("Could not set up windows event log tailer: ", err)
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
	stopper := restart.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
		delete(l.tailers, tailer.Identifier())
	}
	stopper.Stop()
}

// sanitizedConfig sets default values for the config
func (l *Launcher) sanitizedConfig(sourceConfig *config.LogsConfig) *Config {
	config := &Config{sourceConfig.ChannelPath, sourceConfig.Query}
	if config.Query == "" {
		config.Query = "*"
	}
	return config
}

// setupTailer configures and starts a new tailer
func (l *Launcher) setupTailer(source *config.LogSource) (*Tailer, error) {
	sanitizedConfig := l.sanitizedConfig(source.Config)
	config := &Config{sanitizedConfig.ChannelPath, sanitizedConfig.Query}
	tailer := NewTailer(source, config, l.pipelineProvider.NextPipelineChan())
	tailer.Start()
	return tailer, nil
}
