package metrics

import (
	"errors"
	"net"
	"time"
)

type statsWriter struct {
	net.Conn
}

// SetWriteTimeout is not available for Windows Pipes. returns error
func (w *statsWriter) SetWriteTimeout(d time.Duration) error {
	return errors.New("SetWriteTimeout: not supported for Windows Pipe connections")
}
