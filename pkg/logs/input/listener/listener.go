// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// A Listener summons different protocol specific listeners based on configuration
type Listener struct {
	pp           pipeline.Provider
	sources      []*config.IntegrationConfigLogSource
	tcpListeners []*TCPListener
	udpListeners []*UDPListener
}

// New returns an initialized Listener
func New(sources []*config.IntegrationConfigLogSource, pp pipeline.Provider) *Listener {
	return &Listener{
		pp:           pp,
		sources:      sources,
		tcpListeners: []*TCPListener{},
		udpListeners: []*UDPListener{},
	}
}

// Start starts the Listener
func (l *Listener) Start() {
	for _, source := range l.sources {
		switch source.Type {
		case config.TCPType:
			tcpl, err := NewTCPListener(l.pp, source)
			if err != nil {
				log.Error("Can't start tcp source: ", err)
				continue
			}
			tcpl.Start()
			l.tcpListeners = append(l.tcpListeners, tcpl)
		case config.UDPType:
			udpl, err := NewUDPListener(l.pp, source)
			if err != nil {
				log.Error("Can't start udp source: ", err)
				continue
			}
			udpl.Start()
			l.udpListeners = append(l.udpListeners, udpl)
		}
	}
}

// Stop closes all the open connections
func (l *Listener) Stop() {
	// stop all tcp connections
	for _, tcpl := range l.tcpListeners {
		tcpl.Stop()
	}
	l.tcpListeners = l.tcpListeners[:0]

	// stop all udp connections
	for _, udpl := range l.udpListeners {
		udpl.Stop()
	}
	l.udpListeners = l.udpListeners[:0]
}
