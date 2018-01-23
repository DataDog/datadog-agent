// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"sync"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Listener represents a component that can open and read data from a connections
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

// New returns an initialized Listener
func New(sources []*config.LogSource, pp pipeline.Provider) *Listeners {
	return &Listeners{
		pp:        pp,
		sources:   sources,
		listeners: []Listener{},
	}
}

// Start starts the Listener
func (l *Listeners) Start() {
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
func (l *Listeners) Stop() {
	wg := &sync.WaitGroup{}
	for _, listener := range l.listeners {
		wg.Add(1)
		go func(l Listener) {
			l.Stop()
			wg.Done()
		}(listener)
	}
	wg.Wait()
	l.listeners = l.listeners[:0]
}
