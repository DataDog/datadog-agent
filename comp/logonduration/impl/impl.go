// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package logondurationimpl implements the logon duration component
package logondurationimpl

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	logonduration "github.com/DataDog/datadog-agent/comp/logonduration/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// persistentCacheKey stores the last boot time to detect reboots across agent restarts.
const persistentCacheKey = "logon_duration:last_boot_time"

// Provides defines what this component provides
type Provides struct {
	Comp logonduration.Component
}

// Milestone represents a single event in the boot/logon timeline.
type Milestone struct {
	Name      string  `json:"name"`
	OffsetS   float64 `json:"offset_s"`
	DurationS float64 `json:"duration_s"`
	Timestamp string  `json:"timestamp"`
}

// eventInput holds the data needed to build and send a logon duration event.
type eventInput struct {
	Hostname  string
	Message   string
	Timestamp time.Time
	Custom    map[string]interface{}
}

func buildEventPayload(input eventInput) (map[string]interface{}, error) {
	ts := input.Timestamp.In(time.UTC).Format("2006-01-02T15:04:05.000000Z")

	return map[string]interface{}{
		"data": map[string]interface{}{
			"type": "event",
			"attributes": map[string]interface{}{
				"host":           input.Hostname,
				"title":          "Logon duration",
				"category":       "alert",
				"integration_id": "system-notable-events",
				"system-notable-events": map[string]interface{}{
					"event_type": "Logon duration",
				},
				"attributes": map[string]interface{}{
					"status":   "ok",
					"priority": "3",
					"custom":   input.Custom,
				},
				"message":   input.Message,
				"timestamp": ts,
			},
		},
	}, nil
}

func sendEvent(forwarder eventplatform.Forwarder, input eventInput) error {
	payload, err := buildEventPayload(input)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	log.Debugf("Logon duration event payload: %s", string(jsonData))
	log.Debugf("Submitting logon duration event for host %s", input.Hostname)

	m := message.NewMessage(jsonData, nil, "", time.Now().UnixNano())
	if err := forwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeEventManagement); err != nil {
		return fmt.Errorf("failed to send event to platform: %w", err)
	}

	log.Debugf("Successfully submitted logon duration event")
	return nil
}
