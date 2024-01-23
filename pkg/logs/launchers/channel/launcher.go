// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package channel

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/channel"
)

// Launcher reacts to sources with Config.Type = Channel, by creating a tailer
// reading from that channel.
//
// WARNING: removing a source does not stop the corresponding tailer.
type Launcher struct {
	pipelineProvider pipeline.Provider
	sources          chan *sources.LogSource
	tailers          []*tailer.Tailer
	stop             chan struct{}
}

// NewLauncher returns an initialized Launcher
func NewLauncher() *Launcher {
	return &Launcher{
		stop: make(chan struct{}),
	}
}

// Start starts the launcher.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	l.pipelineProvider = pipelineProvider
	l.sources = sourceProvider.GetAddedForType(config.StringChannelType)
	go l.run()
}

func (l *Launcher) startNewTailer(source *sources.LogSource) {
	outputChan := l.pipelineProvider.NextPipelineChan()
	tailer := tailer.NewTailer(source, source.Config.Channel, outputChan)
	l.tailers = append(l.tailers, tailer)
	tailer.Start()
}

func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources:
			l.startNewTailer(source)
			source.Status.Success()
		case <-l.stop:
			return
		}
	}
}

// Stop waits for any running tailer to be flushed.
func (l *Launcher) Stop() {
	panic("not called")
}
