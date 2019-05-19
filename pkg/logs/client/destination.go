package client

type Destination interface {
	Send(payload []byte) error
	SendAsync(payload []byte)
}
