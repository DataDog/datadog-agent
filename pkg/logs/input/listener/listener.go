// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listener

import (
	"log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// A Listener summons different protocol specific listeners based on configuration
type Listener struct {
	pp      pipeline.Provider
	sources []*config.IntegrationConfigLogSource
}

// New returns an initialized Listener
func New(sources []*config.IntegrationConfigLogSource, pp pipeline.Provider) *Listener {
	return &Listener{
		pp:      pp,
		sources: sources,
	}
}

// Start starts the Listener
func (l *Listener) Start() {
	for _, source := range l.sources {
		switch source.Type {
		case config.TCPType:
			tcpl, err := NewTCPListener(l.pp, source)
			if err != nil {
				log.Println("Can't start tcp source:", err)
			} else {
				tcpl.Start()
			}
		case config.UDPType:
			udpl, err := NewUDPListener(l.pp, source)
			if err != nil {
				log.Println("Can't start udp source:", err)
			} else {
				udpl.Start()
			}
		default:
		}
	}
}
