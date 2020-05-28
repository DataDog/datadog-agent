package eval

type Event interface {
	GetID() string
	GetType() string
	GetFieldTags(key string) ([]string, error)
	GetFieldEventType(key string) (string, error)
	SetFieldValue(key string, value interface{}) error
	GetFieldValue(key string) (interface{}, error)
}
