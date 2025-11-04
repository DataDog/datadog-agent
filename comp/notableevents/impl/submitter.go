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

// eventPayload represents a Windows Event Log event to be submitted
// TODO(WINA-1968): TBD format for event payload, finish with intake.
type eventPayload struct {
	Channel   string
	Provider  string
	EventID   uint
	Timestamp time.Time
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

	log.Info("Notable events submitter input channel closed, shutting down")
}

// submitEvent creates a message and submits it to the event platform
func (s *submitter) submitEvent(payload eventPayload) error {
	// Get hostname
	hostnameValue := s.hostname.GetSafe(context.TODO())

	tags := []string{
		fmt.Sprintf("channel:%s", payload.Channel),
		fmt.Sprintf("provider:%s", payload.Provider),
		fmt.Sprintf("event_id:%d", payload.EventID),
		"source:windows_event_log",
	}
	//nolint:misspell
	// TODO(ECT-4182): Add host field to payload when it is supported by the intake
	// the tag doesn't do what we want, but its better than nothing for now
	tags = append(tags, "host:"+hostnameValue)

	// Create Event Management v2 API payload
	timestamp := payload.Timestamp.In(time.UTC).Format("2006-01-02T15:04:05.000000Z")
	eventData := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "event",
			"attributes": map[string]interface{}{
				"title":    fmt.Sprintf("System Error - Event ID %d - %s", payload.EventID, payload.Provider),
				"category": "alert",
				"attributes": map[string]interface{}{
					"status":   "error",
					"priority": "5",
					"custom": map[string]interface{}{
						"channel":  payload.Channel,
						"provider": payload.Provider,
						"event_id": payload.EventID,
						"source":   "windows_event_log",
					},
				},
				"message":   fmt.Sprintf("Windows Event Log detected event %d from %s", payload.EventID, payload.Provider),
				"tags":      tags,
				"timestamp": timestamp,
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	log.Debugf("Submitting notable event: channel=%s, event_id=%d", payload.Channel, payload.EventID)

	// Create message for event platform
	msg := message.NewMessage(jsonData, nil, "", time.Now().UnixNano())

	// Submit to event platform using the eventsv2 event type
	if err := s.eventPlatformForwarder.SendEventPlatformEventBlocking(msg, eventplatform.EventTypeEventManagement); err != nil {
		return fmt.Errorf("failed to send event to platform: %w", err)
	}

	log.Debugf("Successfully submitted notable event: channel=%s, event_id=%d", payload.Channel, payload.EventID)
	return nil
}
