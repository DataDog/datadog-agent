// +build !windows

package metrics

import (
	"fmt"
	"runtime"
	"time"
)

func DialPipe(path string, timeout *time.Duration) (*statsWriter, error) {
	return nil, fmt.Errorf("Windows Pipe not available on %s", runtime.GOOS)
}
