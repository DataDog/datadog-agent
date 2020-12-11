// +build linux

package network

import (
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PortMapping tracks which ports a pid is listening on
type PortMapping struct {
	procRoot    string
	collectTCP  bool
	collectIPv6 bool
	ports       map[portMappingKey]interface{}
	sync.RWMutex
}

type portMappingKey struct {
	ino  uint64
	port uint16
}

//NewPortMapping creates a new PortMapping instance
func NewPortMapping(procRoot string, collectTCP, collectIPv6 bool) *PortMapping {
	return &PortMapping{
		procRoot:    procRoot,
		collectTCP:  collectTCP,
		collectIPv6: collectIPv6,
		ports:       make(map[portMappingKey]interface{}),
	}
}

// AddMapping adds a port mapping in the given network namespace
func (pm *PortMapping) AddMapping(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	pm.ports[portMappingKey{ino: nsIno, port: port}] = struct{}{}
}

// RemoveMapping removes a port mapping in the given network namespace
func (pm *PortMapping) RemoveMapping(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	delete(pm.ports, portMappingKey{ino: nsIno, port: port})
}

// IsListening returns true if something is listening on the given port
// in the given network namespace
func (pm *PortMapping) IsListening(nsIno uint64, port uint16) bool {
	pm.RLock()
	defer pm.RUnlock()

	_, ok := pm.ports[portMappingKey{ino: nsIno, port: port}]
	return ok
}

// ReadInitialState reads the /proc filesystem and determines which ports are being listened on
func (pm *PortMapping) ReadInitialState() error {
	pm.Lock()
	defer pm.Unlock()

	start := time.Now()
	defer func() {
		log.Debugf("Read initial pid->port mapping in %s", time.Now().Sub(start))
	}()

	paths := []string{"net/tcp"}
	if pm.collectIPv6 {
		paths = append(paths, "net/tcp6")
	}

	pm.readState(paths, tcpListen, false)
	return nil
}

// ReadInitialUDPState reads the /proc filesystem and determines which ports are being used as UDP server
func (pm *PortMapping) ReadInitialUDPState() error {
	pm.Lock()
	defer pm.Unlock()

	paths := []string{"net/udp"}
	if pm.collectIPv6 {
		paths = append(paths, "net/udp6")
	}

	pm.readState(paths, tcpClose, true)
	return nil
}

func (pm *PortMapping) readState(paths []string, status int64, isUDP bool) {
	seen := make(map[uint64]interface{})
	_ = util.WithAllProcs(pm.procRoot, func(pid int) error {
		nsIno, err := util.GetNetNsInoFromPid(pm.procRoot, pid)
		if err != nil {
			log.Errorf("error getting net ns for pid %d", pid)
			return nil
		}

		if _, ok := seen[nsIno]; ok {
			return nil
		}

		seen[nsIno] = struct{}{}
		if isUDP {
			// cannot use namespace info in key since ebpf port binding code does not provide namespace info
			nsIno = 0
		}

		for _, p := range paths {
			ports, err := readProcNetWithStatus(path.Join(pm.procRoot, fmt.Sprintf("%d", pid), p), status)
			if err != nil {
				log.Errorf("error reading port state net ns ino=%d pid=%d path=%s status=%d", nsIno, pid, p, status)
				continue
			}

			for _, port := range ports {
				pm.ports[portMappingKey{ino: nsIno, port: port}] = struct{}{}
			}
		}

		return nil
	})
}
