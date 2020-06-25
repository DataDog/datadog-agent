package eval

type Event interface {
	GetID() string
	GetType() string
	GetFieldTags(key string) ([]string, error)
	GetFieldEventType(key string) (string, error)
	SetFieldValue(key string, value interface{}) error
	GetFieldValue(key string) (interface{}, error)
}

// RuleEvent - Rule event wrapper used to send an event to the backend
type RuleEvent struct {
	RuleID string `json:"rule_id"`
	Event  Event  `json:"event"`
}
