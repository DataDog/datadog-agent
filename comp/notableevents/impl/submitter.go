// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// eventPayload represents a notable event to be submitted
type eventPayload struct {
	Timestamp time.Time
	EventType string                 // Category for grouping (e.g., "Unexpected reboot")
	Title     string                 // Short title for display
	Message   string                 // Detailed message
	Custom    map[string]interface{} // Event-specific data (e.g., windows_event_log JSON)
}

// submitter receives event payloads from a channel and forwards them to the event platform
type submitter struct {
	// in
	eventPlatformForwarder eventplatform.Forwarder
	hostname               hostname.Component
	inChan                 <-chan eventPayload
	// internal
	wg sync.WaitGroup
}

// newSubmitter creates a new submitter instance
func newSubmitter(forwarder eventplatform.Forwarder, inChan <-chan eventPayload, hostname hostname.Component) *submitter {
	return &submitter{
		eventPlatformForwarder: forwarder,
		hostname:               hostname,
		inChan:                 inChan,
	}
}

// start begins processing events from the input channel
func (s *submitter) start() {
	s.wg.Add(1)
	go s.run()
}

// stop waits for the submitter to finish draining the channel
func (s *submitter) stop() {
	s.wg.Wait()
}

// run is the main loop that processes events
func (s *submitter) run() {
	defer s.wg.Done()

	for payload := range s.inChan {
		if err := s.submitEvent(payload); err != nil {
			log.Warnf("Failed to submit notable event: %v", err)
		}
	}

	log.Debug("Notable events submitter input channel closed, shutting down")
}

// submitEvent creates a message and submits it to the event platform
func (s *submitter) submitEvent(payload eventPayload) error {
	hostnameValue := s.hostname.GetSafe(context.TODO())
	timestamp := payload.Timestamp.In(time.UTC).Format("2006-01-02T15:04:05.000000Z")

	// Create Event Management v2 API payload
	eventData := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "event",
			"attributes": map[string]interface{}{
				"host":           hostnameValue,
				"title":          payload.Title,
				"category":       "alert",
				"integration_id": "system-notable-events",
				"system-notable-events": map[string]interface{}{
					"event_type": payload.EventType,
				},
				"attributes": map[string]interface{}{
					"status":   "error",
					"priority": "5",
					"custom":   payload.Custom,
				},
				"message":   payload.Message,
				"timestamp": timestamp,
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	log.Debugf("Submitting notable event: title=%s, event_type=%s", payload.Title, payload.EventType)

	// Create message for event platform
	msg := message.NewMessage(jsonData, nil, "", time.Now().UnixNano())

	// Submit to event platform using the eventsv2 event type
	if err := s.eventPlatformForwarder.SendEventPlatformEventBlocking(msg, eventplatform.EventTypeEventManagement); err != nil {
		return fmt.Errorf("failed to send event to platform: %w", err)
	}

	log.Debugf("Successfully submitted notable event: title=%s", payload.Title)
	return nil
}
