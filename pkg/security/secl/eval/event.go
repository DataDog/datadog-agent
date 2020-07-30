// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package eval

import (
	"reflect"
	"unsafe"
)

// EventType - Type of an event
type EventType = string

// Event - Interface that an Event has to implement for the evaluation
type Event interface {
	// GetID - Returns the ID of the Event
	GetID() string
	// GetType - Returns the Type of the Event
	GetType() EventType
	// GetFieldEventType - Returns the Event Type for the given Field
	GetFieldEventType(field Field) (EventType, error)
	// SetFieldValue - Set the value of the given Field
	SetFieldValue(field Field, value interface{}) error
	// GetFieldValue - Returns the value of the given Field
	GetFieldValue(field Field) (interface{}, error)
	// GetFieldType - Returns the Type of the Field
	GetFieldType(field Field) (reflect.Kind, error)
	// GetPointer() - Returns an unsafe.Pointer of this object
	GetPointer() unsafe.Pointer
}

func eventTypesFromFields(model Model, state *state) ([]EventType, error) {
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
