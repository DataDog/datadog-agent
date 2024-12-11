// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package eventdatafilter provides functions to filter events based on event data.
package eventdatafilter

import (
	"fmt"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// EventData is an interface that returns event data.
type EventData interface {
	SystemValues() evtapi.EvtVariantValues
}

// Filter is an interface for filtering events based on event data.
type Filter interface {
	// MatchValues returns true if the event data matches the filter.
	Match(event EventData) (bool, error)
}

type eventIDFilter struct {
	// list of allowed event IDs
	eventIDs []int
}

// NewFilterFromConfig creates a new Filter from the given configuration.
func NewFilterFromConfig(config []byte) (Filter, error) {
	schema, err := unmarshalEventdataFilterSchema(config)
	if err != nil {
		return nil, err
	}
	f := &eventIDFilter{eventIDs: schema.EventIDs}
	return f, nil
}

func (f *eventIDFilter) Match(e EventData) (bool, error) {
	vals := e.SystemValues()
	if vals == nil {
		return false, fmt.Errorf("event data is nil")
	}
	eventID, err := vals.UInt(evtapi.EvtSystemEventID)
	if err != nil {
		return false, fmt.Errorf("error getting event ID: %w", err)
	}
	return f.isAllowedEventID(int(eventID)), nil
}

func (f *eventIDFilter) isAllowedEventID(eventID int) bool {
	// if no event IDs are specified, allow all events
	if len(f.eventIDs) == 0 {
		return true
	}
	for _, id := range f.eventIDs {
		if eventID == id {
			return true
		}
	}
	return false
}
