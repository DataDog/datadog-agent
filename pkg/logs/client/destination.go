package client

// Destination TODO
type Destination interface {
	Send(payload []byte) error
	SendAsync(payload []byte)
}
