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

// defaultFrameSize represents the size of the read buffer of the TCP and UDP sockets.
var defaultFrameSize = config.LogsAgent.GetInt("logs_config.frame_size")

// Listener represents an objet that can accept new incomming connections.
type Listener interface {
	Start()
	Stop()
}

// Listeners summons different protocol specific listeners based on configuration
type Listeners struct {
	pipelineProvider pipeline.Provider
	sources          *config.LogSources
	listeners        []Listener
}

// NewListener returns an initialized Listeners
func NewListener(sources *config.LogSources, pipelineProvider pipeline.Provider) *Listeners {
	return &Listeners{
		pipelineProvider: pipelineProvider,
		sources:          sources,
	}
}

// Start starts all listeners
func (l *Listeners) Start() {
	var listeners []Listener
	for _, source := range l.sources.GetValidSourcesWithType(config.TCPType) {
		listeners = append(listeners, NewTCPListener(l.pipelineProvider, source, defaultFrameSize))
	}
	for _, source := range l.sources.GetValidSourcesWithType(config.UDPType) {
		listeners = append(listeners, NewUDPListener(l.pipelineProvider, source, defaultFrameSize))
	}
	for _, listener := range listeners {
		listener.Start()
	}
	l.listeners = listeners
}

// Stop stops all listeners
func (l *Listeners) Stop() {
	stopper := restart.NewParallelStopper()
	for _, l := range l.listeners {
		stopper.Add(l)
	}
	stopper.Stop()
}
