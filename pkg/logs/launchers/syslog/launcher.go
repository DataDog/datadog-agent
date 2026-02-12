// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package syslog provides a launcher for syslog listeners (TCP and UDP).
package syslog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Launcher creates syslog listeners (TCP or UDP) based on source configuration.
type Launcher struct {
	pipelineProvider pipeline.Provider
	sources          chan *sources.LogSource
	listeners        []startstop.StartStoppable
	stop             chan struct{}
}

// NewLauncher returns an initialized syslog Launcher.
func NewLauncher() *Launcher {
	return &Launcher{
		stop: make(chan struct{}),
	}
}

// Start starts the launcher and begins listening for syslog sources.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, _ auditor.Registry, _ *tailers.TailerTracker) {
	l.pipelineProvider = pipelineProvider
	l.sources = sourceProvider.GetAddedForType(config.SyslogType)
	go l.run()
}

// run processes incoming syslog sources and starts appropriate listeners.
func (l *Launcher) run() {
	for {
		select {
		case source := <-l.sources:
			protocol := source.Config.Protocol
			if protocol == "" {
				protocol = "tcp"
			}
			switch protocol {
			case "tcp":
				listener := NewTCPListener(l.pipelineProvider, source)
				listener.Start()
				l.listeners = append(l.listeners, listener)
			case "udp":
				listener := NewUDPListener(l.pipelineProvider, source)
				listener.Start()
				l.listeners = append(l.listeners, listener)
			default:
				log.Errorf("Unsupported syslog protocol %q for source on port %d", protocol, source.Config.Port)
				source.Status.Error(fmt.Errorf("unsupported syslog protocol %q", protocol))
			}
		case <-l.stop:
			return
		}
	}
}

// Stop stops all listeners managed by this launcher.
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := startstop.NewParallelStopper()
	for _, listener := range l.listeners {
		stopper.Add(listener)
	}
	stopper.Stop()
}
