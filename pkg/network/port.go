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
	procRoot     string
	collectTCP   bool
	collectIPv6  bool
	ports        map[string]interface{}
	defaultNetNs uint64
	sync.RWMutex
}

//NewPortMapping creates a new PortMapping instance
func NewPortMapping(procRoot string, collectTCP, collectIPv6 bool, defaultNetNs uint64) *PortMapping {
	return &PortMapping{
		procRoot:     procRoot,
		collectTCP:   collectTCP,
		collectIPv6:  collectIPv6,
		ports:        make(map[string]interface{}),
		defaultNetNs: defaultNetNs,
	}
}

// AddMapping adds/overwrites a port mapping in the default network
// namespace
func (pm *PortMapping) AddMapping(port uint16) {
	pm.Lock()
	defer pm.Unlock()

	pm.ports[portMappingKey(pm.defaultNetNs, port)] = struct{}{}
}

// RemoveMapping removes a port mapping in the default network
// namespace
func (pm *PortMapping) RemoveMapping(port uint16) {
	pm.Lock()
	defer pm.Unlock()

	delete(pm.ports, portMappingKey(pm.defaultNetNs, port))
}

// AddMappingWithNs adds a port mapping in the given network namespace
func (pm *PortMapping) AddMappingWithNs(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	pm.ports[portMappingKey(nsIno, port)] = struct{}{}
}

// RemoveMappingWithNs removes a port mapping in the given network namespace
func (pm *PortMapping) RemoveMappingWithNs(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	delete(pm.ports, portMappingKey(nsIno, port))
}

// IsListening returns true if something is listening on the given port in
// the default network namespace
func (pm *PortMapping) IsListening(port uint16) bool {
	pm.RLock()
	defer pm.RUnlock()

	_, ok := pm.ports[portMappingKey(pm.defaultNetNs, port)]
	return ok
}

// IsListeningWithNs returns true if something is listening on the given port
// in the given network namespace
func (pm *PortMapping) IsListeningWithNs(nsIno uint64, port uint16) bool {
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
			log.Errorf("error getting net ns for pid %d", pid)
			return nil
		}

		if _, ok := seen[nsIno]; ok {
			return nil
		}

		seen[nsIno] = struct{}{}

		if ports, err := readProcNetListeners(path.Join(pm.procRoot, fmt.Sprintf("%d/net/tcp", pid))); err != nil {
			log.Errorf("error reading tcp state: %s", err)
		} else {
			log.Tracef("read TCP ports for net ns %d: %v", nsIno, ports)
			for _, port := range ports {
				pm.ports[portMappingKey(nsIno, port)] = struct{}{}
			}
		}

		if !pm.collectIPv6 {
			return nil
		}

		if ports, err := readProcNetListeners(path.Join(pm.procRoot, fmt.Sprintf("%d/net/tcp6", pid))); err != nil {
			log.Errorf("error reading tcp6 state: %s", err)
		} else {
			log.Tracef("read TCPv6 ports for net ns %d: %v", nsIno, ports)
			for _, port := range ports {
				pm.ports[portMappingKey(nsIno, port)] = struct{}{}
			}
		}

		return nil
	})
}

// ReadInitialUDPState reads the /proc filesystem and determines which ports are being used as UDP server
func (pm *PortMapping) ReadInitialUDPState() error {
	pm.Lock()
	defer pm.Unlock()

	seen := make(map[uint64]interface{})

	return util.WithAllProcs(pm.procRoot, func(pid int) error {
		nsIno, err := util.GetNetNsInoFromPid(pm.procRoot, pid)
		if err != nil {
			log.Errorf("error getting net ns for pid %d", pid)
			return nil
		}

		if _, ok := seen[nsIno]; ok {
			return nil
		}

		seen[nsIno] = struct{}{}

		udpPath := path.Join(pm.procRoot, fmt.Sprintf("%d/net/udp", pid))
		if ports, err := readProcNetWithStatus(udpPath, tcpClose); err != nil {
			log.Errorf("failed to read UDP state for net ns %d: %s", nsIno, err)
		} else {
			log.Tracef("read UDP ports for net ns %d: %v", nsIno, ports)
			for _, port := range ports {
				pm.ports[portMappingKey(nsIno, port)] = struct{}{}
			}
		}

		if !pm.collectIPv6 {
			return nil
		}

		if ports, err := readProcNetWithStatus(path.Join(pm.procRoot, fmt.Sprintf("%d/net/udp6", pid)), tcpClose); err != nil {
			log.Errorf("error reading UDPv6 state for net ns %d: %s", nsIno, err)
		} else {
			log.Tracef("read UDPv6 state for net ns %d: %v", nsIno, ports)
			for _, port := range ports {
				pm.ports[portMappingKey(nsIno, port)] = struct{}{}
			}
		}

		return nil
	})
}

func portMappingKey(nsIno uint64, port uint16) string {
	return fmt.Sprintf("%d:%d", nsIno, port)
}
