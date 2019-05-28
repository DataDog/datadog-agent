package client

// Destination sends a payload to a specific endpoint over a given network protocol.
type Destination interface {
	Send(payload []byte) error
	SendAsync(payload []byte)
}
