package network

import (
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PortMapping tracks which ports a pid is listening on
type PortMapping struct {
	procRoot string
	config   *Config
	ports    map[uint16]struct{}
	sync.RWMutex
}

//NewPortMapping creates a new PortMapping instance
func NewPortMapping(procRoot string, config *Config) *PortMapping {
	return &PortMapping{
		procRoot: procRoot,
		config:   config,
		ports:    make(map[uint16]struct{}),
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

	if pm.config.CollectTCPConns {
		if ports, err := readProcNet(path.Join(pm.procRoot, "net/tcp")); err != nil {
			log.Errorf("error reading tcp state: %s", err)
		} else {
			for _, port := range ports {
				pm.ports[port] = struct{}{}
			}
		}

		if pm.config.CollectIPv6Conns {
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
