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
	ports       map[string]interface{}
	sync.RWMutex
}

//NewPortMapping creates a new PortMapping instance
func NewPortMapping(procRoot string, collectTCP, collectIPv6 bool) *PortMapping {
	return &PortMapping{
		procRoot:    procRoot,
		collectTCP:  collectTCP,
		collectIPv6: collectIPv6,
		ports:       make(map[string]interface{}),
	}
}

// AddMapping adds a port mapping in the given network namespace
func (pm *PortMapping) AddMapping(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	pm.ports[portMappingKey(nsIno, port)] = struct{}{}
}

// RemoveMapping removes a port mapping in the given network namespace
func (pm *PortMapping) RemoveMapping(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	delete(pm.ports, portMappingKey(nsIno, port))
}

// IsListening returns true if something is listening on the given port
// in the given network namespace
func (pm *PortMapping) IsListening(nsIno uint64, port uint16) bool {
	pm.RLock()
	defer pm.RUnlock()

	_, ok := pm.ports[portMappingKey(nsIno, port)]
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

	seen := make(map[uint64]interface{})

	return util.WithAllProcs(pm.procRoot, func(pid int) error {
		nsIno, err := util.GetNetNsInoFromPid(pm.procRoot, pid)
		if err != nil {
			log.Errorf("error getting net ns for pid %d, port mappings will not be read for this process", pid)
			return nil
		}

		if _, ok := seen[nsIno]; ok {
			return nil
		}

		seen[nsIno] = struct{}{}
		pm.readPorts(nsIno, pid)
		return nil
	})
}

func (pm *PortMapping) readPorts(nsIno uint64, pid int) {
	if ports, err := readProcNetListeners(path.Join(pm.procRoot, fmt.Sprintf("%d/net/tcp", pid))); err != nil {
		log.Errorf("error reading tcp state for pid %d: %s", pid, err)
	} else {
		log.Tracef("read TCP ports for net ns %d: %v", nsIno, ports)
		for _, port := range ports {
			pm.ports[portMappingKey(nsIno, port)] = struct{}{}
		}
	}

	if ports, err := readProcNetWithStatus(path.Join(pm.procRoot, fmt.Sprintf("%d/net/udp", pid)), tcpClose); err != nil {
		log.Errorf("error reading UDP state for pid %d: %s", pid, err)
	} else {
		log.Tracef("read UDP ports for net ns %d: %v", nsIno, ports)
		for _, port := range ports {
			// we use 0 for the network namespace for udp since we don't
			// have net namespace info availlable from bpf for udp
			pm.ports[portMappingKey(0, port)] = struct{}{}
		}
	}

	if !pm.collectIPv6 {
		return
	}

	if ports, err := readProcNetListeners(path.Join(pm.procRoot, fmt.Sprintf("%d/net/tcp6", pid))); err != nil {
		log.Errorf("error reading tcp6 state for pid: %s", pid, err)
	} else {
		log.Tracef("read TCPv6 ports for net ns %d: %v", nsIno, ports)
		for _, port := range ports {
			pm.ports[portMappingKey(nsIno, port)] = struct{}{}
		}
	}

	if ports, err := readProcNetWithStatus(path.Join(pm.procRoot, fmt.Sprintf("%d/net/udp6", pid)), tcpClose); err != nil {
		log.Errorf("error reading UDPv6 state for pid %d: %s", pid, err)
	} else {
		log.Tracef("read UDPv6 ports for net ns %d: %v", nsIno, ports)
		for _, port := range ports {
			// we use 0 for the network namespace for udp since we don't
			// have net namespace info availlable from bpf for udp
			pm.ports[portMappingKey(0, port)] = struct{}{}
		}
	}

}

func portMappingKey(nsIno uint64, port uint16) string {
	return fmt.Sprintf("%d:%d", nsIno, port)
}
