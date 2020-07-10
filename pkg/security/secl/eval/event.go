package eval

import "reflect"

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
}

func eventFromFields(model Model, state *state) ([]EventType, error) {
	events := make(map[EventType]bool)
	for field := range state.fieldValues {
		eventType, err := model.GetEvent().GetFieldEventType(field)
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
