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
