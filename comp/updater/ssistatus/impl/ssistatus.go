// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ssistatusimpl implements the ssistatus component interface
package ssistatusimpl

import (
	"context"
	"os"
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
	newFileCh      chan struct{} // Channel to trigger updates when a new file is added
	fswatcher      *fsnotify.Watcher
}

func (c *ssiStatusComponent) Start(_ context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	c.fswatcher = watcher
	c.stopCh = make(chan struct{})
	c.newFileCh = make(chan struct{}) // Buffered channel to avoid blocking

	// Only update the status if a SSI-related file is modified / created
	go func() {
		for {
			select {
			case event, ok := <-c.fswatcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					c.update()
				}
			case err, ok := <-c.fswatcher.Errors:
				if !ok {
					return
				}
				c.log.Errorf("Error watching file: %v", err)
			case <-c.newFileCh:
				c.update()
			case <-c.stopCh:
				return
			}
		}
	}()

	// Periodically try to watch the files, as they may not exist at the time of the initial start
	go func() {
		watched := make(map[string]bool)
		for {
			select {
			case <-time.After(time.Minute):
				for _, file := range watchedFiles {
					_, err := os.Stat(file)
					if err == nil && !watched[file] {
						if err := c.fswatcher.Add(file); err != nil {
							c.log.Errorf("Error watching file %s: %v", file, err)
						} else {
							watched[file] = true
							c.log.Debugf("Started watching file: %s", file)
							c.newFileCh <- struct{}{} // Trigger an update when a new file is added
						}
					} else if os.IsNotExist(err) && watched[file] {
						if err := c.fswatcher.Remove(file); err != nil {
							c.log.Errorf("Error removing file %s from watcher: %v", file, err)
						} else {
							delete(watched, file)
							c.log.Debugf("Stopped watching file: %s", file)
						}
					}
				}
			case <-c.stopCh:
				return
			}
		}
	}()

	c.update() // Initial update to set the initial state
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
