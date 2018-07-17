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
	pp        pipeline.Provider
	sources   []*config.LogSource
	listeners []Listener
}

// New returns an initialized Listeners
func New(sources []*config.LogSource, pp pipeline.Provider) *Listeners {
	listeners := []Listener{}
	for _, source := range sources {
		switch source.Config.Type {
		case config.TCPType:
			listeners = append(listeners, NewTCPListener(pp, source, defaultFrameSize))
		case config.UDPType:
			listeners = append(listeners, NewUDPListener(pp, source, defaultFrameSize))
		}
	}
	return &Listeners{
		pp:        pp,
		sources:   sources,
		listeners: listeners,
	}
}

// Start starts all listeners
func (l *Listeners) Start() {
	for _, l := range l.listeners {
		l.Start()
	}
}

// Stop stops all listeners
func (l *Listeners) Stop() {
	stopper := restart.NewParallelStopper()
	for _, l := range l.listeners {
		stopper.Add(l)
	}
	stopper.Stop()
}
