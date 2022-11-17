// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package probe

import (
	"context"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/winprocmon"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// CustomEvent is used to send custom security events to Datadog
// for now, provides definition that's supposed to be in custom_events.go
type CustomEvent struct {
	tags []string
}

type Probe struct {
	config       *config.Config
	statsdClient statsd.ClientInterface

	ctx context.Context
}

func NewProbe(config *config.Config, statsdClient statsd.ClientInterface) (*Probe, error) {
	return &Probe{
		config:       config,
		statsdClient: statsdClient,
	}, nil
}

func (p *Probe) Run() {
	winprocmon.RunLoop(func(evt *winprocmon.WinProcessNotification) {
		p.handleEvent(evt)
	})
}

// handleEvent is essentially a dispatcher for monitoring events
// as of writing, it only supports process events, but should be expanded to
// support multiple types of events via changing `WinProcessNotification` to a generic interface.
func (p *Probe) handleEvent(evt *winprocmon.WinProcessNotification) {
	// From here we call out to process_monitor_windows.go

}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	return nil // p.resolvers.Snapshot()
}

// Setup the runtime security probe
func (p *Probe) Setup() error {
	return nil
}

// Start processing events
func (p *Probe) Start() error {
	return nil
}

// SelectProbes applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (p *Probe) SelectProbes(eventTypes []eval.EventType) error {

	return nil
}

// Close the probe
func (p *Probe) Close() error {
	return nil
}

// OnNewDiscarder is called when a new discarder is found
func (p *Probe) OnNewDiscarder(rs *rules.RuleSet, event *Event, field eval.Field, eventType eval.EventType) error {
	return nil
}
