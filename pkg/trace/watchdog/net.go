// +build !windows

package watchdog

import (
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/net"
	"os"
	"time"
)

// Net returns basic network info.
func (pi *CurrentInfo) Net() NetInfo {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	now := time.Now()
	dt := now.Sub(pi.lastNetTime)
	if dt <= pi.cacheDelay {
		return pi.lastNet // don't query too often, cache a little bit
	}
	pi.lastNetTime = now

	connections, err := net.ConnectionsPid("tcp", int32(os.Getpid()))
	if err != nil {
		log.Debugf("unable to get Net connections: %v", err)
		return pi.lastNet
	}

	pi.lastNet.Connections = int32(len(connections))

	return pi.lastNet
}
