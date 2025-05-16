// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ssistatusimpl implements the ssistatus component interface
package ssistatusimpl

import (
	"context"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	ssistatus "github.com/DataDog/datadog-agent/comp/updater/ssistatus/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/ssi"
)

// Requires defines the dependencies for the ssistatus component
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log            log.Component
	InventoryAgent inventoryagent.Component
}

// Provides defines the output of the ssistatus component
type Provides struct {
	Comp ssistatus.Component
}

// NewComponent creates a new ssistatus component
func NewComponent(reqs Requires) (Provides, error) {
	ssiStatus := &ssiStatusComponent{
		inventoryAgent: reqs.InventoryAgent,
		log:            reqs.Log,
	}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: ssiStatus.Start, OnStop: ssiStatus.Stop})

	provides := Provides{
		Comp: ssiStatus,
	}
	return provides, nil
}

type ssiStatusComponent struct {
	inventoryAgent inventoryagent.Component
	log            log.Component
	stopCh         chan struct{}
}

func (c *ssiStatusComponent) Start(_ context.Context) error {
	c.stopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx := context.Background()
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				// APM host-based auto injection (SSI)
				autoInstrumentationEnabled, err := ssi.IsAutoInstrumentationEnabled(ctx)
				if err != nil {
					c.log.Warnf("could not check if APM auto-instrumentation is enabled: %s", err)
				}
				c.inventoryAgent.Set("feature_auto_instrumentation_enabled", autoInstrumentationEnabled)

				// Modes of instrumentation
				// TODO: add Windows instrumentation (IIS)
				instrumentationStatus, err := ssi.GetInstrumentationStatus()
				if err != nil {
					c.log.Warnf("could not get APM auto-instrumentation status: %s", err)
				}
				instrumentationModes := []string{}
				if instrumentationStatus.HostInstrumented {
					instrumentationModes = append(instrumentationModes, "host")
				}
				if instrumentationStatus.DockerInstalled && instrumentationStatus.DockerInstrumented {
					instrumentationModes = append(instrumentationModes, "docker")
				}
				c.inventoryAgent.Set("auto_instrumentation_modes", instrumentationModes)
			case <-c.stopCh:
				return
			}
		}
	}()
	return nil
}

func (c *ssiStatusComponent) Stop(_ context.Context) error {
	close(c.stopCh)
	return nil
}
