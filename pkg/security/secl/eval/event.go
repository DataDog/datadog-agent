package eval

import "reflect"

type Event interface {
	GetID() string
	GetType() EventType
	GetFieldTags(field Field) ([]string, error)
	GetFieldEventType(field Field) (EventType, error)
	SetFieldValue(field Field, value interface{}) error
	GetFieldValue(field Field) (interface{}, error)
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
