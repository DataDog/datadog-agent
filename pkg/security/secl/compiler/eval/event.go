// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"reflect"
	"unsafe"
)

// EventType is the type of an event
type EventType = string

// Event is an interface that an Event has to implement for the evaluation
type Event interface {
	// GetType returns the Type of the Event
	GetType() EventType
	// GetFieldEventType returns the Event Type for the given Field
	GetFieldEventType(field Field) (EventType, error)
	// SetFieldValue sets the value of the given Field
	SetFieldValue(field Field, value interface{}) error
	// GetFieldValue returns the value of the given Field
	GetFieldValue(field Field) (interface{}, error)
	// GetFieldType returns the Type of the Field
	GetFieldType(field Field) (reflect.Kind, error)
	// GetPointer returns an unsafe.Pointer of this object
	GetPointer() unsafe.Pointer
	// GetTags returns a list of tags
	GetTags() []string
}

func eventTypesFromFields(model Model, state *RuleState) ([]EventType, error) {
	events := make(map[EventType]bool)
	for field := range state.fieldValues {
		eventType, err := model.NewEvent().GetFieldEventType(field)
		if err != nil {
			return nil, err
		}

		if eventType != "*" {
			events[eventType] = true
		}
	}

	var uniq []string
	for event := range events {
		uniq = append(uniq, event)
	}
	return uniq, nil
}
