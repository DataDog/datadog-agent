// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package eventlog

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// Launcher is in charge of starting and stopping windows event logs tailers
type Launcher struct {
	sources          []*config.LogSource
	pipelineProvider pipeline.Provider
	auditor          *auditor.Auditor
	tailers          map[string]*Tailer
}

// New returns a new Launcher.
func New(sources []*config.LogSource, pipelineProvider pipeline.Provider, auditor *auditor.Auditor) *Launcher {
	windowsEventSources := []*config.LogSource{}
	for _, source := range sources {
		if source.Config.Type == config.EventLogType {
			windowsEventSources = append(windowsEventSources, source)
		}
	}
	return &Launcher{
		sources:          windowsEventSources,
		pipelineProvider: pipelineProvider,
		auditor:          auditor,
		tailers:          make(map[string]*Tailer),
	}
}

// Start starts new tailers.
func (l *Launcher) Start() {
	log.Info("Start tailing eventlog")
	availableChannels, err := EnumerateChannels()
	if err != nil {
		log.Debug("Could not list eventlog channels: ", err)
	} else {
		log.Debug("Found available eventlog channels: ", availableChannels)
	}

	for _, source := range l.sources {
		identifier := Identifier(source.Config.ChannelPath, source.Config.Query)
		if _, exists := l.tailers[identifier]; exists {
			// tailer already setup
			continue
		}
		tailer, err := l.setupTailer(source)
		if err != nil {
			log.Info("Could not set up eventlog tailer: ", err)
		} else {
			l.tailers[identifier] = tailer
		}
	}
}

// Stop stops all active tailers
func (l *Launcher) Stop() {
	stopper := restart.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
		delete(l.tailers, tailer.Identifier())
	}
	stopper.Stop()
}

func (l *Launcher) sanitizedConfig(sourceConfig *config.LogsConfig) *Config {
	config := &Config{sourceConfig.ChannelPath, sourceConfig.Query}
	if config.Query == "" {
		config.Query = "*"
	}
	// if config.ChannelPath == "" {
	// 	config.Query = `<QueryList><Query Id="0"><Select Path="*"></Select></Query></QueryList>`
	// }
	return config
}

// setupTailer configures and starts a new tailer
func (l *Launcher) setupTailer(source *config.LogSource) (*Tailer, error) {
	sanitizedConfig := l.sanitizedConfig(source.Config)
	config := &Config{sanitizedConfig.ChannelPath, sanitizedConfig.Query}
	tailer := NewTailer(source, config, l.pipelineProvider.NextPipelineChan())
	tailer.Start(l.auditor.GetLastCommittedOffset(tailer.Identifier()))
	return tailer, nil
}
