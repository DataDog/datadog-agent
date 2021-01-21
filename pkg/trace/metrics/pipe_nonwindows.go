// +build !windows

package metrics

import (
	"fmt"
	"runtime"
	"time"
)

func dialPipe(path string, timeout *time.Duration) (*statsWriter, error) {
	return nil, fmt.Errorf("Windows Pipe not available on %s", runtime.GOOS)
}
