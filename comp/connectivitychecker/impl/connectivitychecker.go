// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivitycheckerimpl implements the connectivitychecker component interface
package connectivitycheckerimpl

import (
	"context"
	"sync"
	"time"

	checker "github.com/DataDog/datadog-agent/comp/connectivitychecker/checker"
	connectivitychecker "github.com/DataDog/datadog-agent/comp/connectivitychecker/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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

	// mu guards the fields below, which are read from the background timer
	// goroutine and rewritten by restartTimer/stop on config updates.
	mu            sync.Mutex
	timerStopCh   chan struct{}
	collectCtx    context.Context
	collectCancel context.CancelFunc
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
	reqs.Config.OnUpdate(func(_ string, _ model.Source, _, _ any, _ uint64, _ model.Source) { comp.restartTimer() })

	provides := Provides{Comp: comp}
	return provides, nil
}

func (c *inventoryImpl) startTimer(delay time.Duration) {
	if !c.config.GetBool("inventories_diagnostics_enabled") {
		c.log.Debug("Connectivity check disabled: inventories_diagnostics_enabled is false")
		return
	}

	// Capture the channel/context this goroutine should use for its entire
	// lifetime, so a later restartTimer() reassigning c's fields can't race
	// with the reads below.
	c.mu.Lock()
	stopCh := c.timerStopCh
	ctx := c.collectCtx
	c.mu.Unlock()

	go func() {
		// Initial delay before first run
		select {
		case <-time.After(delay):
		case <-stopCh:
			return
		}

		// Run initial check after delay
		c.collect(ctx)

		// Periodic execution
		for {
			select {
			case <-time.After(interval):
				c.collect(ctx)
			case <-stopCh:
				return
			}
		}
	}()
}

// restartTimer restarts the timer process (called on config updates)
func (c *inventoryImpl) restartTimer() {
	c.log.Debug("Connectivity check restarted due to config update")
	_ = c.stop(context.Background())

	c.mu.Lock()
	c.timerStopCh = make(chan struct{})
	// Create new context for the restarted timer
	c.collectCtx, c.collectCancel = context.WithCancel(context.Background())
	c.mu.Unlock()

	c.startTimer(0)
}

func (c *inventoryImpl) collect(ctx context.Context) {
	diagnoses, err := checker.Check(ctx, c.config, c.log)
	if err != nil {
		// Check if the error is due to context cancellation
		if ctx.Err() == context.Canceled {
			c.log.Debug("Connectivity check cancelled")
			return
		}
		c.log.Errorf("Connectivity check failed: %v", err)
		return
	}

	// Check if we should stop before setting data
	select {
	case <-ctx.Done():
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
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel any ongoing collect operations
	c.collectCancel()

	// Safely close the timer channel if it's not already closed
	select {
	case <-c.timerStopCh:
		// Channel is already closed, do nothing
	default:
		close(c.timerStopCh)
	}

	return nil
}
