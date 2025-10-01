// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl implements the health-platform component interface
package healthplatformimpl

import (
	"context"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

const (
	// tickerInterval is the interval for the health check ticker
	tickerInterval = 15 * time.Second
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
}

// Provides defines the output of the health-platform component
type Provides struct {
	Comp healthplatform.Component
}

// healthPlatformImpl implements the health platform component
type healthPlatformImpl struct {
	log    log.Component
	ticker *time.Ticker
	stopCh chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

// NewComponent creates a new health-platform component
func NewComponent(reqs Requires) (Provides, error) {
	reqs.Log.Info("Creating health platform component")
	ctx, cancel := context.WithCancel(context.Background())

	comp := &healthPlatformImpl{
		log:    reqs.Log,
		ticker: time.NewTicker(tickerInterval),
		stopCh: make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}

	// Register lifecycle hooks
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})

	provides := Provides{Comp: comp}
	return provides, nil
}

// start starts the health platform component
func (h *healthPlatformImpl) start(_ context.Context) error {
	h.log.Info("Starting health platform component")

	go h.runTicker()
	return nil
}

// stop stops the health platform component
func (h *healthPlatformImpl) stop(_ context.Context) error {
	h.log.Info("Stopping health platform component")

	h.cancel()
	h.ticker.Stop()
	close(h.stopCh)

	return nil
}

// runTicker runs the periodic ticker that logs every 15 seconds
func (h *healthPlatformImpl) runTicker() {
	for {
		select {
		case <-h.ticker.C:
			h.log.Info("Health platform ticker - component is running")
		case <-h.stopCh:
			return
		case <-h.ctx.Done():
			return
		}
	}
}

// Run runs the health checks and reports the issues
func (h *healthPlatformImpl) Run(_ context.Context) (*healthplatform.HealthReport, error) {
	// TODO: Implement actual health checks
	return nil, nil
}
