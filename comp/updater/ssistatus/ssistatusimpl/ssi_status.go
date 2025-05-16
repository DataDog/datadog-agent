// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssistatusimpl implements the ssistatus component.
package ssistatusimpl

import (
	"context"
	"time"

	"go.uber.org/fx"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/updater/ssistatus"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/ssi"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is the fx module for the updater.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newSSIStatusComponent),
	)
}

// dependencies contains the dependencies to build the updater.
type dependencies struct {
	fx.In

	Log            log.Component
	InventoryAgent inventoryagent.Component
}

func newSSIStatusComponent(lc fx.Lifecycle, dependencies dependencies) (ssistatus.Component, error) {
	ssiStatus := &ssiStatusComponent{
		inventoryAgent: dependencies.InventoryAgent,
		log:            dependencies.Log,
	}
	lc.Append(fx.Hook{OnStart: ssiStatus.Start, OnStop: ssiStatus.Stop})
	return ssiStatus, nil
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
				// APM host-based auto injection (SSI)
				autoInstrumentationEnabled, err := ssi.IsAutoInstrumentationEnabled()
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
