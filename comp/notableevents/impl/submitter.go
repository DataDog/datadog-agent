// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// eventPayload represents a Windows Event Log event to be submitted
type eventPayload struct {
	Channel string
	EventID uint
}

// eventsForwarder is an abstraction that supports both default and event platform forwarders
type eventsForwarder interface {
	submitEvent(jsonData []byte) error
}

// epForwarderAdapter adapts the event platform forwarder to the eventsForwarder interface
type epForwarderAdapter struct {
	forwarder eventplatform.Forwarder
}

// submitEvent sends an event using the event platform forwarder
func (a *epForwarderAdapter) submitEvent(jsonData []byte) error {
	msg := message.NewMessage(jsonData, nil, "", time.Now().UnixNano())
	return a.forwarder.SendEventPlatformEventBlocking(msg, eventplatform.EventTypeEventsV2)
}

// defaultForwarderAdapter adapts the default forwarder to the eventsForwarder interface
type defaultForwarderAdapter struct {
	forwarder defaultforwarder.Forwarder
}

// submitEvent sends an event using the default forwarder
func (a *defaultForwarderAdapter) submitEvent(jsonData []byte) error {
	payload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&jsonData})
	return a.forwarder.SubmitEvents(payload, nil)
}

// submitter receives event payloads from a channel and forwards them using the configured forwarder
type submitter struct {
	forwarder eventsForwarder
	inChan    <-chan eventPayload
	wg        sync.WaitGroup
}

// newSubmitter creates a new submitter instance
func newSubmitter(fwd eventsForwarder, inChan <-chan eventPayload) *submitter {
	return &submitter{
		forwarder: fwd,
		inChan:    inChan,
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
	// Create Event Management v2 API payload
	timestamp := time.Now().In(time.UTC).Format("2006-01-02T15:04:05.000000Z")
	eventData := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "event",
			"attributes": map[string]interface{}{
				"title":    fmt.Sprintf("[TEST] Windows Event - %s Channel - Event ID %d", payload.Channel, payload.EventID),
				"category": "alert",
				"attributes": map[string]interface{}{
					"status":   "ok",
					"priority": "5",
					"custom": map[string]interface{}{
						"channel":  payload.Channel,
						"event_id": payload.EventID,
						"source":   "windows_event_log",
					},
				},
				"message": fmt.Sprintf("[TEST] Windows Event Log detected event %d in %s channel", payload.EventID, payload.Channel),
				"tags": []string{
					fmt.Sprintf("channel:%s", payload.Channel),
					fmt.Sprintf("event_id:%d", payload.EventID),
					"source:windows_event_log",
				},
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

	// Submit using the configured forwarder (either default or event platform)
	if err := s.forwarder.submitEvent(jsonData); err != nil {
		return fmt.Errorf("failed to send event to platform: %w", err)
	}

	log.Debugf("Successfully submitted notable event: channel=%s, event_id=%d", payload.Channel, payload.EventID)
	return nil
}
