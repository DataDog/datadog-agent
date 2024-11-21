package tcp

import "errors"

func (t *TCPv4) TracerouteSequential() (*Results, error) {
	return nil, errors.New("not implemented")
}

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (t *TCPv4) Close() error {
	return nil
}
