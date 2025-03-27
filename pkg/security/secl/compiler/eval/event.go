// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"reflect"
)

// EventType is the type of an event
type EventType = string

// Event is an interface that an Event has to implement for the evaluation
type Event interface {
	// Init initialize the event
	Init()
	// GetType returns the Type of the Event
	GetType() EventType
	// GetFieldEventType returns the Event Field Metadata for the given Field
	GetFieldMetadata(field Field) (EventType, reflect.Kind, string, error)
	// SetFieldValue sets the value of the given Field
	SetFieldValue(field Field, value interface{}) error
	// GetFieldValue returns the value of the given Field
	GetFieldValue(field Field) (interface{}, error)
	// GetTags returns a list of tags
	GetTags() []string
}

func eventTypeFromFields(model Model, state *State) (EventType, error) {
	var eventType EventType

	// if there are no fields, we can't determine the event type
	// this is not uncommon, especially in macros
	if len(state.fieldValues) == 0 {
		return eventType, nil
	}

	ev := model.NewEvent()
	for field := range state.fieldValues {
		evt, _, _, err := ev.GetFieldMetadata(field)
		if err != nil {
			return "", err
		}

		if evt != "" {
			if eventType != "" && eventType != evt {
				return "", ErrMultipleEventTypes
			}
			eventType = evt
		}
	}
	return eventType, nil
}
