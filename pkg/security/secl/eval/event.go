package eval

type Event interface {
	GetID() string
	GetType() string
	GetTags(key string) ([]string, error)
	GetEventType(key string) (string, error)
	SetEventValue(key string, value interface{}) error
}
