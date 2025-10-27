// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	evtsubscribe "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

// collector monitors Windows Event Log for notable events
type collector struct {
	// in
	api         evtapi.API
	channelPath string
	query       string
	// out
	outChan chan<- eventPayload
	// internal
	sub    evtsubscribe.PullSubscription
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// newCollector creates a new collector instance
func newCollector(outChan chan<- eventPayload) *collector {
	// TODO(WINA-1970): make real query
	return &collector{
		api:         winevtapi.New(),
		channelPath: "System",
		query:       "*[System[(Level=1 or Level=2 or Level=3)]]",
		outChan:     outChan,
	}
}

// start begins monitoring Windows Event Log
func (c *collector) start() error {
	// Create runtime context for the collector's lifetime
	runtimeCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	// Create subscription object (will be started in the event loop)
	c.sub = evtsubscribe.NewPullSubscription(
		c.channelPath,
		c.query,
		evtsubscribe.WithWindowsEventLogAPI(c.api),
		evtsubscribe.WithStartAtOldestRecord(),
	)

	log.Infof("Initialized Windows Event Log subscription: channel=%s, query=%s", c.channelPath, c.query)

	// Start processing events in background
	c.wg.Add(1)
	go c.run(runtimeCtx)

	return nil
}

// stop signals the collector to stop and waits for it to finish
func (c *collector) stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

// retryForeverWithCancel retries an operation with exponential backoff until it succeeds or context is cancelled
func retryForeverWithCancel(ctx context.Context, operation backoff.Operation) error {
	resetBackoff := backoff.NewExponentialBackOff()
	resetBackoff.InitialInterval = 1 * time.Second
	resetBackoff.MaxInterval = 1 * time.Minute
	// retry never stops if MaxElapsedTime == 0
	resetBackoff.MaxElapsedTime = 0

	return backoff.Retry(operation, backoff.WithContext(resetBackoff, ctx))
}

// run is the main event processing loop
func (c *collector) run(ctx context.Context) {
	defer c.wg.Done()
	defer func() {
		if c.sub != nil {
			c.sub.Stop()
		}
	}()

	// Create render context for extracting system properties
	renderCtx, err := c.api.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		log.Errorf("Failed to create render context: %v", err)
		return
	}
	defer evtapi.EvtCloseRenderContext(c.api, renderCtx)

	// Main event loop - handles subscription retries
	for {
		// Check if loop should exit
		select {
		case <-ctx.Done():
			log.Info("Notable events collector context cancelled, shutting down")
			return
		default:
		}

		// If subscription is not running, try to start it with exponential backoff
		if !c.sub.Running() {
			err := retryForeverWithCancel(ctx, func() error {
				err := c.sub.Start()
				if err != nil {
					log.Warnf("Failed to start event log subscription: %v", err)
					return err
				}
				// Subscription started successfully
				log.Infof("Started Windows Event Log subscription: channel=%s, query=%s", c.channelPath, c.query)
				return nil
			})
			if err != nil {
				// Subscription failed to start, retry returned probably because
				// context was cancelled. Go back to top of loop to check for cancellation.
				continue
			}
		}

		// Subscription is running, wait for events or cancellation
		select {
		case <-ctx.Done():
			log.Info("Notable events collector context cancelled, shutting down")
			return
		case events, ok := <-c.sub.GetEvents():
			if !ok {
				// Events channel is closed, fetch the error and stop the subscription so we may retry
				err := c.sub.Error()
				log.Warnf("GetEvents failed, stopping subscription: %v", err)
				c.sub.Stop()
				// Continue to top of loop to restart subscription
				continue
			}

			// Process each event
			for _, eventRecord := range events {
				if err := c.processEvent(renderCtx, eventRecord); err != nil {
					log.Warnf("Failed to process event: %v", err)
				}
				// Close the event record handle
				evtapi.EvtCloseRecord(c.api, eventRecord.EventRecordHandle)
			}
		}
	}
}

// processEvent extracts event data and sends it to the output channel
func (c *collector) processEvent(renderCtx evtapi.EventRenderContextHandle, eventRecord *evtapi.EventRecord) error {
	// Render event values
	vals, err := c.api.EvtRenderEventValues(renderCtx, eventRecord.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to render event values: %w", err)
	}
	defer vals.Close()

	// Extract Event ID
	eventID, err := vals.UInt(evtapi.EvtSystemEventID)
	if err != nil {
		return fmt.Errorf("failed to get event ID: %w", err)
	}

	log.Debugf("Collected notable event: channel=%s, event_id=%d", c.channelPath, eventID)

	// TODO(WINA-1968): submit event to intake
	payload := eventPayload{
		Channel: c.channelPath,
		EventID: uint(eventID),
	}
	c.outChan <- payload

	return nil
}
