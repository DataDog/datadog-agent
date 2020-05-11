package eval

type Event interface {
	GetID() string
	GetType() string
}
