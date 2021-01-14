package metrics

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func DialPipe(path string, timeout *time.Duration) (net.Conn, error) {
	return winio.DialPipe(path, timeout)
}
