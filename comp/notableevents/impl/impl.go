// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package notableeventsimpl implements the notable events component
package notableeventsimpl

import (
	"context"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	notableevents "github.com/DataDog/datadog-agent/comp/notableevents/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the notable events component
type Requires struct {
	Lc            compdef.Lifecycle
	Config        configcomp.Component
	Log           logcomp.Component
	EventPlatform eventplatform.Component
	Hostname      hostname.Component
}

// Provides defines what this component provides
type Provides struct {
	Comp notableevents.Component
}

type notableEventsComponent struct {
	collector *collector
	submitter *submitter
	eventChan chan eventPayload
}

// NewComponent creates a new notable events component
func NewComponent(reqs Requires) Provides {
	// Check if the component is enabled
	if !reqs.Config.GetBool("notable_events.enabled") {
		log.Debug("Notable events component is disabled")
		return Provides{
			Comp: &notableEventsComponent{},
		}
	}

	forwarder, ok := reqs.EventPlatform.Get()
	if !ok {
		log.Error("Failed to get event platform forwarder")
		return Provides{
			Comp: &notableEventsComponent{},
		}
	}

	// Create the event channel (unbuffered for backpressure)
	eventChan := make(chan eventPayload)

	// Create collector and submitter
	collector, err := newCollector(eventChan)
	if err != nil {
		log.Errorf("Failed to create notable events collector: %v", err)
		return Provides{
			Comp: &notableEventsComponent{},
		}
	}
	submitter := newSubmitter(forwarder, eventChan, reqs.Hostname)

	comp := &notableEventsComponent{
		collector: collector,
		submitter: submitter,
		eventChan: eventChan,
	}

	// Register lifecycle hooks
	reqs.Lc.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			return comp.start(ctx)
		},
		OnStop: func(_ context.Context) error {
			comp.stop()
			return nil
		},
	})

	log.Debug("Notable events component initialized")

	return Provides{
		Comp: comp,
	}
}

// start initiates the collector and submitter
func (c *notableEventsComponent) start(_ context.Context) error {
	// Check if component is actually initialized
	if c.collector == nil || c.submitter == nil {
		log.Debug("Notable events component not fully initialized, skipping start")
		return nil
	}

	log.Debug("Starting notable events component")

	// Start submitter first (consumer)
	c.submitter.start()

	// Start collector (producer)
	if err := c.collector.start(); err != nil {
		log.Errorf("Failed to start notable events collector: %v", err)
		c.submitter.stop()
		return err
	}

	log.Debug("Notable events component started successfully")
	return nil
}

// stop gracefully shuts down the collector and submitter
func (c *notableEventsComponent) stop() {
	// Check if component is actually initialized
	if c.collector == nil || c.submitter == nil {
		log.Debug("Notable events component not fully initialized, skipping stop")
		return
	}

	log.Debug("Stopping notable events component")

	// Stop collector first (producer)
	c.collector.stop()

	// Close the event channel to signal submitter to stop
	close(c.eventChan)

	// Wait for submitter (consumer) to finish draining the channel
	c.submitter.stop()

	log.Debug("Notable events component stopped")
}
