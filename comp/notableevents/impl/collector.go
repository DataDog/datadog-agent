// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	evtsubscribe "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

// eventDefinition describes a notable event type and how to collect/process it
type eventDefinition struct {
	// Event identification (for lookup after receiving event)
	Provider string
	EventID  uint

	// Event metadata (for payload)
	EventType string
	Title     string
	Message   string

	// Query definition - inner content of <Query> block
	Channel   string
	QueryBody string

	// Optional formatter for dynamic payload content
	FormatPayload PayloadFormatter
}

// eventKey uniquely identifies an event by provider and event ID
type eventKey struct {
	Provider string
	EventID  uint
}

// bookmarkPersistentCacheKey is the key used to store the bookmark in persistent cache
// Stores in path: run/notable_events/event_log_bookmark
const bookmarkPersistentCacheKey = "notable_events:event_log_bookmark"

// persistentCacheSaver implements evtbookmark.Saver using the Agent's persistent cache.
type persistentCacheSaver struct {
	key string
}

// newPersistentCacheSaver creates a new persistentCacheSaver with the given cache key.
func newPersistentCacheSaver(key string) evtbookmark.Saver {
	return &persistentCacheSaver{key: key}
}

// Save writes the bookmark XML to the persistent cache.
func (s *persistentCacheSaver) Save(bookmarkXML string) error {
	err := persistentcache.Write(s.key, bookmarkXML)
	if err != nil {
		return fmt.Errorf("failed to write bookmark to persistent cache: %w", err)
	}
	return nil
}

// Load reads the bookmark XML from the persistent cache.
func (s *persistentCacheSaver) Load() (string, error) {
	bookmarkXML, err := persistentcache.Read(s.key)
	if err != nil {
		// persistentcache.Read() does not return error if key does not exist,
		// but we'll handle any other errors
		return "", fmt.Errorf("failed to read bookmark from persistent cache: %w", err)
	}
	return bookmarkXML, nil
}

// collector monitors Windows Event Log for notable events
type collector struct {
	// in
	api         evtapi.API
	query       string
	eventLookup map[eventKey]*eventDefinition
	// out
	outChan chan<- eventPayload
	// internal
	sub             evtsubscribe.PullSubscription
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	bookmarkSaver   evtbookmark.Saver
	bookmarkManager evtbookmark.Manager
}

// getEventDefinitions returns the list of notable events to collect
func getEventDefinitions() []eventDefinition {
	e := []eventDefinition{
		{
			Provider:      "Microsoft-Windows-Kernel-Power",
			EventID:       41,
			Channel:       "System",
			QueryBody:     `    <Select Path="System">*[System[Provider[@Name='Microsoft-Windows-Kernel-Power'] and EventID=41]]</Select>`,
			EventType:     "Unexpected reboot",
			Title:         "Unexpected reboot",
			Message:       "The system has rebooted without cleanly shutting down first",
			FormatPayload: formatUnexpectedRebootPayload,
		},
		{
			Provider:      "Application Error",
			EventID:       1000,
			Channel:       "Application",
			QueryBody:     `    <Select Path="Application">*[System[Provider[@Name='Application Error'] and EventID=1000]]</Select>`,
			EventType:     "Application crash",
			Title:         "Application crash",
			Message:       "An application crashed unexpectedly",
			FormatPayload: formatAppCrashPayload,
		},
		{
			Provider:      "Application Hang",
			EventID:       1002,
			Channel:       "Application",
			QueryBody:     `    <Select Path="Application">*[System[Provider[@Name='Application Hang'] and EventID=1002]]</Select>`,
			EventType:     "Application hang",
			Title:         "Application hang",
			Message:       "An application stopped responding and was terminated",
			FormatPayload: formatAppHangPayload,
		},
		{
			Provider:      "Microsoft-Windows-WindowsUpdateClient",
			EventID:       20,
			Channel:       "System",
			QueryBody:     `    <Select Path="System">*[System[Provider[@Name='Microsoft-Windows-WindowsUpdateClient'] and EventID=20]]</Select>`,
			EventType:     "Failed Windows update",
			Title:         "Failed Windows update",
			Message:       "A Windows Update installation failed",
			FormatPayload: formatWindowsUpdateFailedPayload,
		},
		{
			Provider: "MsiInstaller",
			EventID:  1033,
			Channel:  "Application",
			QueryBody: `
	<Select Path="Application">*[System[Provider[@Name='MsiInstaller'] and EventID=1033]]</Select>
    <Suppress Path="Application">*[System[Provider[@Name='MsiInstaller'] and EventID=1033] and EventData/Data[4]='0']</Suppress>`,
			EventType:     "Failed application installation",
			Title:         "Failed application installation",
			Message:       "An application installation (MSI) failed",
			FormatPayload: formatMsiInstaller1033Payload,
		},
		{
			Provider: "MsiInstaller",
			EventID:  1034,
			Channel:  "Application",
			QueryBody: `
	<Select Path="Application">*[System[Provider[@Name='MsiInstaller'] and EventID=1034]]</Select>
    <Suppress Path="Application">*[System[Provider[@Name='MsiInstaller'] and EventID=1034] and EventData/Data[4]='0']</Suppress>`,
			EventType:     "Failed application removal",
			Title:         "Failed application removal",
			Message:       "An application removal (MSI) failed",
			FormatPayload: formatMsiInstaller1034Payload,
		},
	}
	return e
}

// buildEventLookup creates a map for fast event definition lookup.
// Returns error if duplicate event keys are found.
func buildEventLookup(events []eventDefinition) (map[eventKey]*eventDefinition, error) {
	lookup := make(map[eventKey]*eventDefinition)
	for i := range events {
		def := &events[i]
		key := eventKey{Provider: def.Provider, EventID: def.EventID}
		if _, exists := lookup[key]; exists {
			return nil, fmt.Errorf("duplicate event definition: %s/%d", def.Provider, def.EventID)
		}
		lookup[key] = def
	}
	return lookup, nil
}

// buildQuery generates full XML query from event definitions.
// Each event gets its own <Query> block with auto-generated ID.
func buildQuery(events []eventDefinition) string {
	var queries []string
	for i, def := range events {
		query := fmt.Sprintf(`  <Query Id="%d" Path="%s">
%s
  </Query>`, i, def.Channel, def.QueryBody)
		queries = append(queries, query)
	}
	return fmt.Sprintf("<QueryList>\n%s\n</QueryList>", strings.Join(queries, "\n"))
}

// newCollector creates a new collector instance
func newCollector(outChan chan<- eventPayload) (*collector, error) {
	events := getEventDefinitions()
	lookup, err := buildEventLookup(events)
	if err != nil {
		return nil, fmt.Errorf("failed to build event lookup: %w", err)
	}

	api := winevtapi.New()
	bookmarkSaver := newPersistentCacheSaver(bookmarkPersistentCacheKey)
	bookmarkManager := evtbookmark.NewManager(evtbookmark.Config{
		API:               api,
		Saver:             bookmarkSaver,
		BookmarkFrequency: 1,
	})

	return &collector{
		api:             api,
		query:           buildQuery(events),
		outChan:         outChan,
		eventLookup:     lookup,
		bookmarkSaver:   bookmarkSaver,
		bookmarkManager: bookmarkManager,
	}, nil
}

// start begins monitoring Windows Event Log
func (c *collector) start() error {
	// Create runtime context for the collector's lifetime
	runtimeCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	// Create subscription object (will be started in the event loop)
	c.sub = evtsubscribe.NewPullSubscription(
		"", // empty channel path when XML query is used
		c.query,
		evtsubscribe.WithWindowsEventLogAPI(c.api),
		evtsubscribe.WithBookmarkSaver(c.bookmarkSaver),
		evtsubscribe.WithStartMode("now"), // Start from latest event when no bookmark exists
	)
	log.Debugf("Initialized Windows Event Log subscription: query=%s", c.query)

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

	// Close the bookmark manager to release resources
	if c.bookmarkManager != nil {
		c.bookmarkManager.Close()
	}
}

// retryForeverWithCancel retries an operation with exponential backoff until it succeeds or context is cancelled
func retryForeverWithCancel(ctx context.Context, operation func() error) error {
	resetBackoff := backoff.NewExponentialBackOff()
	resetBackoff.InitialInterval = 1 * time.Second
	resetBackoff.MaxInterval = 1 * time.Minute
	// retry never stops if MaxElapsedTime == 0
	_, err := backoff.Retry(ctx, func() (any, error) {
		return nil, operation()
	}, backoff.WithBackOff(resetBackoff), backoff.WithMaxElapsedTime(0))
	return err
}

// run is the main event processing loop
func (c *collector) run(ctx context.Context) {
	defer c.wg.Done()
	defer func() {
		// Save bookmark before stopping subscription
		if c.bookmarkManager != nil {
			if err := c.bookmarkManager.Save(); err != nil {
				log.Warnf("Failed to save bookmark on shutdown: %v", err)
			}
		}
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
			log.Debug("Notable events collector context cancelled, shutting down")
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
				log.Debugf("Started Windows Event Log subscription: query=%s", c.query)
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
			log.Debug("Notable events collector context cancelled, shutting down")
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

	// Extract provider and event ID for lookup
	providerName, err := vals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		return fmt.Errorf("failed to get provider name: %w", err)
	}
	eventID, err := vals.UInt(evtapi.EvtSystemEventID)
	if err != nil {
		return fmt.Errorf("failed to get event ID: %w", err)
	}

	// Lookup event definition
	def, found := c.eventLookup[eventKey{Provider: providerName, EventID: uint(eventID)}]
	if !found {
		return fmt.Errorf("unknown event: %s/%d", providerName, eventID)
	}

	// Render full event XML
	xmlUTF16, err := c.api.EvtRenderEventXml(eventRecord.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to render event XML: %w", err)
	}
	xmlString := windows.UTF16ToString(xmlUTF16)

	// Convert XML to JSON map
	eventMap, err := parseEventXML([]byte(xmlString))
	if err != nil {
		return fmt.Errorf("failed to parse event XML: %w", err)
	}

	// Build custom data with windows_event_log
	customData := map[string]interface{}{
		"windows_event_log": eventMap.Map,
	}

	// Get timestamp
	var timestamp time.Time
	unixTimestamp, err := vals.Time(evtapi.EvtSystemTimeCreated)
	if err != nil {
		// if no timestamp default to current time
		timestamp = time.Now().UTC()
	} else {
		timestamp = time.Unix(unixTimestamp, 0)
	}

	// Build payload with defaults
	payload := eventPayload{
		Timestamp: timestamp,
		EventType: def.EventType,
		Title:     def.Title,
		Message:   def.Message,
		Custom:    customData,
	}

	// Apply custom formatter if defined
	if def.FormatPayload != nil {
		def.FormatPayload(&payload, eventMap.Map)
	}

	log.Debugf("Collected notable event: provider=%s, event_id=%d, title=%s", providerName, eventID, payload.Title)

	c.outChan <- payload

	// Update bookmark to track this event as processed
	// This must happen after successful processing but before the event handle is closed
	if c.bookmarkManager != nil {
		if err := c.bookmarkManager.UpdateAndSave(eventRecord.EventRecordHandle); err != nil {
			log.Warnf("Failed to update bookmark: %v", err)
			// Don't return error - event was successfully processed, bookmark failure is non-fatal
		}
	}

	return nil
}
