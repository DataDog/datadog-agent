// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// Listener summons different protocol specific listeners based on configuration
type Listener struct {
	pp        pipeline.Provider
	sources   []*config.LogSource
	listeners []restart.Stoppable
}

// New returns an initialized Listener
func New(sources []*config.LogSource, pp pipeline.Provider) *Listener {
	return &Listener{
		pp:        pp,
		sources:   sources,
		listeners: []restart.Stoppable{},
	}
}

// Start starts the Listener
func (l *Listener) Start() {
	for _, source := range l.sources {
		switch source.Config.Type {
		case config.TCPType:
			tcpl, err := NewTCPListener(l.pp, source)
			if err != nil {
				log.Error("Can't start tcp source: ", err)
				continue
			}
			tcpl.Start()
			l.listeners = append(l.listeners, tcpl)
		case config.UDPType:
			udpl, err := NewUDPListener(l.pp, source)
			if err != nil {
				log.Error("Can't start udp source: ", err)
				continue
			}
			udpl.Start()
			l.listeners = append(l.listeners, udpl)
		}
	}
}

// Stop stops all the listeners
func (l *Listener) Stop() {
	stopper := restart.NewParallelStopper(l.listeners...)
	stopper.Stop()
	l.listeners = l.listeners[:0]
}
