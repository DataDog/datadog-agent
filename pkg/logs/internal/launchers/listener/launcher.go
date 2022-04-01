// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Launcher summons different protocol specific listeners based on configuration
type Launcher struct {
	pipelineProvider pipeline.Provider
	frameSize        int
	tcpSources       chan *config.LogSource
	udpSources       chan *config.LogSource
	listeners        []startstop.StartStoppable
	stop             chan struct{}
}

// NewLauncher returns an initialized Launcher
func NewLauncher(frameSize int) *Launcher {
	return &Launcher{
		frameSize: frameSize,
		stop:      make(chan struct{}),
	}
}

// Start starts the listener.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	l.pipelineProvider = pipelineProvider
	l.tcpSources = sourceProvider.GetAddedForType(config.TCPType)
	l.udpSources = sourceProvider.GetAddedForType(config.UDPType)
	go l.run()
}

// run starts new network listeners.
func (l *Launcher) run() {
	for {
		select {
		case source := <-l.tcpSources:
			listener := NewTCPListener(l.pipelineProvider, source, l.frameSize)
			listener.Start()
			l.listeners = append(l.listeners, listener)
		case source := <-l.udpSources:
			listener := NewUDPListener(l.pipelineProvider, source, l.frameSize)
			listener.Start()
			l.listeners = append(l.listeners, listener)
		case <-l.stop:
			return
		}
	}
}

// Stop stops all listeners
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := startstop.NewParallelStopper()
	for _, l := range l.listeners {
		stopper.Add(l)
	}
	stopper.Stop()
}
