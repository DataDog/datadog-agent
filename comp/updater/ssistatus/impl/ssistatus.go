// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ssistatusimpl implements the ssistatus component interface
package ssistatusimpl

import (
	"context"
	"embed"
	"io"
	"runtime"
	"slices"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	ssistatus "github.com/DataDog/datadog-agent/comp/updater/ssistatus/def"
)

// Requires defines the dependencies for the ssistatus component
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log            log.Component
	InventoryAgent inventoryagent.Component
}

// Provides defines the output of the ssistatus component
type Provides struct {
	Comp   ssistatus.Component
	Status status.InformationProvider
}

// NewComponent creates a new ssistatus component
func NewComponent(reqs Requires) (Provides, error) {
	ssiStatus := &ssiStatusComponent{
		inventoryAgent: reqs.InventoryAgent,
		log:            reqs.Log,
	}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: ssiStatus.Start, OnStop: ssiStatus.Stop})

	provides := Provides{
		Comp:   ssiStatus,
		Status: status.NewInformationProvider(ssiStatus),
	}
	return provides, nil
}

//go:embed status_templates
var templatesFS embed.FS

type ssiStatusComponent struct {
	inventoryAgent inventoryagent.Component
	log            log.Component
	stopCh         chan struct{}
}

func (c *ssiStatusComponent) Start(_ context.Context) error {
	c.stopCh = make(chan struct{})
	go func() {
		for {
			select {
			case <-time.After(10 * time.Minute):
				c.update()
			case <-c.stopCh:
				return
			}
		}
	}()
	c.update()
	return nil
}

func (c *ssiStatusComponent) update() {
	// APM host-based auto injection (SSI)
	autoInstrumentationEnabled, instrumentationModes, err := c.autoInstrumentationStatus()
	if err != nil {
		c.log.Warnf("could not check APM auto-instrumentation status: %s", err)
	}
	c.inventoryAgent.Set("feature_auto_instrumentation_enabled", autoInstrumentationEnabled)
	c.inventoryAgent.Set("auto_instrumentation_modes", instrumentationModes)
}

func (c *ssiStatusComponent) Stop(_ context.Context) error {
	close(c.stopCh)
	return nil
}

// Name renders the name
func (c *ssiStatusComponent) Name() string {
	return "SSI"
}

// Section renders the section
func (c *ssiStatusComponent) Section() string {
	return "SSI"
}

// JSON renders the JSON
func (c *ssiStatusComponent) JSON(_ bool, stats map[string]interface{}) error {
	c.populateStatus(stats)
	return nil
}

// Text renders the text output
func (c *ssiStatusComponent) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "ssistatus.tmpl", buffer, c.getStatusInfo())
}

// HTML renders the html output
func (c *ssiStatusComponent) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "ssistatusHTML.tmpl", buffer, c.getStatusInfo())
}

func (c *ssiStatusComponent) populateStatus(stats map[string]interface{}) {
	autoInstrumentationEnabled, instrumentationModes, err := c.autoInstrumentationStatus()
	ssiStatus := make(map[string]interface{})
	if err != nil {
		ssiStatus["status"] = "unavailable"
	} else {
		if autoInstrumentationEnabled {
			ssiStatus["status"] = "enabled"
		} else {
			ssiStatus["status"] = "disabled"
		}
	}
	modes := make(map[string]bool)
	switch os := runtime.GOOS; os {
	case "windows":
	case "linux":
		modes["host"] = slices.Contains(instrumentationModes, "host")
		modes["docker"] = slices.Contains(instrumentationModes, "docker")
	default:
		ssiStatus["status"] = "unsupported"
	}
	ssiStatus["modes"] = modes
	stats["ssiStatus"] = ssiStatus
}

func (c *ssiStatusComponent) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	c.populateStatus(stats)

	return stats
}
