package network

import (
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PortMapping tracks which ports a pid is listening on
type PortMapping struct {
	procRoot    string
	collectTCP  bool
	collectIPv6 bool
	ports       map[uint16]struct{}
	sync.RWMutex
}

//NewPortMapping creates a new PortMapping instance
func NewPortMapping(procRoot string, collectTCP, collectIPv6 bool) *PortMapping {
	return &PortMapping{
		procRoot:    procRoot,
		collectTCP:  collectTCP,
		collectIPv6: collectIPv6,
		ports:       make(map[uint16]struct{}),
	}
}

// AddMapping indicates that something is listening on the provided port
func (pm *PortMapping) AddMapping(port uint16) {
	pm.Lock()
	defer pm.Unlock()

	pm.ports[port] = struct{}{}
}

// RemoveMapping indicates that the provided port is no longer being listened on
func (pm *PortMapping) RemoveMapping(port uint16) {
	pm.Lock()
	defer pm.Unlock()

	delete(pm.ports, port)
}

// IsListening returns true if something is listening on the given port
func (pm *PortMapping) IsListening(port uint16) bool {
	pm.RLock()
	defer pm.RUnlock()

	_, ok := pm.ports[port]
	return ok
}

// ReadInitialState reads the /proc filesystem and determines which ports are being listened on
func (pm *PortMapping) ReadInitialState() error {
	pm.Lock()
	defer pm.Unlock()

	start := time.Now()

	if pm.collectTCP {
		if ports, err := readProcNet(path.Join(pm.procRoot, "net/tcp")); err != nil {
			log.Errorf("error reading tcp state: %s", err)
		} else {
			for _, port := range ports {
				pm.ports[port] = struct{}{}
			}
		}

		if pm.collectIPv6 {
			if ports, err := readProcNet(path.Join(pm.procRoot, "net/tcp6")); err != nil {
				log.Errorf("error reading tcp6 state: %s", err)
			} else {
				for _, port := range ports {
					pm.ports[port] = struct{}{}
				}
			}
		}
	}

	log.Debugf("Read initial pid->port mapping in %s", time.Now().Sub(start))

	return nil
}

// ReadInitialUDPState reads the /proc filesystem and determines which ports are being used as UDP server
func (pm *PortMapping) ReadInitialUDPState() error {
	pm.Lock()
	defer pm.Unlock()

	udpPath := path.Join(pm.procRoot, "net/udp")
	if ports, err := readProcNetWithStatus(udpPath, tcpClose); err != nil {
		log.Errorf("failed to read UDP state: %s", err)
	} else {
		log.Info("read UDP ports: %v", ports)
		for _, port := range ports {
			pm.ports[port] = struct{}{}
		}
	}

	if pm.collectIPv6 {
		if ports, err := readProcNetWithStatus(path.Join(pm.procRoot, "net/udp6"), 7); err != nil {
			log.Errorf("error reading UDPv6 state: %s", err)
		} else {
			log.Info("read UDPv6 state: %v", ports)
			for _, port := range ports {
				pm.ports[port] = struct{}{}
			}
		}
	}

	return nil
}
