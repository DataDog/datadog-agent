// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ssistatusimpl implements the ssistatus component interface
package ssistatusimpl

import (
	"context"
	"time"

	"github.com/fsnotify/fsnotify"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	installerexec "github.com/DataDog/datadog-agent/comp/updater/installerexec/def"
	ssistatus "github.com/DataDog/datadog-agent/comp/updater/ssistatus/def"
)

// Requires defines the dependencies for the ssistatus component
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log            log.Component
	InventoryAgent inventoryagent.Component
	InstallerExec  installerexec.Component
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
		iexec:          reqs.InstallerExec,
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
	iexec          installerexec.Component
	stopCh         chan struct{}
	fswatcher      *fsnotify.Watcher
}

func (c *ssiStatusComponent) Start(_ context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	c.fswatcher = watcher
	c.stopCh = make(chan struct{})
	go func() {
		for {
			select {
			case event, ok := <-c.fswatcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					c.update()
				}
			case err, ok := <-c.fswatcher.Errors:
				if !ok {
					return
				}
				c.log.Errorf("Error watching file: %v", err)
			case <-c.stopCh:
				return
			}
		}
	}()
	c.update() // Initial update to set the initial state
	files := []string{"/etc/ld.so.preload", "/etc/docker/daemon.json", "/opt/datadog-packages/datadog-apm-inject"}
	for _, file := range files {
		err := c.fswatcher.Add(file)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *ssiStatusComponent) update() {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// APM host-based auto injection (SSI)
	autoInstrumentationEnabled, instrumentationModes, err := c.autoInstrumentationStatus(ctx)
	if err != nil {
		c.log.Warnf("could not check APM auto-instrumentation status: %s", err)
	}
	c.inventoryAgent.Set("feature_auto_instrumentation_enabled", autoInstrumentationEnabled)
	c.inventoryAgent.Set("auto_instrumentation_modes", instrumentationModes)
}

func (c *ssiStatusComponent) Stop(_ context.Context) error {
	c.fswatcher.Close()
	close(c.stopCh)
	return nil
}
