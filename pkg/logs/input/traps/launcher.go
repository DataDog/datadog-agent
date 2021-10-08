// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traps

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
)

// Launcher runs a forwarder based on configuration.
type Launcher struct {
	pipelineProvider pipeline.Provider
	sources          chan *config.LogSource
	tailer           *Tailer
	stop             chan interface{}
}

// NewLauncher returns an initialized Launcher
func NewLauncher(sources *config.LogSources, pipelineProvider pipeline.Provider) *Launcher {
	return &Launcher{
		pipelineProvider: pipelineProvider,
		sources:          sources.GetAddedForType(config.SnmpTrapsType),
		stop:             make(chan interface{}, 1),
	}
}

// Start starts the launcher.
func (l *Launcher) Start() {
	go l.run()
}

func (l *Launcher) startNewTailer(source *config.LogSource, inputChan chan *traps.SnmpPacket) {
	outputChan := l.pipelineProvider.NextPipelineChan()
	l.tailer = NewTailer(source, inputChan, outputChan)
	l.tailer.Start()
}

func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources:
			if l.tailer == nil {
				l.startNewTailer(source, traps.GetPacketsChannel())
				source.Status.Success()
			}
		case <-l.stop:
			return
		}
	}
}

// Stop waits for any running tailer to be flushed.
func (l *Launcher) Stop() {
	if l.tailer != nil {
		l.tailer.WaitFlush()
		l.tailer = nil
	}
	l.stop <- true
}
