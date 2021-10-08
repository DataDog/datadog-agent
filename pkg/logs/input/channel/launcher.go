// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Launcher starts a channel reader on the given channel of string.
type Launcher struct {
	pipelineProvider pipeline.Provider
	sources          chan *config.LogSource
	tailers          []*Tailer
	stop             chan struct{}
}

// NewLauncher returns an initialized Launcher
func NewLauncher(sources *config.LogSources, pipelineProvider pipeline.Provider) *Launcher {
	return &Launcher{
		pipelineProvider: pipelineProvider,
		sources:          sources.GetAddedForType(config.StringChannelType),
		stop:             make(chan struct{}),
	}
}

// Start starts the launcher.
func (l *Launcher) Start() {
	go l.run()
}

func (l *Launcher) startNewTailer(source *config.LogSource) {
	outputChan := l.pipelineProvider.NextPipelineChan()
	tailer := NewTailer(source, source.Config.Channel, outputChan)
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
	for _, tailer := range l.tailers {
		tailer.WaitFlush()
	}
	l.stop <- struct{}{}
}
