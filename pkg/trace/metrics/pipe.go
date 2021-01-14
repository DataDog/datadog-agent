// +build !windows

package metrics

import (
	"fmt"
	"net"
	"runtime"
	"time"
)

func DialPipe(path string, timeout *time.Duration) (net.Conn, error) {
	return nil, fmt.Errorf("Windows Pipe not available on %s", runtime.GOOS)
}
