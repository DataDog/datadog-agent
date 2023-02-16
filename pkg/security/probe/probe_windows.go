// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package probe

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/time/rate"
)

type PlatformProbe struct {
}

// AddEventHandler set the probe event handler
func (p *Probe) AddEventHandler(eventType model.EventType, handler EventHandler) error {
	return nil
}

// Init initializes the probe
func (p *Probe) Init() error {
	return nil
}

// Setup the runtime security probe
func (p *Probe) Setup() error {
	return nil
}

// Start processing events
func (p *Probe) Start() error {
	return nil
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	return nil
	//return p.resolvers.Snapshot()
}

// Close the probe
func (p *Probe) Close() error {
	return nil
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats() error {
	//p.resolvers.TCResolver.SendTCProgramsStats(p.StatsdClient)
	//
	//return p.monitor.SendStats()
	return nil
}

// GetDebugStats returns the debug stats
func (p *Probe) GetDebugStats() map[string]interface{} {
	debug := map[string]interface{}{
		"start_time": p.startTime.String(),
	}
	// TODO(Will): add manager state
	return debug
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts) (*Probe, error) {
	opts.normalize()

	ctx, cancel := context.WithCancel(context.Background())

	p := &Probe{
		Opts:                 opts,
		Config:               config,
		ctx:                  ctx,
		cancelFnc:            cancel,
		StatsdClient:         opts.StatsdClient,
		discarderRateLimiter: rate.NewLimiter(rate.Every(time.Second/5), 100),
		event:                &model.Event{},
	}
	return p, nil
}
