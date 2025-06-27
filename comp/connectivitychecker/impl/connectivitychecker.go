// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package connectivitycheckerimpl implements the connectivitychecker component interface
package connectivitycheckerimpl

import (
	"context"
	"time"

	connectivitychecker "github.com/DataDog/datadog-agent/comp/connectivitychecker/def"
	runner "github.com/DataDog/datadog-agent/comp/connectivitychecker/runner"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
)

// Requires defines the dependencies for the connectivitychecker component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
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
	stopCh         chan struct{}
}

// NewComponent creates a new connectivitychecker component
func NewComponent(reqs Requires) (Provides, error) {
	comp := &inventoryImpl{
		log:            reqs.Log,
		config:         reqs.Config,
		inventoryAgent: reqs.InventoryAgent,
		stopCh:         make(chan struct{}),
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.Start, OnStop: comp.Stop})

	provides := Provides{Comp: comp}
	return provides, nil
}

func (c *inventoryImpl) Start(_ context.Context) error {
	c.stopCh = make(chan struct{})
	go func() {
		for {
			select {
			case <-time.After(10 * time.Minute):
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
	c.collect()
	return nil
}

func (c *inventoryImpl) collect() {
	diagnostics, err := runner.Diagnose(c.config, c.log)
	if err != nil {
		c.log.Errorf("Error while running diagnostics: %s", err)
		return
	}

	c.inventoryAgent.Set("diagnostics", diagnostics)

}

func (c *inventoryImpl) Stop(_ context.Context) error {
	close(c.stopCh)
	return nil
}
