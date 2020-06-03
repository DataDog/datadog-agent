package eval

import "reflect"

type Event interface {
	GetID() string
	GetType() EventType
	GetFieldTags(field string) ([]string, error)
	GetFieldEventType(field Field) (EventType, error)
	SetFieldValue(field Field, value interface{}) error
	GetFieldValue(field Field) (interface{}, error)
	GetFieldType(field Field) (reflect.Kind, error)
}

// RuleEvent - Rule event wrapper used to send an event to the backend
type RuleEvent struct {
	RuleID string `json:"rule_id"`
	Event  Event  `json:"event"`
}
