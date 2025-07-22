// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivitycheckerimpl implements the connectivitychecker component interface
package connectivitycheckerimpl

import (
	"context"
	"time"

	checker "github.com/DataDog/datadog-agent/comp/connectivitychecker/checker"
	connectivitychecker "github.com/DataDog/datadog-agent/comp/connectivitychecker/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
)

const (
	initialDelay = 30 * time.Second
	interval     = 10 * time.Minute
)

// Requires defines the dependencies for the connectivitychecker component
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log            log.Component
	Config         config.Component
	InventoryAgent inventoryagent.Component
}

// Provides defines the output of the connectivitychecker component
type Provides struct {
	Comp connectivitychecker.Component
}

type inventoryImpl struct {
	log            log.Component
	config         config.Component
	inventoryAgent inventoryagent.Component
	timerStopCh    chan struct{}
	collectCtx     context.Context
	collectCancel  context.CancelFunc
}

// NewComponent creates a new connectivitychecker component
func NewComponent(reqs Requires) (Provides, error) {
	collectCtx, collectCancel := context.WithCancel(context.Background())
	comp := &inventoryImpl{
		log:            reqs.Log,
		config:         reqs.Config,
		inventoryAgent: reqs.InventoryAgent,
		timerStopCh:    make(chan struct{}),
		collectCtx:     collectCtx,
		collectCancel:  collectCancel,
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.start, OnStop: comp.stop})
	reqs.Config.OnUpdate(func(_ string, _, _ any, _ uint64) { comp.restartTimer() })

	provides := Provides{Comp: comp}
	return provides, nil
}

func (c *inventoryImpl) startTimer(delay time.Duration) {
	go func() {
		// Initial delay before first run
		select {
		case <-time.After(delay):
		case <-c.timerStopCh:
			return
		}

		// Run initial check after delay
		c.collect()

		// Periodic execution
		for {
			select {
			case <-time.After(interval):
				c.collect()
			case <-c.timerStopCh:
				return
			}
		}
	}()
}

// restartTimer restarts the timer process (called on config updates)
func (c *inventoryImpl) restartTimer() {
	c.log.Debug("Connectivity check restarted due to config update")
	// Safely close the timer channel if it's not already closed
	select {
	case <-c.timerStopCh:
		// Channel is already closed, do nothing
	default:
		_ = c.stop(context.Background())
	}

	c.timerStopCh = make(chan struct{})
	// Create new context for the restarted timer
	c.collectCtx, c.collectCancel = context.WithCancel(context.Background())
	c.startTimer(0)
}

func (c *inventoryImpl) collect() {
	diagnoses, err := checker.Check(c.collectCtx, c.config, c.log)
	if err != nil {
		// Check if the error is due to context cancellation
		if c.collectCtx.Err() == context.Canceled {
			c.log.Debug("Connectivity check cancelled")
			return
		}
		c.log.Errorf("Connectivity check failed: %v", err)
		return
	}

	// Check if we should stop before setting data
	select {
	case <-c.collectCtx.Done():
		return
	default:
		// Continue with setting data
	}

	// Send results to inventory agent
	c.inventoryAgent.Set("diagnostics", diagnoses)
	c.log.Debug("Connectivity check completed successfully")
}

func (c *inventoryImpl) start(_ context.Context) error {
	c.startTimer(initialDelay)
	return nil
}

func (c *inventoryImpl) stop(_ context.Context) error {
	// Cancel any ongoing collect operations
	c.collectCancel()
	close(c.timerStopCh)
	return nil
}
