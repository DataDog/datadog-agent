// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Event is a payload type for events
type Event struct {
	Source string
	*event.Event
	collectedTime time.Time
}

func (p *Event) name() string {
	return p.Source
}

// GetTags returns the tags from a payload
func (p *Event) GetTags() []string {
	return p.Tags
}

// GetCollectedTime returns the time at which the event was received by the fake intake
func (p *Event) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseEventPayload parses a payload into a list of EventPayload
func ParseEventPayload(payload api.Payload) ([]*Event, error) {
	if len(payload.Data) == 0 {
		return nil, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("could not inflate payload: %w", err)
	}

	var data struct {
		APIKey           string                  `json:"apiKey"`
		Events           map[string]event.Events `json:"events"`
		InternalHostName string                  `json:"internalHostname"`
	}

	if err := json.Unmarshal(inflated, &data); err != nil {
		return nil, fmt.Errorf("could not unmarshal payload: %w", err)
	}

	payloads := make([]*Event, 0, lo.SumBy(lo.Values(data.Events), func(item event.Events) int {
		return len(item)
	}))
	for source, events := range data.Events {
		for _, event := range events {
			payloads = append(payloads, &Event{
				Source:        source,
				Event:         event,
				collectedTime: payload.Timestamp,
			})
		}
	}

	return payloads, nil
}

// EventAggregator struct {
type EventAggregator struct {
	Aggregator[*Event]
}

// NewEventAggregator returns a new EventAggregator
func NewEventAggregator() EventAggregator {
	return EventAggregator{
		Aggregator: newAggregator(ParseEventPayload),
	}
}
