package listeners

// Payload reprensents a statsd packet ready to process,
// with its origin metadata if applicable.
type Payload struct {
	Contents  []byte // Contents, might contain several messages
	Container string // Origin container if identified
}

// StatsdListener opens a communication channel to get statsd packets in.
type StatsdListener interface {
	Listen()
	Stop()
}
