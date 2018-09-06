// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// Listener represents an objet that can accept new incomming connections.
type Listener interface {
	Start()
	Stop()
}

// Listeners summons different protocol specific listeners based on configuration
type Listeners struct {
	pipelineProvider pipeline.Provider
	frameSize        int
	sources          *config.LogSources
	listeners        []Listener
	stop             chan struct{}
}

// NewListener returns an initialized Listeners
func NewListener(sources *config.LogSources, frameSize int, pipelineProvider pipeline.Provider) *Listeners {
	return &Listeners{
		pipelineProvider: pipelineProvider,
		frameSize:        frameSize,
		sources:          sources,
		stop:             make(chan struct{}),
	}
}

// Start starts the listener.
func (l *Listeners) Start() {
	go l.run()
}

// run starts new network listeners.
func (l *Listeners) run() {
	for {
		select {
		case source := <-l.sources.GetSourceStreamForType(config.TCPType):
			listener := NewTCPListener(l.pipelineProvider, source, l.frameSize)
			listener.Start()
			l.listeners = append(l.listeners, listener)
		case source := <-l.sources.GetSourceStreamForType(config.UDPType):
			listener := NewUDPListener(l.pipelineProvider, source, l.frameSize)
			listener.Start()
			l.listeners = append(l.listeners, listener)
		case <-l.stop:
			return
		}
	}
}

// Stop stops all listeners
func (l *Listeners) Stop() {
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, l := range l.listeners {
		stopper.Add(l)
	}
	stopper.Stop()
}
